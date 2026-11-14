package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/hub"
	"mega-magic/internal/provisioner/routeros"
)

type pendingManagement struct {
	private, public, proof, user, routerKey string
	tunnelScript                            string
	sourceCIDR                              string
	expires                                 time.Time
}

// Setup material stays until the router connects or the lifetime ends, so a script
// pasted late still connects without a second paste. It is encrypted in the database,
// never plaintext. Verified keys use the existing encrypted credential store and
// pinned transport, including after restarts.
type automaticManagementOptions struct {
	sourceCIDR string
	store      *database.PendingEnrollmentStore
}

func configuredManagementSource(interfaceName, explicit string) (string, error) {
	if explicit != "" {
		p, e := netip.ParsePrefix(explicit)
		if e != nil || !p.Addr().Is4() || !p.Addr().IsPrivate() || p.Bits() != 32 {
			return "", errors.New("management source must be one private IPv4 /32")
		}
		return p.String(), nil
	}
	iface, e := net.InterfaceByName(interfaceName)
	if e != nil {
		return "", errors.New("management tunnel interface unavailable")
	}
	addrs, e := iface.Addrs()
	if e != nil {
		return "", e
	}
	return managementSourceCIDR(addrs)
}
func configureAutomaticHubManagement(h *handler.VPSHandler, source *database.DeviceCredentialSource, cidrs []string, interfaceName string, options ...automaticManagementOptions) {
	var option automaticManagementOptions
	if len(options) > 0 {
		option = options[0]
	}
	h.ConfigureHubConnectionState(func(ctx context.Context, biz, id string) (handler.HubConnectionState, error) {
		if option.store == nil {
			return handler.HubConnectionState{Management: "unknown", Setup: "unknown"}, nil
		}
		metadata, e := option.store.Inspect(ctx, biz, id)
		if e != nil {
			return handler.HubConnectionState{}, e
		}
		return hubConnectionState(metadata), nil
	})
	h.ConfigureAutomaticPreflight(func(ctx context.Context, biz, id string) error {
		if _, e := configuredManagementSource(interfaceName, option.sourceCIDR); e != nil {
			return e
		}
		previous, e := source.TargetForDevice(ctx, biz, id)
		if e != nil && !errors.Is(e, routeros.ErrNoTarget) {
			return e
		}
		if previous != nil {
			return handler.ErrHubManagementConfigured
		}
		return nil
	})
	var mu sync.Mutex
	pending := make(map[string]pendingManagement)
	h.ConfigureAutomaticManagement(func(ctx context.Context, biz, id, routerKey, tunnelScript string) (string, error) {
		sourceCIDR, err := configuredManagementSource(interfaceName, option.sourceCIDR)
		if err != nil {
			return "", err
		}
		if !strings.Contains(tunnelScript, `dst-address="`+sourceCIDR+`"`) {
			return "", errors.New("management source is not included in scoped tunnel routes")
		}
		previous, err := source.TargetForDevice(ctx, biz, id)
		if err != nil && !errors.Is(err, routeros.ErrNoTarget) {
			return "", err
		}
		if previous != nil {
			return "", handler.ErrHubManagementConfigured
		}
		mu.Lock()
		defer mu.Unlock()
		for k, p := range pending {
			if time.Now().After(p.expires) {
				delete(pending, k)
			}
		}
		key := biz + "/" + id
		if p, ok := pending[key]; ok && p.routerKey == routerKey && p.sourceCIDR == sourceCIDR {
			return automaticEnrollmentScript(id, p)
		}
		if option.store == nil && len(pending) >= 256 {
			return "", errors.New("too many pending enrollments")
		}
		var p pendingManagement
		if option.store != nil {
			generate := func() (string, error) {
				v, e := newPendingManagement(biz, id, routerKey, tunnelScript, sourceCIDR)
				if e != nil {
					return "", e
				}
				return encodePendingManagement(v)
			}
			raw, e := option.store.GetOrCreate(ctx, biz, id, "management", generate, database.PendingEnrollmentLifetime)
			if e != nil {
				return "", e
			}
			p, e = decodePendingManagement(raw)
			if e != nil {
				return "", e
			}
			// The router was given a new key, so older setup material no longer fits it.
			if p.routerKey != routerKey || p.sourceCIDR != sourceCIDR {
				if e = option.store.DeletePurpose(ctx, biz, id, "management"); e != nil {
					return "", e
				}
				if raw, e = option.store.GetOrCreate(ctx, biz, id, "management", generate, database.PendingEnrollmentLifetime); e != nil {
					return "", e
				}
				if p, e = decodePendingManagement(raw); e != nil {
					return "", e
				}
			}
		} else {
			var e error
			p, e = newPendingManagement(biz, id, routerKey, tunnelScript, sourceCIDR)
			if e != nil {
				return "", e
			}
		}
		script, err := automaticEnrollmentScript(id, p)
		if err != nil {
			return "", err
		}
		pending[key] = p
		return script, nil
	}, func(ctx context.Context, biz, id, address, routerKey string) error {
		mu.Lock()
		defer mu.Unlock()
		key := biz + "/" + id
		previous, err := source.TargetForDevice(ctx, biz, id)
		if err != nil && !errors.Is(err, routeros.ErrNoTarget) {
			return err
		}
		if previous != nil {
			if previous.Addr != routeros.JoinHostPort(address, 22) {
				return errors.New("management address changed")
			}
			policy, err := routeros.NewCIDRTargetSource(enrollmentTarget{previous}, cidrs)
			if err != nil {
				return err
			}
			checked, err := policy.TargetForDevice(ctx, biz, id)
			if err != nil {
				return err
			}
			cfg, err := routeros.SSHConfigFromTarget(checked, 15*time.Second)
			if err != nil {
				return err
			}
			_, err = routeros.NewSSHTransport(cfg).Run(ctx, "/system resource print")
			if err == nil && option.store != nil {
				return option.store.Delete(ctx, biz, id)
			}
			return err
		}
		p, ok := pending[key]
		if option.store != nil {
			raw, e := option.store.GetOrCreate(ctx, biz, id, "management", nil, 0)
			if e != nil {
				return e
			}
			if raw != "" {
				p, e = decodePendingManagement(raw)
				if e != nil {
					return e
				}
				ok = true
			}
		}
		if !ok || time.Now().After(p.expires) || p.routerKey != routerKey {
			return errors.New("pending enrollment unavailable")
		}
		target := &routeros.DeviceTarget{DeviceID: id, Addr: routeros.JoinHostPort(address, 22), User: p.user, Kind: "key", Secret: p.private}
		policy, err := routeros.NewCIDRTargetSource(enrollmentTarget{target}, cidrs)
		if err != nil {
			return err
		}
		if _, err := policy.TargetForDevice(ctx, biz, id); err != nil {
			return err
		}
		fingerprint, err := verifyAutomaticRouter(ctx, target, p)
		if err != nil {
			return err
		}
		if err := source.SetDeviceCredentials(ctx, biz, id, address, p.user, "key", p.private, 22, fingerprint); err != nil {
			return err
		}
		if option.store != nil {
			if e := option.store.Delete(ctx, biz, id); e != nil {
				return e
			}
		}
		delete(pending, key)
		return nil
	})
}

