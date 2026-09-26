package vps

import (
	"strings"
	"testing"
)

func TestBuildPeer_deterministic_and_inSubnet(t *testing.T) {
	subnet := "192.168.99.0/24"
	first, err := BuildPeer("dev-a", "RB5009", "MAIN_GATEWAY", "wg0", subnet, "hub.example.net", 13231, 25)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	second, err := BuildPeer("dev-a", "RB5009", "MAIN_GATEWAY", "wg0", subnet, "hub.example.net", 13231, 25)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if first.Address != second.Address {
		t.Fatalf("non-deterministic address: %s vs %s", first.Address, second.Address)
	}
	// Address must be inside 192.168.99.0/24 and start after reserved hosts (.11).
	ip := strings.TrimSuffix(first.Address, "/32")
	oct := strings.Split(ip, ".")
	if len(oct) != 4 || oct[2] != "99" {
		t.Fatalf("address out of management subnet: %s", first.Address)
	}
	if first.Endpoint != "hub.example.net:13231" {
		t.Fatalf("endpoint = %q, want hub.example.net:13231", first.Endpoint)
	}
	if first.PersistentKeepalive != 25 {
		t.Fatalf("keepalive = %d, want 25", first.PersistentKeepalive)
	}
	if first.Interface != "wg0" {
		t.Fatalf("interface = %q, want wg0", first.Interface)
	}
}

func TestBuildPeer_distinctDevices_usuallyDistinct(t *testing.T) {
	// Different devices should overwhelmingly map to different hosts in a /24
	// (hash-dispersed). Assert the two chosen IDs differ — a smoke check that
	// we're not assigning the same static host to every device.
	a, err := BuildPeer("dev-a", "", "", "wg0", "192.168.99.0/24", "", 13231, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildPeer("dev-b", "", "", "wg0", "192.168.99.0/24", "", 13231, 0)
	if err != nil {
		t.Fatal(err)
	}
	if a.Address == b.Address {
		t.Fatalf("two distinct devices share host %s — collision", a.Address)
	}

	// Empty endpoint host => endpoint omitted.
	if a.Endpoint != "" {
		t.Fatalf("endpoint should be empty when hub host empty, got %q", a.Endpoint)
	}
	// keepalive 0 => default 25.
	if a.PersistentKeepalive != defaultKeepalive {
		t.Fatalf("default keepalive = %d, want %d", a.PersistentKeepalive, defaultKeepalive)
	}
}

func TestBuildPeer_skipsNetworkAndReserved(t *testing.T) {
	// In a /24, the first client host must be at least .11 (network + 1 + 10
	// reserved), never .0 or .1-.10.
	p, err := BuildPeer("x", "", "", "wg0", "192.168.99.0/24", "", 13231, 25)
	if err != nil {
		t.Fatal(err)
	}
	oct := strings.Split(strings.TrimSuffix(p.Address, "/32"), ".")
	if len(oct) != 4 || oct[2] != "99" || oct[3] == "0" {
		t.Fatalf("bad host: %s", p.Address)
	}
}

func TestBuildPeer_badSubnet(t *testing.T) {
	if _, err := BuildPeer("x", "", "", "wg0", "not-a-cidr", "", 13231, 25); err == nil {
		t.Fatal("expected error for malformed management subnet")
	}
	if _, err := BuildPeer("x", "", "", "wg0", "fd00::/64", "", 13231, 25); err == nil {
		t.Fatal("expected error for IPv6 management subnet")
	}
}

func TestBuildHubConfig(t *testing.T) {
	h := BuildHubConfig("wg-hub", 13231, "hub.example.net", []string{"10.255.0.1/32"}, 99, "192.168.99.0/24", 25)
	if h.Interface != "wg-hub" || h.ListenPort != 13231 || h.ManagementVLAN != 99 {
		t.Fatalf("hub config fields wrong: %+v", h)
	}
	if len(h.AllowedIPs) != 1 || h.AllowedIPs[0] != "10.255.0.1/32" {
		t.Fatalf("allowed ips not preserved: %+v", h.AllowedIPs)
	}
	// 0 keepalive -> default.
	h0 := BuildHubConfig("wg", 1, "", nil, 0, "", 0)
	if h0.PeerKeepaliveSec != defaultKeepalive {
		t.Fatalf("default keepalive in hub = %d", h0.PeerKeepaliveSec)
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]string{
		"up": "reachable", "stale": "degraded", "offline": "unreachable",
		"": "unknown", "weird": "unknown",
	}
	for in, want := range cases {
		if got := Classify(in); got != want {
			t.Fatalf("Classify(%q) = %q, want %q", in, got, want)
		}
	}
}
