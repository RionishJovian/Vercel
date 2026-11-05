package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mega-magic/internal/hub"
)

// TestCHRLabOnePaste runs the real generated scripts on a disposable RouterOS CHR.
// It is skipped unless MM_CHR_SSH=admin@host and MM_CHR_KEY=path are set.
type labRouter struct {
	t           *testing.T
	target, key string
}

func (r labRouter) sshArgs(extra ...string) []string {
	return append([]string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "LogLevel=ERROR", "-o", "BatchMode=yes", "-o", "ConnectTimeout=20", "-i", r.key}, extra...)
}

func (r labRouter) run(command string) string {
	r.t.Helper()
	var out []byte
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		out, err = exec.Command("ssh", r.sshArgs(r.target, command)...).CombinedOutput()
		if err == nil {
			break
		}
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		r.t.Fatalf("ssh %q: %v %s", command, err, out)
	}
	return strings.TrimSpace(string(out))
}

// paste uploads a script and runs it with no SSH session open, as in a WinBox terminal.
func (r labRouter) paste(script string) string {
	r.t.Helper()
	dir := r.t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lab-paste.rsc"), []byte(script), 0o600); err != nil {
		r.t.Fatal(err)
	}
	if out, err := exec.Command("scp", r.sshArgs("-O", filepath.Join(dir, "lab-paste.rsc"), r.target+":/lab-paste.rsc")...).CombinedOutput(); err != nil {
		r.t.Fatalf("scp: %v %s", err, out)
	}
	r.run(`:execute script="{:delay 6s; /import file-name=lab-paste.rsc verbose=no}" file=lab-paste-out`)
	time.Sleep(25 * time.Second)
	out := r.run(`:put [/file get [find where name~"lab-paste-out"] contents]`)
	r.run(`/file remove [find where name~"lab-paste"]`)
	return out
}

func (r labRouter) pasteWithSSHOpen(script string) string {
	r.t.Helper()
	dir := r.t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lab-paste.rsc"), []byte(script), 0o600); err != nil {
		r.t.Fatal(err)
	}
	if out, err := exec.Command("scp", r.sshArgs("-O", filepath.Join(dir, "lab-paste.rsc"), r.target+":/lab-paste.rsc")...).CombinedOutput(); err != nil {
		r.t.Fatalf("scp: %v %s", err, out)
	}
	out := r.run(`/import file-name=lab-paste.rsc verbose=no`)
	r.run(`/file remove [find where name~"lab-paste"]`)
	return out
}

func (r labRouter) count(command string) string {
	r.t.Helper()
	fields := strings.Fields(r.run(`:put [:len [` + command + `]]`))
	return fields[len(fields)-1]
}

func (r labRouter) clean() {
	r.t.Helper()
	r.run(`/ip firewall filter remove [find where comment~"^mega-magic:mm-"]; /ip firewall nat remove [find where comment~"^mega-magic:mm-"]; /user/ssh-keys remove [find where user~"^mm-"]; /user remove [find where name~"^mm-"]; /user group remove [find where name~"^mm-"]; /ip route remove [find where comment~"^mega-magic:mmh-"]; /ip address remove [find where comment~"^mega-magic:mmh-"]; /interface wireguard peers remove [find where comment~"^mega-magic:mmh-"]; /interface wireguard remove [find where name~"^mmh-"]`)
}

func (r labRouter) installed(t *testing.T, device string) {
	t.Helper()
	name, _ := labNames(device)
	for what, command := range map[string]string{
		"interface":     `/interface/wireguard find where name="` + name + `"`,
		"peer":          `/interface/wireguard/peers find where interface="` + name + `"`,
		"address":       `/ip/address find where interface="` + name + `"`,
		"route":         `/ip/route find where comment="mega-magic:` + name + `"`,
		"firewall rule": `/ip/firewall/filter find where in-interface="` + name + `" and comment~"^mega-magic:mm-"`,
	} {
		if got := r.count(command); got != "1" {
			t.Errorf("%s: want 1, got %s", what, got)
		}
	}
	if got := r.count(`/user find where name~"^mm-"`); got != "1" {
		t.Errorf("management users: want 1, got %s", got)
	}
	if got := r.count(`/user/ssh-keys find where user~"^mm-"`); got != "1" {
		t.Errorf("SSH keys: want 1, got %s", got)
	}
	if got := r.count(`/interface/wireguard find where name~"^mmh-"`); got != "1" {
		t.Errorf("tunnels: want 1, got %s", got)
	}
}

func labNames(device string) (string, string) {
	script, _ := hub.BootstrapScript(device)
	at := strings.Index(script, `name="`) + 6
	name := script[at : at+strings.Index(script[at:], `"`)]
	return name, "mega-magic:" + name
}