// The combined app/hub host and split-host deployments use different source
// addresses. Read the configured tunnel interface; never broaden to a subnet.
func managementSourceCIDR(addresses []net.Addr) (string, error) {
	var source string
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil {
			return "", errors.New("invalid management interface address")
		}
		if ip.To4() == nil {
			continue
		}
		if !ip.IsPrivate() || source != "" {
			return "", errors.New("ambiguous or public management source")
		}
		source = ip.String() + "/32"
	}
	if source == "" {
		return "", errors.New("management interface has no private IPv4 address")
	}
	return source, nil
}

// Keep both stages in one RouterOS execution block: a tunnel validation error
// must stop execution before accounts, keys or management rules are changed.
func automaticEnrollmentScript(id string, p pendingManagement) (string, error) {
	ip, network, err := net.ParseCIDR(p.sourceCIDR)
	if err != nil || ip.To4() == nil || !ip.IsPrivate() {
		return "", errors.New("invalid management source")
	}
	ones, bits := network.Mask.Size()
	if ones != 32 || bits != 32 {
		return "", errors.New("management source must be a single host")
	}
	tunnel := strings.TrimSpace(p.tunnelScript)
	if strings.Contains(tunnel, redactedRouterKey) {
		return "", errors.New("the router already connected and its setup key was removed; no script is reissued")
	}
	management := strings.TrimSpace(automaticManagementScript(id, p))
	if !strings.HasPrefix(tunnel, "{\n") || !strings.HasSuffix(tunnel, "\n}") {
		return "", errors.New("invalid tunnel execution block")
	}
	management = strings.ReplaceAll(management, "10.77.0.9/32", p.sourceCIDR)
	management = strings.ReplaceAll(management, "10.77.0.9", ip.String())
	// The tunnel block's header says it never touches SSH or users; this combined
	// paste does, so say what it really changes before the operator runs it.
	tunnel = strings.Replace(tunnel, "# Scoped tunnel only. Does not enable SSH/REST or change WAN/LAN/firewall/users.",
		"# Creates the Magic Hub tunnel, then a dedicated management user, its group and SSH key,\n# lets SSH in from the Hub address only, and adds one input firewall rule for it.\n# WAN, LAN and your other users are not changed.", 1)
	// The final line break makes the terminal run the block on paste. Without it,
	// a real WinBox terminal waited for one more Enter and looked finished.
	return strings.TrimSuffix(tunnel, "\n}") + "\n" + strings.TrimPrefix(management, "{\n") + "\n", nil
}

