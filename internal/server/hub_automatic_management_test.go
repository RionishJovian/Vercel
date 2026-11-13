package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"mega-magic/internal/hub"
	"mega-magic/internal/provisioner/routeros"
)

func TestAutomaticRouterProof(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	p := pendingManagement{private: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), user: "mm-test", proof: strings.Repeat("a", 64), routerKey: "expected-wireguard-key"}
	for _, tc := range []struct {
		name, proof, key string
		success          bool
	}{
		{"valid", p.proof, p.routerKey, true},
		{"wrong proof", "wrong", p.routerKey, false},
		{"wrong router key", p.proof, "other-key", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
				if string(key.Marshal()) != string(signer.PublicKey().Marshal()) {
					t.Error("wrong authentication key")
				}
				return nil, nil
			}}
			cfg.AddHostKey(signer)
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				server, chans, reqs, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					channel, requests, err := ch.Accept()
					if err != nil {
						return
					}
					for req := range requests {
						if req.Type == "exec" {
							_ = req.Reply(true, nil)
							_, _ = channel.Write([]byte("mega-magic:" + p.user + ":" + tc.proof + "\n" + tc.key + "\nversion: 7.20\nboard-name: CHR\n"))
							_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
							_ = channel.Close()
							_ = server.Wait()
							return
						} else {
							_ = req.Reply(false, nil)
						}
					}
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			pin, err := verifyAutomaticRouter(ctx, &routeros.DeviceTarget{DeviceID: "router", Addr: listener.Addr().String()}, p)
			if (err == nil) != tc.success {
				t.Fatalf("success=%v, err=%v", tc.success, err)
			}
			if tc.success && pin != ssh.FingerprintSHA256(signer.PublicKey()) {
				t.Fatal("wrong host key pinned")
			}
			if !tc.success && pin != "" {
				t.Fatal("unverified host key escaped")
			}
			<-done
		})
	}
}

func TestAutomaticManagementScriptAndOutputBounds(t *testing.T) {
	p := pendingManagement{user: "mm-123", public: "ssh-rsa public", private: "PRIVATE MUST STAY IN APP", proof: strings.Repeat("b", 64)}
	script := automaticManagementScript("router", p)
	if strings.Contains(script, "%!") || strings.Contains(script, p.private) {
		t.Fatal("invalid script or private SSH key disclosure")
	}
	if !strings.Contains(script, "address=10.77.0.9/32") || !strings.Contains(script, "in-interface=\"mmh-") {
		t.Fatal("missing private management restrictions")
	}
	// RouterOS 7.25 warns about the old name when it is parsed, so the current name goes first
	// and the old one stays inside a string that runs only if the current one fails.
	if strings.Index(script, "available-from=") > strings.Index(script, `" address=" . $2`) || strings.Contains(script, "get $mmSSH address") {
		t.Fatal("the deprecated SSH allow-list name must be the fallback only")
	}
	// Finishing the script must not claim the router reached the Hub: it reports only data
	// received during the check (not the cumulative counter) and never says to close WinBox.
	start := strings.Index(script, ":set mmRxStart")
	checked := strings.Index(script, "> $mmRxStart")
	active := strings.Index(script, `:if ($mmReached) do={ :put "TUNNEL ACTIVE:`)
	if start < 0 || checked < start || active < checked || !strings.Contains(script, "NOT CONNECTED YET") {
		t.Fatal("setup must observe fresh traffic before reporting the tunnel active")
	}
	if strings.Contains(script, "close this window") || strings.Contains(script, "Setup complete") {
		t.Fatal("setup must not tell the operator to close the window or claim completion")
	}
	if !strings.Contains(script, "SAFE MODE: if you turned on Safe Mode (Ctrl+X) before pasting, press Ctrl+X again now") {
		t.Fatal("the Safe Mode commit instruction must always be printed")
	}
	// Setup never deletes other Magic Hub accounts, keys or rules (defect: broad mm-* cleanup).
	for _, broad := range []string{`name~"^mm-"`, "mmOldUser", `/user remove`, `/user/group remove`} {
		if strings.Contains(script, broad) {
			t.Fatalf("setup removes other accounts: %q", broad)
		}
	}
	// A group with our name is adopted only when proven ours, before any write.
	groupCheck := strings.Index(script, `"STOP: a group named mm-123 exists and is not from Magic Hub`)
	firstWrite := strings.Index(script, "/user/group add")
	if groupCheck < 0 || firstWrite < 0 || groupCheck > firstWrite || !strings.Contains(script, `comment="mega-magic:mm-123"`) {
		t.Fatal("group ownership must be proven before the first account write, and marked when created")
	}
	b := &boundedEnrollmentOutput{}
	if _, err := b.Write(make([]byte, 65536)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("x")); err == nil {
		t.Fatal("unbounded router response")
	}
}

