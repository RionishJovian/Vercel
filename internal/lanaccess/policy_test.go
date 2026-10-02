package lanaccess

import (
	"net/netip"
	"testing"
	"time"
)

func fixture() (Scope, Request, time.Time) {
	return Scope{BusinessID: "a", GatewayID: "gw", UserID: "owner", Role: "OWNER", Verified: true,
			LANs:      []netip.Prefix{netip.MustParsePrefix("192.168.88.0/24")},
			Protected: []netip.Prefix{netip.MustParsePrefix("192.168.88.1/32")}},
		Request{TargetIP: "192.168.88.20", Port: 5000, Duration: 15 * time.Minute}, time.Unix(1700000000, 0)
}

func TestCreateRejectsUnsafeTargets(t *testing.T) {
	for _, target := range []string{"", "host.local", "http://192.168.88.20", "192.168.88.20/24", "127.0.0.1", "169.254.169.254", "8.8.8.8", "0.0.0.0", "224.0.0.1", "192.168.89.20", "192.168.88.0", "192.168.88.255", "192.168.88.1", "::ffff:192.168.88.20"} {
		t.Run(target, func(t *testing.T) {
			s, r, now := fixture()
			r.TargetIP = target
			if _, err := Create(s, r, now); err == nil {
				t.Fatal("unsafe target accepted")
			}
		})
	}
}

func TestCreateRoleAndBounds(t *testing.T) {
	for _, role := range []string{"VIEWER", "TECHNICIAN", "SUPPORT", "OPERATOR", "", "owner"} {
		s, r, now := fixture()
		s.Role = role
		if _, err := Create(s, r, now); err == nil {
			t.Fatalf("role %s accepted", role)
		}
	}
	for _, port := range []int{-1, 0, 65536} {
		s, r, now := fixture()
		r.Port = port
		if _, err := Create(s, r, now); err == nil {
			t.Fatal("invalid port accepted")
		}
	}
	for _, duration := range []time.Duration{-1, 0, MaxDuration + 1} {
		s, r, now := fixture()
		r.Duration = duration
		if _, err := Create(s, r, now); err == nil {
			t.Fatal("invalid duration accepted")
		}
	}
	s, r, now := fixture()
	s.Verified = false
	if _, err := Create(s, r, now); err == nil {
		t.Fatal("unverified gateway accepted")
	}
	s, r, now = fixture()
	s.LANs = nil
	if _, err := Create(s, r, now); err == nil {
		t.Fatal("missing inventory accepted")
	}
	s, r, now = fixture()
	s.Protected = append(s.Protected, netip.Prefix{})
	if _, err := Create(s, r, now); err == nil {
		t.Fatal("invalid protected inventory accepted")
	}
}

func TestGrantRevalidatesScopeDestinationAndExpiry(t *testing.T) {
	s, r, now := fixture()
	g, err := Create(s, r, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Authorize(s, g.Target, g.Port, now); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name   string
		mutate func(*Scope, *Grant)
	}{
		{"other tenant same LAN", func(s *Scope, g *Grant) { s.BusinessID = "b" }},
		{"other gateway", func(s *Scope, g *Grant) { s.GatewayID = "other" }},
		{"other user", func(s *Scope, g *Grant) { s.UserID = "other" }},
		{"role changed", func(s *Scope, g *Grant) { s.Role = "VIEWER" }},
		{"gateway disconnected", func(s *Scope, g *Grant) { s.Verified = false }},
		{"revoked", func(s *Scope, g *Grant) { g.RevokedAt = now }},
		{"LAN removed", func(s *Scope, g *Grant) { s.LANs = nil }},
		{"missing platform denylist", func(s *Scope, g *Grant) { s.Protected = nil }},
		{"new protected target", func(s *Scope, g *Grant) { s.Protected = append(s.Protected, netip.MustParsePrefix("192.168.88.20/32")) }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			s, r, now := fixture()
			g, _ := Create(s, r, now)
			check.mutate(&s, &g)
			if err := g.Authorize(s, g.Target, g.Port, now); err == nil {
				t.Fatal("authorization should fail")
			}
		})
	}
	for _, at := range []time.Time{now.Add(-time.Second), g.ExpiresAt, g.ExpiresAt.Add(time.Second)} {
		if err := g.Authorize(s, g.Target, g.Port, at); err == nil {
			t.Fatal("invalid time accepted")
		}
	}
	if err := g.Authorize(s, netip.MustParseAddr("192.168.88.21"), g.Port, now); err == nil {
		t.Fatal("redirect destination accepted")
	}
	if err := g.Authorize(s, g.Target, 443, now); err == nil {
		t.Fatal("other port accepted")
	}
}