func automaticManagementScript(id string, p pendingManagement) string {
	hash := sha256.Sum256([]byte(id))
	iface := fmt.Sprintf("mmh-%x", hash[:6])
	marker := "mega-magic:" + p.user
	// All interpolated values are generated locally as hex, fixed labels or RSA
	// authorized-key data. No operator text reaches the RouterOS commands.
	// /ip/service also lists live SSH connections, so the service is the first match.
	return fmt.Sprintf(`{
# Dedicated Magic Hub management.
:local mmSSH [:pick [/ip/service find where name="ssh"] 0]
:local mmPort [/ip/service get $mmSSH port]
:local account [/user find where name="%[1]s"]
:if ([:len $account] = 1) do={
  :if ([:pick [/user get $account comment] 0 %[2]d] != "%[3]s:") do={ :error "STOP: a user named %[1]s exists and is not from Magic Hub; management unchanged" }
}
:foreach mmOldUser in=[/user find where name~"^mm-" and name!="%[1]s"] do={
  :local mmOldName [/user get $mmOldUser name]
  :if ([:pick [/user get $mmOldUser comment] 0 14] = "mega-magic:mm-") do={
    /user/ssh-keys remove [find where user=$mmOldName]
    /ip/firewall/filter remove [find where comment=("mega-magic:" . $mmOldName)]
    /ip/firewall/nat remove [find where comment=("mega-magic:" . $mmOldName)]
    /user remove $mmOldUser
    :do { /user/group remove [find where name=$mmOldName] } on-error={}
  }
}
:local grp [/user/group find where name="%[1]s"]
:if ([:len $grp] = 0) do={ /user/group add name="%[1]s" policy=ssh,read,write,policy,test,sensitive,reboot,ftp } else={ /user/group set $grp policy=ssh,read,write,policy,test,sensitive,reboot,ftp }
:if ([:len $account] = 0) do={ /user add name="%[1]s" group="%[1]s" address=10.77.0.9/32 password="%[4]s" comment="%[3]s:%[4]s" }
:if ([:len $account] = 1) do={ /user set $account group="%[1]s" address=10.77.0.9/32 password="%[4]s" comment="%[3]s:%[4]s" disabled=no }
:local keyfile "%[1]s.pub.txt"
:if ([:len [/file find where name=$keyfile]] = 0) do={ /file print file="%[1]s.pub"; :delay 1s }
/file set [find where name=$keyfile] contents="%[5]s"
/user/ssh-keys remove [find where user="%[1]s"]
/user/ssh-keys import public-key-file=$keyfile user="%[1]s"
/file remove [find where name=$keyfile]
# RouterOS names this list "available-from" now and "address" on older versions. Each name
# is parsed only when it runs, so a current router never reads the deprecated one.
:local mmSetAllowed do={ :do { [[:parse ("/ip/service set " . $1 . " available-from=" . $2)]] } on-error={ [[:parse ("/ip/service set " . $1 . " address=" . $2)]] } }
:local mmGetAllowed do={ :local mmValue ""; :do { :set mmValue [[:parse (":return [/ip/service get " . $1 . " available-from]")]] } on-error={ :do { :set mmValue [[:parse (":return [/ip/service get " . $1 . " address]")]] } on-error={} }; :return $mmValue }
:local mmAllowed [$mmGetAllowed $mmSSH]
:if ([/ip/service get $mmSSH disabled]) do={
  $mmSetAllowed $mmSSH "10.77.0.9/32"
  /ip/service set $mmSSH disabled=no
} else={
  :if ([:len $mmAllowed] > 0) do={
    :local mmHas false
    :local mmList "10.77.0.9/32"
    :foreach mmEntry in=$mmAllowed do={
      :if ([:tostr $mmEntry] = "10.77.0.9/32" || [:tostr $mmEntry] = "10.77.0.9") do={ :set mmHas true }
      :set mmList ($mmList . "," . [:tostr $mmEntry])
    }
    :if (!$mmHas) do={ $mmSetAllowed $mmSSH $mmList }
  }
}
/ip/firewall/filter remove [find where comment="%[3]s"]
:local mmFirst [/ip/firewall/filter find where dynamic=no]
:if ([:len $mmFirst] = 0) do={ /ip/firewall/filter add chain=input action=accept in-interface="%[6]s" src-address=10.77.0.9/32 protocol=tcp dst-port=$mmPort comment="%[3]s" } else={ /ip/firewall/filter add chain=input action=accept in-interface="%[6]s" src-address=10.77.0.9/32 protocol=tcp dst-port=$mmPort comment="%[3]s" place-before=[:pick $mmFirst 0] }
# The app always reaches this router on port 22 through the tunnel; a custom SSH port is mapped here.
/ip/firewall/nat remove [find where comment="%[3]s"]
:if ($mmPort != 22) do={
  :local mmNat [/ip/firewall/nat find where dynamic=no]
  :if ([:len $mmNat] = 0) do={ /ip/firewall/nat add chain=dstnat action=redirect to-ports=$mmPort in-interface="%[6]s" src-address=10.77.0.9/32 protocol=tcp dst-port=22 comment="%[3]s" } else={ /ip/firewall/nat add chain=dstnat action=redirect to-ports=$mmPort in-interface="%[6]s" src-address=10.77.0.9/32 protocol=tcp dst-port=22 comment="%[3]s" place-before=[:pick $mmNat 0] }
}
# Say what actually happened: a finished script does not mean the router reached the Hub.
:put "Checking the connection to Magic Hub (up to 30 seconds)..."
:local mmReached false
:foreach mmPeer in=[/interface/wireguard/peers find where interface="%[6]s"] do={
  :for mmTry from=1 to=30 do={
    :if (!$mmReached) do={
      :delay 1s
      :do { :if ([:tonum [/interface/wireguard/peers get $mmPeer rx]] > 0) do={ :set mmReached true } } on-error={}
    }
  }
}
:if ($mmReached) do={ :put "Setup complete. You can close this window: Mega Magic connects by itself." } else={ :put "NOT CONNECTED YET: this router has not reached Magic Hub. Check its internet connection. There is nothing more to paste: it connects by itself as soon as it can." }
}`, p.user, len(marker)+1, marker, p.proof, p.public, iface)
}

