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

// combinedHeader describes what the combined paste really changes. It replaces
// the tunnel block's own header, which only describes the tunnel.
const combinedHeader = `# Magic Hub one-paste setup for this gateway. Paste the entire block in WinBox New Terminal.
# It checks everything first and stops before any change if something does not belong to this setup.
# It then adds: this gateway's WireGuard tunnel (mmh-...) with one address and /32 routes to Magic Hub;
# a dedicated management user and group (mm-...) that can log in only from the Magic Hub address, with
# an SSH key; one input firewall rule for that address (and a port redirect if SSH is not on port 22).
# If the SSH service is disabled it is enabled for the Magic Hub address only; an existing SSH
# allow-list gets that address added. WAN, LAN, Wi-Fi, HotSpot, DHCP, DNS, your other users,
# other VPNs and other firewall rules are not changed. Other Magic Hub accounts are not removed.
# Turn on Safe Mode (Ctrl+X) before pasting to be able to undo it; the end of the output says how to keep it.
`

// Keep both stages in one RouterOS execution block, and run the management
// ownership checks together with the tunnel checks, before the tunnel's first
// write: any refusal stops execution before tunnels, accounts, keys or rules change.
// (One RouterOS block is not a transaction; Safe Mode is the undo.)
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
	if !strings.HasPrefix(tunnel, "{\n") || !strings.HasSuffix(tunnel, "\n}") {
		return "", errors.New("invalid tunnel execution block")
	}
	applyAt := strings.Index(tunnel, "\n# APPLY:")
	if applyAt < 0 || strings.Count(tunnel, "\n# APPLY:") != 1 {
		return "", errors.New("tunnel block lacks a single preflight/apply boundary")
	}
	// Drop the tunnel-only header comment lines; the combined header replaces them.
	body := strings.TrimPrefix(tunnel[:applyAt], "{\n")
	for strings.HasPrefix(body, "#") {
		if nl := strings.IndexByte(body, '\n'); nl >= 0 {
			body = body[nl+1:]
		} else {
			body = ""
		}
	}
	scrub := func(s string) string {
		s = strings.ReplaceAll(s, "10.77.0.9/32", p.sourceCIDR)
		return strings.ReplaceAll(s, "10.77.0.9", ip.String())
	}
	preflight := scrub(managementPreflight(p))
	apply := scrub(managementApply(id, p))
	tunnelApply := strings.TrimSuffix(tunnel[applyAt+1:], "\n}")
	// The final line break makes the terminal run the block on paste. Without it,
	// a real WinBox terminal waited for one more Enter and looked finished.
	return "{\n" + combinedHeader + body + preflight + "\n" + tunnelApply + "\n" + apply + "}\n", nil
}

// automaticManagementScript is the management part on its own (checks, then
// changes) as one block. The combined paste uses the two parts separately.
func automaticManagementScript(id string, p pendingManagement) string {
	return "{\n# Dedicated Magic Hub management.\n" + managementPreflight(p) + managementApply(id, p) + "}"
}

// managementPreflight checks, without writing, that the management user and
// group either do not exist or demonstrably belong to this enrollment.
// Other Magic Hub (mm-...) accounts are left alone: retiring them is an
// explicit step (the disconnect script), never a side effect of setup.
func managementPreflight(p pendingManagement) string {
	marker := "mega-magic:" + p.user
	return fmt.Sprintf(`
# PREFLIGHT (management): ownership checks before any change.
:local account [/user find where name="%[1]s"]
:if ([:len $account] > 1) do={ :error "STOP: duplicate users named %[1]s; nothing changed" }
:if ([:len $account] = 1) do={
  :if ([:pick [/user get $account comment] 0 %[2]d] != "%[3]s:") do={ :error "STOP: a user named %[1]s exists and is not from Magic Hub; nothing changed" }
}
:local grp [/user/group find where name="%[1]s"]
:if ([:len $grp] > 1) do={ :error "STOP: duplicate groups named %[1]s; nothing changed" }
:if ([:len $grp] = 1) do={
  :if ([/user/group get $grp comment] != "%[3]s") do={
    # Earlier setups created the group without a marker. Adopt it only when its
    # sole member is this setup's own account; otherwise it is someone else's.
    :local mmMembers [/user find where group="%[1]s"]
    :if ([:len $mmMembers] != 1 || [:len $account] != 1) do={ :error "STOP: a group named %[1]s exists and is not from Magic Hub; nothing changed" }
    :if ([/user get [:pick $mmMembers 0] name] != "%[1]s") do={ :error "STOP: a group named %[1]s exists and is not from Magic Hub; nothing changed" }
  }
}
`, p.user, len(marker)+1, marker)
}

func managementApply(id string, p pendingManagement) string {
	hash := sha256.Sum256([]byte(id))
	iface := fmt.Sprintf("mmh-%x", hash[:6])
	marker := "mega-magic:" + p.user
	// All interpolated values are generated locally as hex, fixed labels or RSA
	// authorized-key data. No operator text reaches the RouterOS commands.
	// /ip/service also lists live SSH connections, so the service is the first match.
	return fmt.Sprintf(`# APPLY (management): this enrollment's own account, key and access rules.
:local mmSSH [:pick [/ip/service find where name="ssh"] 0]
:local mmPort [/ip/service get $mmSSH port]
:if ([:len $grp] = 0) do={ /user/group add name="%[1]s" policy=ssh,read,write,policy,test,sensitive,reboot,ftp comment="%[3]s" } else={ /user/group set $grp policy=ssh,read,write,policy,test,sensitive,reboot,ftp comment="%[3]s" }
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
# Report only what this check observed. Data received DURING the check shows the tunnel works now;
# the cumulative counter alone does not. Management is confirmed separately by the app.
:put "Checking for traffic from Magic Hub (up to 30 seconds)..."
:local mmReached false
:local mmHubPeer [:pick [/interface/wireguard/peers find where interface="%[6]s"] 0]
:local mmRxStart 0
:do { :set mmRxStart [:tonum [/interface/wireguard/peers get $mmHubPeer rx]] } on-error={}
:for mmTry from=1 to=30 do={
  :if (!$mmReached) do={
    :delay 1s
    :do { :if ([:tonum [/interface/wireguard/peers get $mmHubPeer rx]] > $mmRxStart) do={ :set mmReached true } } on-error={}
  }
}
:if ($mmReached) do={ :put "TUNNEL ACTIVE: this router is receiving data from Magic Hub now. The app confirms router management by itself; that is a separate check shown in the app." } else={ :put "NOT CONNECTED YET: no data from Magic Hub during this check. Check this router's internet connection. Nothing more needs pasting: it connects by itself as soon as it can." }
:put "SAFE MODE: if you turned on Safe Mode (Ctrl+X) before pasting, press Ctrl+X again now to keep these changes. Closing WinBox or the terminal while Safe Mode is on undoes them."
`, p.user, len(marker)+1, marker, p.proof, p.public, iface)
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
