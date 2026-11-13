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

// Some connections have no working DNS, so the Hub hands out its IP address.
func TestQuickConnectUsesAnIPEndpointWithoutDNS(t *testing.T) {
	privateKey, publicKey, err := GenerateRouterKeypair()
	if err != nil {
		t.Fatal(err)
	}
	p := scriptFixture()
	p.RouterPublicKey = publicKey
	p.Endpoint = "203.0.113.10:51820"
	script, err := BuildQuickConnectScript("gateway", p, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, `endpoint-address="203.0.113.10" endpoint-port=51820`) {
		t.Fatal("script must connect to the IP address")
	}
	// A router set up with a name must move to the IP when the script is pasted again.
	if !strings.Contains(script, `endpoint-address] != "203.0.113.10"`) {
		t.Fatal("repeat paste must correct an older endpoint")
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
	if !strings.Contains(script, "belongs to another setup") || !strings.Contains(script, "persistent-keepalive=25s") {
		t.Fatal("one-paste script lost setup guards")
	}
	// Anything belonging to someone else must stop the script before the first change.
	firstWrite := strings.Index(script, "# APPLY: only add absent resources")
	if firstWrite < 0 { t.Fatal("missing explicit preflight/apply boundary") }
	for _, guard := range []string{"belongs to another setup", "multiple existing peers", "tunnel address already assigned elsewhere", "Hub destination route differs", "older Magic Hub tunnel found", "holds a key from an earlier setup. In the app, choose Start over"} {
		if at := strings.Index(script, guard); at < 0 || at > firstWrite {
			t.Fatalf("guard %q is missing or comes after the first change", guard)
		}
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

func TestQuickConnectDoesNotDeleteOrReplaceExistingTunnelResources(t *testing.T) {
	privateKey, publicKey, err := GenerateRouterKeypair()
	if err != nil { t.Fatal(err) }
	p := scriptFixture()
	p.RouterPublicKey = publicKey
	p.AllowedIPs = []string{"10.77.0.1/32", "10.77.0.9/32"}
	script, err := BuildQuickConnectScript("gateway", p, privateKey)
	if err != nil { t.Fatal(err) }
	for _, forbidden := range []string{
		"/interface/wireguard remove", "/interface/wireguard/peers remove",
		"/ip/address remove", "/ip/route remove",
		"/interface/wireguard set $mmIface private-key=",
		"/ip/route set $mmItem gateway=", "/ip/address set $mmItem interface=",
	} {
		if strings.Contains(script, forbidden) { t.Fatalf("quick paste performs destructive action %q", forbidden) }
	}
	for _, guard := range []string{
		"older Magic Hub tunnel found", "older Magic Hub route found",
		"existing Hub peer differs", "existing interface address differs",
		"tunnel address already assigned elsewhere", "Hub destination route differs",
	} {
		if !strings.Contains(script, guard) { t.Fatalf("missing non-destructive preflight guard: %q", guard) }
	}
	if !strings.Contains(script, "allowed-address=10.77.0.1/32,10.77.0.9/32") {
		t.Fatal("multi-address allowlist not preserved in new peer creation")
	}
}

func TestDisconnectScriptRejectsUnsafeManagementUser(t *testing.T) {
	good := ManagementUserName("biz", "gateway")
	if _, err := BuildDisconnectScript("gateway", scriptFixture(), good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "admin", "mm-xyz", `mm-0123456789ab"; /system reset-configuration; "`, "mm-0123456789AB", "mm-0123456789abc"} {
		if _, err := BuildDisconnectScript("gateway", scriptFixture(), bad); err == nil {
			t.Errorf("accepted unsafe management user %q", bad)
		}
	}
	if ManagementUserName("biz", "gateway") != good || ManagementUserName("biz2", "gateway") == good {
		t.Error("management user name must be stable per business and router")
	}
}

func TestStartOverRemovesOnlyThisGatewaysMarkedTunnel(t *testing.T) {
	privateKey, publicKey, err := GenerateRouterKeypair()
	if err != nil {
		t.Fatal(err)
	}
	p := scriptFixture()
	p.RouterPublicKey = publicKey
	script, err := BuildStartOverScript("gateway", p, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	name, marker, _ := scriptNames("gateway")
	apply := strings.Index(script, "\n# APPLY:")
	if apply < 0 {
		t.Fatal("missing preflight/apply boundary")
	}
	// Every removal is scoped to this gateway's exact marker.
	for _, line := range strings.Split(script, "\n") {
		if strings.Contains(line, " remove [") && !strings.Contains(line, `comment="`+marker+`"`) {
			t.Fatalf("unscoped removal: %s", line)
		}
	}
	for _, want := range []string{
		`/interface/wireguard/peers remove [find where interface="` + name + `" and comment="` + marker + `"]`,
		`/interface/wireguard set $mmIface private-key="` + privateKey + `"`,
	} {
		if at := strings.Index(script, want); at < apply {
			t.Fatalf("missing or before APPLY: %s", want)
		}
	}
	// Foreign or ambiguous things still stop it before the first write; a key from an
	// earlier attempt of THIS gateway does not.
	for _, guard := range []string{"interface belongs to another setup", "a peer on the Magic Hub interface belongs to another setup", "an address on the Magic Hub interface belongs to another setup", "tunnel address already assigned elsewhere", "older Magic Hub tunnel found", "older Magic Hub route found", "Hub destination route belongs to another setup"} {
		if at := strings.Index(script, guard); at < 0 || at > apply {
			t.Fatalf("guard %q missing or after the first write", guard)
		}
	}
	if strings.Contains(script[:apply], `do={ :error "STOP: this router holds a key from an earlier setup`) {
		t.Fatal("start over must not stop on this gateway's own earlier key")
	}
	if strings.Contains(script, "/interface/wireguard remove") || strings.Contains(script, "/user") {
		t.Fatal("start over must not delete the interface or touch users")
	}
	// The default paste keeps refusing the earlier key and never removes anything.
	quick, _ := BuildQuickConnectScript("gateway", p, privateKey)
	if strings.Contains(quick, " remove [") || !strings.Contains(quick, "choose Start over; nothing changed") {
		t.Fatal("default paste became destructive or lost the recovery hint")
	}
}

func TestResumeUsesTheRoutersOwnKeyAndStopsBeforeWriting(t *testing.T) {
	p := scriptFixture()
	script, err := BuildResumeScript("gateway", p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, "private-key") {
		t.Fatal("resume must never carry a private key")
	}
	apply := strings.Index(script, "\n# APPLY:")
	for _, guard := range []string{"does not hold the key from the earlier setup", "holds a different Magic Hub key"} {
		if at := strings.Index(script, guard); at < 0 || at > apply {
			t.Fatalf("guard %q missing or after the first write", guard)
		}
	}
	if strings.Contains(script, " remove [") {
		t.Fatal("resume removed something")
	}
}

func TestTwoGatewaysInOneTenantHaveSeparateIdentities(t *testing.T) {
	ifaceA, markerA, _ := scriptNames("11111111-1111-4111-8111-111111111111")
	ifaceB, markerB, _ := scriptNames("22222222-2222-4222-8222-222222222222")
	userA := ManagementUserName("tenant", "11111111-1111-4111-8111-111111111111")
	userB := ManagementUserName("tenant", "22222222-2222-4222-8222-222222222222")
	if ifaceA == ifaceB || markerA == markerB || userA == userB {
		t.Fatal("gateways of one tenant share router identities")
	}
	privB, pubB, _ := GenerateRouterKeypair()
	p := scriptFixture()
	p.RouterID, p.RouterPublicKey = "22222222-2222-4222-8222-222222222222", pubB
	script, err := BuildStartOverScript(p.RouterID, p, privB)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(script, ifaceA) || strings.Contains(script, markerA) || strings.Contains(script, userA) {
		t.Fatal("gateway B's recovery touches gateway A's identity")
	}
	disconnect, err := BuildDisconnectScript(p.RouterID, p, userB)
	if err != nil || strings.Contains(disconnect, markerA) || strings.Contains(disconnect, userA) {
		t.Fatal("gateway B's disconnect touches gateway A")
	}
}