// Bootstrap trust is scoped to the cryptographically authenticated WireGuard
// peer and proven by a 256-bit secret installed by the local paste. Only SSH
// public-key authentication is offered, so no reusable password is sent to an
// unpinned host. The captured host key is persisted only after proof checks.
func verifyAutomaticRouter(ctx context.Context, target *routeros.DeviceTarget, p pendingManagement) (string, error) {
	signer, err := ssh.ParsePrivateKey([]byte(p.private))
	if err != nil {
		return "", err
	}
	var fingerprint string
	cfg := &ssh.ClientConfig{User: p.user, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fingerprint = ssh.FingerprintSHA256(key)
		return nil
	}}
	conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, "tcp", target.Addr)
	if err != nil {
		return "", errors.New("management connection unavailable")
	}
	defer conn.Close()
	deadline := time.Now().Add(15 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	sconn, chans, reqs, err := ssh.NewClientConn(conn, target.Addr, cfg)
	if err != nil {
		return "", errors.New("management key rejected")
	}
	client := ssh.NewClient(sconn, chans, reqs)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	hash := sha256.Sum256([]byte(target.DeviceID))
	output := &boundedEnrollmentOutput{}
	session.Stdout, session.Stderr = output, output
	err = session.Run(fmt.Sprintf(`:put [/user get [find where name="%s"] comment]; :put [/interface/wireguard get [find where name="mmh-%x"] public-key]; /system resource print`, p.user, hash[:6]))
	text := output.String()
	proofOK, keyOK := false, false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		proofOK = proofOK || line == "mega-magic:"+p.user+":"+p.proof
		keyOK = keyOK || line == p.routerKey
	}
	if err != nil || !proofOK || !keyOK || !strings.Contains(text, "board-name:") || !strings.Contains(text, "version:") {
		return "", errors.New("router enrollment proof failed")
	}
	return fingerprint, nil
}