func TestAutomaticEnrollmentRunsAsOneBlock(t *testing.T) {
	private, key, err := hub.GenerateRouterKeypair()
	if err != nil {
		t.Fatal(err)
	}
	peer := hub.Peer{RouterID: "router", RouterPublicKey: key, HubPublicKey: key,
		Endpoint: "hub.example.com:51820", AllowedIPs: []string{"10.77.0.9/32"},
		EnrollmentVersion: 1, State: "active", TunnelAddress: "10.77.0.2/32"}
	scripts, err := hub.BuildRouterScripts("router", peer)
	if err != nil {
		t.Fatal(err)
	}
	quick, err := hub.BuildQuickConnectScript("router", peer, private)
	if err != nil {
		t.Fatal(err)
	}
	for _, tunnel := range []string{scripts.Setup, quick} {
		p := pendingManagement{user: "mm-test", public: "ssh-rsa public", proof: strings.Repeat("b", 64), routerKey: key, tunnelScript: tunnel, sourceCIDR: "10.77.0.9/32"}
		combined, err := automaticEnrollmentScript("router", p)
		if err != nil {
			t.Fatal(err)
		}
		// There must be no second top-level block that a terminal can execute
		// after the first block reports a tunnel error.
		if strings.Contains(combined, "\n}\n{") || !strings.HasPrefix(combined, "{\n") {
			t.Fatal("management can execute independently of tunnel setup")
		}
		// Ending on a line break is what makes one paste run without another Enter.
		if !strings.HasSuffix(combined, "\n}\n") {
			t.Fatal("the script must run on paste")
		}
		// A user or group with our name that is not ours stops the script before ANY change,
		// including the tunnel's first write.
		tunnelWrite := strings.Index(combined, "\n# APPLY:")
		for _, guard := range []string{`"STOP: a user named`, `"STOP: a group named`} {
			at := strings.Index(combined, guard)
			if at < 0 || tunnelWrite < 0 || at > tunnelWrite {
				t.Fatalf("%s is checked after the first write", guard)
			}
		}
		if strings.Count(combined, "\n# APPLY:") != 1 || strings.Count(combined, "\n# APPLY (management)") != 1 {
			t.Fatal("expected one tunnel and one management apply section")
		}
		// The header must describe what the combined paste changes.
		if strings.Contains(combined, "Does not enable SSH/REST or change WAN/LAN/firewall/users") ||
			strings.Contains(combined, "Does not remove prior tunnels or change WAN/LAN/users") ||
			!strings.Contains(combined, "a dedicated management user and group (mm-...)") ||
			!strings.Contains(combined, "If the SSH service is disabled it is enabled for the Magic Hub address only") {
			t.Fatal("combined header misdescribes the changes")
		}
	}
	if _, err := automaticEnrollmentScript("router", pendingManagement{tunnelScript: ":error \"failed\""}); err == nil {
		t.Fatal("accepted tunnel without an execution block")
	}
}

func TestManagementSourceCIDR(t *testing.T) {
	for _, tc := range []struct {
		addresses []string
		want      string
	}{
		{[]string{"10.77.0.1/24"}, "10.77.0.1/32"},
		{[]string{"10.77.0.9/32", "fd00::9/64"}, "10.77.0.9/32"},
		{[]string{"10.77.0.1/24", "10.77.0.9/32"}, ""},
		{[]string{"8.8.8.8/32"}, ""},
		{nil, ""},
	} {
		var addresses []net.Addr
		for _, raw := range tc.addresses {
			ip, network, _ := net.ParseCIDR(raw)
			network.IP = ip
			addresses = append(addresses, network)
		}
		got, err := managementSourceCIDR(addresses)
		if got != tc.want || (err == nil) != (tc.want != "") {
			t.Fatalf("addresses=%v got=%s err=%v", tc.addresses, got, err)
		}
	}
	key := strings.Repeat("a", 64)
	p := pendingManagement{user: "mm-test", proof: key, sourceCIDR: "10.77.0.1/32", tunnelScript: "{\n:put \"check\"\n# APPLY: tunnel\n:put \"tunnel\"\n}"}
	script, err := automaticEnrollmentScript("router", p)
	if err != nil || !strings.Contains(script, "address=10.77.0.1/32") || strings.Contains(script, "10.77.0.9/32") {
		t.Fatal("source address did not follow current topology", err)
	}
	p.sourceCIDR = "10.77.0.0/24"
	if _, err := automaticEnrollmentScript("router", p); err == nil {
		t.Fatal("subnet-wide management accepted")
	}
}
