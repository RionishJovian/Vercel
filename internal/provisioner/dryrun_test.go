package provisioner

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// --- config validation ------------------------------------------------------

const validConfig = `
# synthetic valid provisioning artifact
/interface wireguard
add name=wg-magichub listen-port=13231
/interface wireguard peers
add allowed-address=10.255.0.1/32 interface=wg-magichub public-key="KW" persistent-keepalive=25s
/ip firewall filter
add action=accept chain=input protocol=udp dst-port=13231
add action=drop chain=input src-address-list=portal_bruteforce
`

func TestValidateProvisioningConfig_valid(t *testing.T) {
	issues := ValidateProvisioningConfig([]byte(validConfig))
	if HasErrors(issues) {
		t.Fatalf("valid config flagged errors: %+v", issues)
	}
}

func TestValidateProvisioningConfig_keepalive(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"missing", "add allowed-address=10.255.0.1/32 listen-port=13231", "keepalive_missing"},
		{"wrong", "persistent-keepalive=15s listen-port=13231", "keepalive_wrong"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := ValidateProvisioningConfig([]byte(tc.text))
			if !hasCode(issues, tc.want) {
				t.Fatalf("issues = %+v, want code %q", issues, tc.want)
			}
		})
	}
}

func TestValidateProvisioningConfig_broadAllowedAddress(t *testing.T) {
	text := validConfig + "\nadd allowed-address=0.0.0.0/0 interface=wg-magichub"
	issues := ValidateProvisioningConfig([]byte(text))
	if !hasCode(issues, "allowed_address_too_broad") {
		t.Fatalf("issues = %+v, want allowed_address_too_broad", issues)
	}
}

func TestValidateProvisioningConfig_firewallOrdering(t *testing.T) {
	dropFirst := `
persistent-keepalive=25s listen-port=13231 allowed-address=10.255.0.1/32
/ip firewall filter
add action=drop chain=input comment="CAPTIVE PORTAL GUARD"
add action=accept chain=input protocol=udp dst-port=13231
`
	issues := ValidateProvisioningConfig([]byte(dropFirst))
	if !hasCode(issues, "wg_accept_rule_missing") {
		t.Fatalf("issues = %+v, want wg_accept_rule_missing when drop precedes accept", issues)
	}
	if HasErrors(ValidateProvisioningConfig([]byte(validConfig))) {
		t.Fatal("accept-above-drop should validate clean")
	}
}

func TestValidateProvisioningConfig_listenPortMissing(t *testing.T) {
	text := "persistent-keepalive=25s allowed-address=10.255.0.1/32\nadd action=accept chain=input protocol=udp dst-port=13231"
	issues := ValidateProvisioningConfig([]byte(text))
	if !hasCode(issues, "listen_port_missing") {
		t.Fatalf("issues = %+v, want listen_port_missing", issues)
	}
}

func TestValidateProvisioningConfig_ignoresComments(t *testing.T) {
	// A commented-out drop must not shadow a real accept.
	text := validConfig + "\n# add action=drop chain=input"
	if HasErrors(ValidateProvisioningConfig([]byte(text))) {
		t.Fatal("commented drop line should be ignored")
	}
}

// --- recovery planning ------------------------------------------------------