type boundedEnrollmentOutput struct{ buffer bytes.Buffer }

func (b *boundedEnrollmentOutput) String() string { return b.buffer.String() }

func (b *boundedEnrollmentOutput) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > 65536 {
		return 0, errors.New("enrollment response too large")
	}
	return b.buffer.Write(p)
}

func newPendingManagement(biz, id, routerKey, tunnelScript, sourceCIDR string) (pendingManagement, error) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return pendingManagement{}, err
	}
	pub, err := ssh.NewPublicKey(&private.PublicKey)
	if err != nil {
		return pendingManagement{}, err
	}
	proof := make([]byte, 32)
	if _, err := rand.Read(proof); err != nil {
		return pendingManagement{}, err
	}
	p := pendingManagement{private: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(private)})), public: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))), proof: hex.EncodeToString(proof), user: hub.ManagementUserName(biz, id), routerKey: routerKey, expires: time.Now().Add(database.PendingEnrollmentLifetime)}
	p.tunnelScript = tunnelScript
	p.sourceCIDR = sourceCIDR
	return p, nil
}

type storedPendingManagement struct {
	Private, Public, Proof, User, RouterKey, TunnelScript, SourceCIDR string
	Expires                                                           time.Time
}

func encodePendingManagement(p pendingManagement) (string, error) {
	b, e := json.Marshal(storedPendingManagement{p.private, p.public, p.proof, p.user, p.routerKey, p.tunnelScript, p.sourceCIDR, p.expires})
	return string(b), e
}
func decodePendingManagement(raw string) (pendingManagement, error) {
	var s storedPendingManagement
	if json.Unmarshal([]byte(raw), &s) != nil || s.Private == "" || s.Proof == "" || s.TunnelScript == "" {
		return pendingManagement{}, errors.New("invalid pending management envelope")
	}
	return pendingManagement{s.Private, s.Public, s.Proof, s.User, s.RouterKey, s.TunnelScript, s.SourceCIDR, s.Expires}, nil
}
