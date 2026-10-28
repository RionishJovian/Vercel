package hub

import (
	"encoding/base64"
	"strings"
	"testing"
)

func scriptFixture() Peer {
	return Peer{RouterID: "gateway", RouterPublicKey: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("r", 32))), HubPublicKey: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("h", 32))), TunnelAddress: "10.77.0.2/32", Endpoint: "hub.example.com:51820", AllowedIPs: []string{"10.77.0.1/32"}, State: "active", EnrollmentVersion: 1}
}
func TestRouterScriptSafetyContract(t *testing.T) {
	s, err := BuildRouterScripts("gateway", scriptFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"private-key=", "/user/", "/ip/service", "/ip/firewall", "0.0.0.0/0", "/interface/wireguard remove", "/interface/wireguard/peers remove"} {
		if strings.Contains(s.Bootstrap+s.Setup, bad) {
			t.Fatal("unsafe setup operation", bad)
		}
	}
	for _, required := range []string{"public-key]", "comment]", "ambiguous"} {
		if !strings.Contains(s.Bootstrap, required) {
			t.Fatal("bootstrap lacks guard", required)
		}
	}
	for _, required := range []string{"router key differs", "conflicting Hub route", "/32", "persistent-keepalive=25s", "run step 1 first"} {
		if !strings.Contains(s.Setup, required) {
			t.Fatal("setup lacks guard", required)
		}
	}
	if strings.Contains(s.Disconnect, "/interface/wireguard remove") || !strings.Contains(s.Disconnect, "ownership mismatch") {
		t.Fatal("disconnect deletes identity or lacks ownership check")
	}
	again, _ := BuildRouterScripts("gateway", scriptFixture())
	if s != again {
		t.Fatal("retry changes artifacts")
	}
	other, _ := BootstrapScript("other-gateway")
	if other == s.Bootstrap {
		t.Fatal("gateway interfaces collide")
	}
}
func TestRouterScriptRejectsUnsafeAllocation(t *testing.T) {
	for _, change := range []func(*Peer){
		func(p *Peer) { p.Endpoint = "127.0.0.1:51820" },
		func(p *Peer) { p.Endpoint = "169.254.169.254:51820" },
		func(p *Peer) { p.RouterID = "foreign" }, func(p *Peer) { p.HubPublicKey = "bad" }, func(p *Peer) { p.RouterPublicKey = strings.Repeat("A", 43) + "=" },
		func(p *Peer) { p.Endpoint = `hub.example.com\";:51820` }, func(p *Peer) { p.Endpoint = "hub.example.com:0" }, func(p *Peer) { p.Endpoint = "hub.example.com:65536" },
		func(p *Peer) { p.TunnelAddress = "8.8.8.8/32" }, func(p *Peer) { p.TunnelAddress = "10.77.0.2/24" },
		func(p *Peer) { p.AllowedIPs = []string{"0.0.0.0/0"} }, func(p *Peer) { p.AllowedIPs = []string{"10.77.0.1/24"} }, func(p *Peer) { p.AllowedIPs = []string{"10.77.0.2/32"} }, func(p *Peer) { p.AllowedIPs = []string{"10.77.0.1/32", "10.77.0.1/32"} },
		func(p *Peer) { p.State = "disabled" }, func(p *Peer) { p.EnrollmentVersion = 0 },
	} {
		p := scriptFixture()
		change(&p)
		s, err := BuildRouterScripts("gateway", p)
		if err == nil || s != (RouterScripts{}) {
			t.Fatal("unsafe allocation produced script")
		}
	}
	if _, err := BootstrapScript(`gateway";`); err == nil {
		t.Fatal("identity script injection")
	}
}

func TestRouterScriptRepeatPasteUsesRouterOSArrayRendering(t *testing.T) {
	p := scriptFixture()
	p.AllowedIPs = []string{"10.77.0.1/32", "10.77.0.9/32"}
	s, err := BuildRouterScripts("gateway", p)
	if err != nil {
		t.Fatal(err)
	}
	// RouterOS :tostr arrays use semicolons; the write grammar still takes CSV.
	if !strings.Contains(s.Setup, `[:tostr [/interface/wireguard/peers get $peer allowed-address]] != "10.77.0.1/32;10.77.0.9/32"`) || !strings.Contains(s.Setup, "allowed-address=10.77.0.1/32,10.77.0.9/32") {
		t.Fatal("repeat-paste read validation uses the write serialization")
	}
}

func TestQuickConnectKeepsPrivateKeyOnlyInOnePasteScript(t *testing.T) {
	privateKey, publicKey, err := GenerateRouterKeypair()
	if err != nil {
		t.Fatal(err)
	}
	p := scriptFixture()
	p.RouterPublicKey = publicKey
	script, err := BuildQuickConnectScript("gateway", p, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, `private-key="`+privateKey+`"`) {
		t.Fatal("one-paste script does not install the router key")
	}
	if !strings.Contains(script, "router key differs") || !strings.Contains(script, "persistent-keepalive=25s") {
		t.Fatal("one-paste script lost setup guards")
	}
	if _, err := BuildQuickConnectScript("gateway", p, "not-a-key"); err == nil {
		t.Fatal("invalid private key accepted")
	}
	if strings.Contains(script, "step 1 first") || strings.Contains(script, "Magic Hub step 2") {
		t.Fatal("one-paste script still instructs a missing preparation step")
	}
	otherPrivate, _, err := GenerateRouterKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildQuickConnectScript("gateway", p, otherPrivate); err == nil {
		t.Fatal("mismatched private key accepted")
	}
}