func TestPlanRecovery_ladder(t *testing.T) {
	cases := []struct {
		name string
		obs  Observation
		want []string
	}{
		// PlanRecovery's contract is never-nil, so a healthy peer yields an empty
		// (not nil) slice.
		{"healthy up", Observation{State: StateUp, Keepalive: ExpectedKeepalive, KeepaliveKnown: true}, []string{}},
		{"up but keepalive unset", Observation{State: StateUp, KeepaliveKnown: true}, []string{ActionEnsureKeepalive}},
		{"stale pinhole", Observation{State: StateStale, Keepalive: ExpectedKeepalive, KeepaliveKnown: true}, []string{ActionReprobe, ActionResetPeer}},
		{"offline full ladder", Observation{State: StateOffline, Keepalive: ExpectedKeepalive, KeepaliveKnown: true}, []string{ActionReprobe, ActionResetPeer, ActionRotateListenPort, ActionEscalate}},
		{"unknown probes only", Observation{State: StateUnknown, Keepalive: ExpectedKeepalive, KeepaliveKnown: true}, []string{ActionReprobe}},
		{"empty state probes only", Observation{Keepalive: ExpectedKeepalive, KeepaliveKnown: true}, []string{ActionReprobe}},
		{"stale with keepalive fix prepended", Observation{State: StateStale, KeepaliveKnown: true}, []string{ActionEnsureKeepalive, ActionReprobe, ActionResetPeer}},
		// Unknown keepalive must not be invented as a defect.
		{"unknown keepalive is not guessed", Observation{State: StateStale}, []string{ActionReprobe, ActionResetPeer}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := actionsOf(PlanRecovery(tc.obs))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("actions = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlanRecovery_neverNilAndDeterministic(t *testing.T) {
	obs := Observation{PeerID: "p1", State: StateOffline}
	first := PlanRecovery(obs)
	if first == nil {
		t.Fatal("PlanRecovery returned nil")
	}
	second := PlanRecovery(obs)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("plan not deterministic:\n first=%+v\nsecond=%+v", first, second)
	}
}

func TestPlanRecovery_targetsAndRisk(t *testing.T) {
	changes := PlanRecovery(Observation{PeerID: "peer-9", State: StateOffline, Keepalive: ExpectedKeepalive, KeepaliveKnown: true})
	for _, c := range changes {
		if !strings.Contains(c.Target, "peer-9") && !strings.Contains(c.Target, "listener") {
			t.Fatalf("change %q target %q does not reference the peer", c.Action, c.Target)
		}
		if c.Risk == "" || c.Reason == "" {
			t.Fatalf("change %q missing risk/reason: %+v", c.Action, c)
		}
	}
}

// --- dry-run orchestration --------------------------------------------------

func TestDryRunAction_stalePeer(t *testing.T) {
	p := New(StaticProber{
		Obs:  Observation{State: StateStale, Keepalive: ExpectedKeepalive, KeepaliveKnown: true, DataPlaneUp: true},
		Conn: Connectivity{Checked: true, Reachable: false, Endpoint: "10.77.0.4:443", Detail: "no reply"},
	}).WithFileReader(func(path string) ([]byte, error) { return []byte(validConfig), nil })

	rep, err := p.DryRunAction(context.Background(), "peer-1", "provisioning/x.rsc")
	if err != nil {
		t.Fatalf("DryRunAction: %v", err)
	}
	if !rep.DryRun || rep.WritesToProd {
		t.Fatalf("report must be dry-run with no prod writes: %+v", rep)
	}
	if !rep.ConfigValid {
		t.Fatalf("config should be valid: %+v", rep.Issues)
	}
	if rep.Connectivity.Checked != true || rep.Connectivity.Reachable {
		t.Fatalf("connectivity not plumbed: %+v", rep.Connectivity)
	}
	if got := actionsOf(rep.Changes); !reflect.DeepEqual(got, []string{ActionReprobe, ActionResetPeer}) {
		t.Fatalf("changes = %v, want [reprobe reset_peer]", got)
	}
	if rep.PeerID != "peer-1" || rep.ConfigPath != "provisioning/x.rsc" {
		t.Fatalf("report identity wrong: %+v", rep)
	}
}

func TestDryRunAction_invalidConfigPropagates(t *testing.T) {
	p := New(StaticProber{Obs: Observation{State: StateUp, Keepalive: ExpectedKeepalive}}).
		WithFileReader(func(string) ([]byte, error) { return []byte("add listen-port=13231"), nil })

	rep, err := p.DryRunAction(context.Background(), "peer-2", "cfg.rsc")
	if err != nil {
		t.Fatalf("DryRunAction: %v", err)
	}
	if rep.ConfigValid {
		t.Fatal("config should be invalid")
	}
	if !HasErrors(rep.Issues) {
		t.Fatalf("expected error issues, got %+v", rep.Issues)
	}
}

func TestDryRunAction_requiresPeerAndReadsOnce(t *testing.T) {
	if _, err := New(nil).DryRunAction(context.Background(), "", "cfg.rsc"); err == nil {
		t.Fatal("empty peer id should error")
	}

	var calls []string
	p := New(StaticProber{Obs: Observation{State: StateUp}}).
		WithFileReader(func(path string) ([]byte, error) {
			calls = append(calls, path)
			return []byte(validConfig), nil
		})
	if _, err := p.DryRunAction(context.Background(), "peer-3", "the-config.rsc"); err != nil {
		t.Fatalf("DryRunAction: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"the-config.rsc"}) {
		t.Fatalf("config read calls = %v, want exactly one read", calls)
	}
}

func TestDryRunAction_readError(t *testing.T) {
	wantErr := errors.New("boom")
	p := New(StaticProber{}).WithFileReader(func(string) ([]byte, error) { return nil, wantErr })
	if _, err := p.DryRunAction(context.Background(), "peer-4", "missing.rsc"); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestDryRunAction_probeError(t *testing.T) {
	p := New(errProber{}).WithFileReader(func(string) ([]byte, error) { return []byte(validConfig), nil })
	if _, err := p.DryRunAction(context.Background(), "peer-5", "cfg.rsc"); err == nil {
		t.Fatal("probe error should propagate")
	}
}

// --- io helpers -------------------------------------------------------------

func TestResolveEndpoint(t *testing.T) {
	cases := []struct {
		peer, addr string
		want       string
		ok         bool
	}{
		{"peer-uuid", "10.77.0.4", "10.77.0.4:443", true},
		{"peer-uuid", "10.77.0.4:8443", "10.77.0.4:8443", true},
		{"10.77.0.4", "", "10.77.0.4:443", true},
		{"10.77.0.4:443", "", "10.77.0.4:443", true},
		{"peer-uuid", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		got, ok := ResolveEndpoint(tc.peer, tc.addr)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("ResolveEndpoint(%q,%q) = (%q,%v), want (%q,%v)", tc.peer, tc.addr, got, ok, tc.want, tc.ok)
		}
	}
}

func TestUncheckedConnectivity_isNotSilent(t *testing.T) {
	c := UncheckedConnectivity("peer-uuid")
	if c.Checked {
		t.Fatal("unchecked connectivity must not claim Checked")
	}
	if !strings.Contains(c.Detail, "peer-uuid") || !strings.Contains(c.Detail, "--peer-addr") {
		t.Fatalf("detail should explain the gap and the fix: %q", c.Detail)
	}
}

func TestTCPProber_readOnlyDial(t *testing.T) {
	// No listener: the probe must return an offline observation, a checked
	// connectivity record, and no error (a failed dial is evidence, not a crash).
	pr := TCPProber{Endpoint: "127.0.0.1:1", Timeout: 200 * time.Millisecond}
	obs, conn, err := pr.Observe(context.Background(), "peer-6")
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if !conn.Checked || conn.Reachable {
		t.Fatalf("expected checked+unreachable, got %+v", conn)
	}
	if obs.State != StateOffline {
		t.Fatalf("state = %q, want offline", obs.State)
	}
}

// --- helpers ----------------------------------------------------------------

type errProber struct{}

func (errProber) Observe(context.Context, string) (Observation, Connectivity, error) {
	return Observation{}, Connectivity{}, errors.New("probe down")
}

func actionsOf(changes []Change) []string {
	if changes == nil {
		return nil
	}
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, c.Action)
	}
	return out
}

func hasCode(issues []Issue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}