const labBusiness = "99999999-9999-4999-8999-999999999999"

func labScript(t *testing.T, device, tunnel string) (string, hub.Peer) {
	t.Helper()
	private, public, err := hub.GenerateRouterKeypair()
	if err != nil {
		t.Fatal(err)
	}
	_, hubPublic, _ := hub.GenerateRouterKeypair()
	peer := hub.Peer{PeerID: "lab", TenantID: labBusiness, RouterID: device, RouterPublicKey: public, TunnelAddress: tunnel, HubPublicKey: hubPublic, Endpoint: "megamagic.tech:51820", AllowedIPs: []string{"10.77.0.1/32"}, State: "active", EnrollmentVersion: 1}
	quick, err := hub.BuildQuickConnectScript(device, peer, private)
	if err != nil {
		t.Fatal(err)
	}
	p, err := newPendingManagement(labBusiness, device, public, quick, "10.77.0.1/32")
	if err != nil {
		t.Fatal(err)
	}
	script, err := automaticEnrollmentScript(device, p)
	if err != nil {
		t.Fatal(err)
	}
	return script, peer
}

func TestCHRLabOnePaste(t *testing.T) {
	target := os.Getenv("MM_CHR_SSH")
	if target == "" {
		t.Skip("set MM_CHR_SSH and MM_CHR_KEY to run against a disposable CHR")
	}
	r := labRouter{t: t, target: target, key: os.Getenv("MM_CHR_KEY")}
	const a, b = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	r.clean()
	t.Cleanup(func() {
		r.run(`/ip/service set [:pick [/ip/service find where name="ssh"] 0] port=22 address=""`)
		r.clean()
	})
	mustComplete := func(t *testing.T, out string) {
		t.Helper()
		if !strings.Contains(out, "Setup complete") || strings.Contains(out, "STOP") || strings.Contains(out, "rror") {
			t.Fatalf("paste did not complete:\n%s", out)
		}
	}
	scriptA, peerA := labScript(t, a, "10.77.9.50/32")

	t.Run("first paste", func(t *testing.T) {
		mustComplete(t, r.paste(scriptA))
		r.installed(t, a)
	})
	t.Run("same script again", func(t *testing.T) {
		mustComplete(t, r.paste(scriptA))
		r.installed(t, a)
	})
	t.Run("again with an SSH connection open", func(t *testing.T) {
		mustComplete(t, r.pasteWithSSHOpen(scriptA))
		r.installed(t, a)
	})
	t.Run("a new script with a new key on the same record", func(t *testing.T) {
		newer, _ := labScript(t, a, "10.77.9.52/32")
		mustComplete(t, r.paste(newer))
		r.installed(t, a)
		name, _ := labNames(a)
		if got := r.run(`:put [/ip/address get [find where interface="` + name + `"] address]`); !strings.Contains(got, "10.77.9.52/32") {
			t.Errorf("address not corrected: %s", got)
		}
		mustComplete(t, r.paste(scriptA))
	})
	t.Run("custom SSH port and a restricted SSH service", func(t *testing.T) {
		r.run(`/ip/service set [:pick [/ip/service find where name="ssh"] 0] address=192.168.64.0/24,172.16.0.0/12`)
		mustComplete(t, r.paste(scriptA))
		got := r.run(`:put [:tostr [/ip/service get [:pick [/ip/service find where name="ssh"] 0] available-from]]`)
		for _, want := range []string{"192.168.64.0/24", "172.16.0.0/12", "10.77.0.1"} {
			if !strings.Contains(got, want) {
				t.Errorf("SSH service addresses lost %s: %s", want, got)
			}
		}
		// Change the port last: the test itself reaches the router over SSH.
		r.run(`:execute script="{:delay 2s; /ip/service set [:pick [/ip/service find where name=\"ssh\"] 0] port=2222}"`)
		time.Sleep(5 * time.Second)
		restore := exec.Command("ssh", r.sshArgs("-p", "2222", target, `/ip/service set [:pick [/ip/service find where name="ssh"] 0] port=22 address=""`)...)
		if out, err := restore.CombinedOutput(); err != nil {
			t.Fatalf("could not reach the router on the custom port: %v %s", err, out)
		}
		time.Sleep(2 * time.Second)
	})
	t.Run("custom SSH port gets a redirect and its own firewall port", func(t *testing.T) {
		name, _ := labNames(a)
		script := strings.ReplaceAll(scriptA, `:local mmPort [/ip/service get $mmSSH port]`, `:local mmPort 2222`)
		mustComplete(t, r.paste(script))
		// RouterOS does not match ports in "where", so read the values.
		user := hub.ManagementUserName(labBusiness, a)
		got := r.run(`:local n [/ip/firewall/nat find where comment="mega-magic:` + user + `"]; :put ([/ip/firewall/nat get $n action] . " " . [/ip/firewall/nat get $n to-ports] . " " . [/ip/firewall/nat get $n dst-port] . " " . [/ip/firewall/nat get $n in-interface])`)
		if want := "redirect 2222 22 " + name; !strings.Contains(got, want) {
			t.Errorf("redirect: want %q, got %q", want, got)
		}
		got = r.run(`:put [/ip/firewall/filter get [find where comment="mega-magic:` + user + `"] dst-port]`)
		if !strings.Contains(got, "2222") {
			t.Errorf("firewall rule on the custom port: got %q", got)
		}
		mustComplete(t, r.paste(scriptA))
		if got := r.count(`/ip/firewall/nat find where in-interface="` + name + `"`); got != "0" {
			t.Errorf("redirect left behind after going back to port 22: %s", got)
		}
	})
	t.Run("firewall rule is above the static drop rules", func(t *testing.T) {
		// Spares the test's own address: a plain drop-all would lock the test out of the router.
		r.run(`/ip/firewall/filter add chain=input action=drop src-address=!192.168.64.0/24 comment=lab-drop-all`)
		defer r.run(`/ip/firewall/filter remove [find where comment=lab-drop-all]`)
		mustComplete(t, r.paste(scriptA))
		order := r.run(`:foreach i in=[/ip/firewall/filter find where dynamic=no] do={ :put [/ip/firewall/filter get $i comment] }`)
		if !strings.HasPrefix(order, "mega-magic:mm-") {
			t.Errorf("management rule is not first among static rules:\n%s", order)
		}
	})
	t.Run("record deleted, new record on the same router", func(t *testing.T) {
		scriptB, _ := labScript(t, b, "10.77.9.51/32")
		mustComplete(t, r.paste(scriptB))
		r.installed(t, b)
	})
	t.Run("something else using the Hub route stops before any change", func(t *testing.T) {
		r.clean()
		r.run(`/ip/route add dst-address=10.77.0.1/32 gateway=192.168.64.1 comment=not-ours`)
		defer r.run(`/ip/route remove [find where comment=not-ours]`)
		out := r.paste(scriptA)
		if !strings.Contains(out, "STOP: a route to the Hub already exists") {
			t.Fatalf("want a STOP, got:\n%s", out)
		}
		if got := r.count(`/interface/wireguard find where name~"^mmh-"`); got != "0" {
			t.Errorf("a tunnel was created before the STOP")
		}
	})
	t.Run("a user with our name that is not ours stops", func(t *testing.T) {
		user := hub.ManagementUserName(labBusiness, a)
		r.run(fmt.Sprintf(`/user add name=%s group=read password=lab-Passw0rd-x comment=not-ours`, user))
		defer r.run(fmt.Sprintf(`/user remove [find where name=%s]`, user))
		if out := r.paste(scriptA); !strings.Contains(out, "is not from Magic Hub") {
			t.Fatalf("want a STOP, got:\n%s", out)
		}
		r.cleanTunnel()
	})
	t.Run("cleanup script removes everything and can be pasted again", func(t *testing.T) {
		r.clean()
		mustComplete(t, r.paste(scriptA))
		cleanup, err := hub.BuildDisconnectScript(a, peerA, hub.ManagementUserName(labBusiness, a))
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if out := r.paste(cleanup); !strings.Contains(out, "Magic Hub setup removed") || strings.Contains(out, "rror") {
				t.Fatalf("cleanup run %d:\n%s", i+1, out)
			}
		}
		for what, command := range map[string]string{
			"tunnel": `/interface/wireguard find where name~"^mmh-"`, "user": `/user find where name~"^mm-"`, "group": `/user/group find where name~"^mm-"`,
			"route": `/ip/route find where comment~"^mega-magic:"`, "address": `/ip/address find where comment~"^mega-magic:"`, "firewall": `/ip/firewall/filter find where comment~"^mega-magic:"`,
		} {
			if got := r.count(command); got != "0" {
				t.Errorf("%s left behind: %s", what, got)
			}
		}
	})
}

// cleanTunnel removes the tunnel a stopped paste installed before reaching the user check.
func (r labRouter) cleanTunnel() {
	r.t.Helper()
	r.run(`/ip route remove [find where comment~"^mega-magic:mmh-"]; /ip address remove [find where comment~"^mega-magic:mmh-"]; /interface wireguard peers remove [find where comment~"^mega-magic:mmh-"]; /interface wireguard remove [find where name~"^mmh-"]`)
}
