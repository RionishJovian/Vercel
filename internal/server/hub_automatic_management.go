package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/provisioner/routeros"
)

type pendingManagement struct {
	private, public, proof, user, routerKey string
	tunnelScript                            string
	sourceCIDR                              string
	expires                                 time.Time
}

// Pending keys live for 30 minutes in memory, never in plaintext database rows.
// A restart requires another setup script. Verified keys use the existing
// encrypted credential store and pinned transport, including after restarts.
func configureAutomaticHubManagement(h *handler.VPSHandler, source *database.DeviceCredentialSource, cidrs []string, interfaceName string) {
	var mu sync.Mutex
	pending := make(map[string]pendingManagement)
	h.ConfigureAutomaticManagement(func(ctx context.Context, biz, id, routerKey, tunnelScript string) (string, error) {
		iface, err := net.InterfaceByName(interfaceName)
		if err != nil {
			return "", errors.New("management tunnel interface unavailable")
		}
		addresses, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		sourceCIDR, err := managementSourceCIDR(addresses)
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
			return "", errors.New("management already configured; use existing connection")
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
		if len(pending) >= 256 {
			return "", errors.New("too many pending enrollments")
		}
		private, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return "", err
		}
		pub, err := ssh.NewPublicKey(&private.PublicKey)
		if err != nil {
			return "", err
		}
		proof := make([]byte, 32)
		if _, err := rand.Read(proof); err != nil {
			return "", err
		}
		hash := sha256.Sum256([]byte(biz + "/" + id))
		p := pendingManagement{private: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(private)})), public: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))), proof: hex.EncodeToString(proof), user: fmt.Sprintf("mm-%x", hash[:6]), routerKey: routerKey, expires: time.Now().Add(30 * time.Minute)}
		p.tunnelScript = tunnelScript
		p.sourceCIDR = sourceCIDR
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
			return err
		}
		p, ok := pending[key]
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
	management := strings.TrimSpace(automaticManagementScript(id, p))
	if !strings.HasPrefix(tunnel, "{\n") || !strings.HasSuffix(tunnel, "\n}") {
		return "", errors.New("invalid tunnel execution block")
	}
	management = strings.ReplaceAll(management, "10.77.0.9/32", p.sourceCIDR)
	return strings.TrimSuffix(tunnel, "\n}") + "\n" + strings.TrimPrefix(management, "{\n"), nil
}

func automaticManagementScript(id string, p pendingManagement) string {
	hash := sha256.Sum256([]byte(id))
	iface := fmt.Sprintf("mmh-%x", hash[:6])
	marker := "mega-magic:" + p.user
	// All interpolated values are generated locally as hex, fixed labels or RSA
	// authorized-key data. No operator text reaches the RouterOS commands.
	return fmt.Sprintf(`{
# Dedicated Magic Hub management. Run within 30 minutes of generation.
:local managementIface [/interface/wireguard find where name="%s"]
:if ([:len $managementIface] != 1) do={ :error "STOP: Magic Hub tunnel missing or ambiguous; management unchanged" }
:if ([/interface/wireguard get $managementIface comment] != "mega-magic:%s") do={ :error "STOP: Magic Hub tunnel ownership mismatch; management unchanged" }
:if ([/interface/wireguard get $managementIface public-key] != "%s" || [/interface/wireguard get $managementIface disabled]) do={ :error "STOP: Magic Hub tunnel key mismatch or disabled; management unchanged" }
:if ([/ip/service get [find where name="ssh"] port] != 22) do={ :error "STOP: custom SSH port; use advanced management setup" }
:local account [/user find where name="%s"]
:if ([:len $account] > 1) do={ :error "STOP: ambiguous management account" }
:if ([:len $account] = 1) do={
  :if ([:pick [/user get $account comment] 0 %d] != "%s:") do={ :error "STOP: existing account is not owned by Magic Hub" }
}
:local grp [/user/group find where name="%s"]
:if ([:len $grp] = 0) do={ /user/group add name="%s" policy=ssh,read,write,policy,test,sensitive,reboot,ftp }
:if ([:len $grp] = 1) do={
  :foreach privilege in=[/user/group get $grp policy] do={
    :if ([:find ",ssh,read,write,policy,test,sensitive,reboot,ftp," ("," . $privilege . ",")] = nil) do={ :error "STOP: dedicated management group has unexpected permissions" }
  }
}
:if ([:len $account] = 0) do={ /user add name="%s" group="%s" address=10.77.0.9/32 password="%s" comment="%s:%s" }
:if ([:len $account] = 1) do={ /user set $account group="%s" address=10.77.0.9/32 password="%s" comment="%s:%s" disabled=no }
:local keyfile "%s.pub.txt"
:if ([:len [/file find where name=$keyfile]] = 0) do={ /file print file="%s.pub"; :delay 1s }
/file set [find where name=$keyfile] contents="%s"
/user/ssh-keys remove [find where user="%s"]
/user/ssh-keys import public-key-file=$keyfile user="%s"
/file remove [find where name=$keyfile]
:local sshservice [/ip/service find where name="ssh"]
:if ([/ip/service get $sshservice disabled]) do={
  /ip/service set $sshservice address=10.77.0.9/32 disabled=no
} else={
  :local allowed [:tostr [/ip/service get $sshservice address]]
  :if ([:len $allowed] > 0 && [:find $allowed "10.77.0.9/32"] = nil) do={ /ip/service set $sshservice address=($allowed . ",10.77.0.9/32") }
}
:local rule [/ip/firewall/filter find where comment="%s"]
:if ([:len $rule] > 1) do={ :error "STOP: ambiguous management rule" }
:if ([:len $rule] = 1) do={
  :if ([/ip/firewall/filter get $rule chain] != "input" || [/ip/firewall/filter get $rule action] != "accept" || [/ip/firewall/filter get $rule in-interface] != "%s" || [:tostr [/ip/firewall/filter get $rule src-address]] != "10.77.0.9/32" || [:tostr [/ip/firewall/filter get $rule dst-port]] != "22" || [/ip/firewall/filter get $rule protocol] != "tcp") do={ :error "STOP: management firewall rule changed" }
  /ip/firewall/filter enable $rule
  /ip/firewall/filter move $rule destination=0
} else={
  :if ([:len [/ip/firewall/filter find]] = 0) do={ /ip/firewall/filter add chain=input action=accept in-interface="%s" src-address=10.77.0.9/32 protocol=tcp dst-port=22 comment="%s" } else={ /ip/firewall/filter add chain=input action=accept in-interface="%s" src-address=10.77.0.9/32 protocol=tcp dst-port=22 comment="%s" place-before=0 }
}
:put "Setup complete. Return to Mega Magic and select Finish connection."
}`, iface, iface, p.routerKey, p.user, len(marker)+1, marker, p.user, p.user, p.user, p.user, p.proof, marker, p.proof, p.user, p.proof, marker, p.proof, p.user, p.user, p.public, p.user, p.user, marker, iface, iface, marker, iface, marker)
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
