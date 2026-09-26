package routeros

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mega-magic/internal/provisioner"
)

func TestTranslate_enforceGuestQuota(t *testing.T) {
	a := NewApplier(&fakeRunner{}, "wg-magichub", 13241, 0)
	steps, err := a.Translate(provisioner.Change{
		Action:   provisioner.ActionEnforceGuestQuota,
		Target:   "AA:BB:CC:DD:EE:FF",
		Proposed: `{"data_bytes":1073741824,"uptime_seconds":3600}`,
	})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	var cmds []string
	for _, s := range steps {
		if s.Command != "" {
			cmds = append(cmds, s.Command)
		}
	}
	joined := strings.Join(cmds, "\n")
	for _, want := range []string{
		"/ip hotspot user add name=mm-AA-BB-CC-DD-EE-FF",
		"limit-uptime=3600",
		"limit-bytes-total=1073741824",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("enforce cmds missing %q in:\n%s", want, joined)
		}
	}
}

func TestTranslate_revokeGuestQuota(t *testing.T) {
	a := NewApplier(&fakeRunner{}, "wg-magichub", 13241, 0)
	steps, err := a.Translate(provisioner.Change{Action: provisioner.ActionRevokeGuestQuota, Target: "AA:BB:CC:DD:EE:FF"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	joined := ""
	for _, s := range steps {
		joined += s.Command + "\n"
	}
	if !strings.Contains(joined, "/ip hotspot user remove [find name=mm-AA-BB-CC-DD-EE-FF]") {
		t.Fatalf("revoke cmds = %q", joined)
	}
}

func TestTranslate_enforceGuestQuota_failsClosed(t *testing.T) {
	a := NewApplier(&fakeRunner{}, "wg-magichub", 13241, 0)
	cases := []provisioner.Change{
		// missing identity
		{Action: provisioner.ActionEnforceGuestQuota, Target: "", Proposed: `{"data_bytes":100}`},
		// malformed payload
		{Action: provisioner.ActionEnforceGuestQuota, Target: "AA:BB", Proposed: "not-json"},
		// negative limit
		{Action: provisioner.ActionEnforceGuestQuota, Target: "AA:BB", Proposed: `{"data_bytes":-5}`},
		// nothing to enforce
		{Action: provisioner.ActionEnforceGuestQuota, Target: "AA:BB", Proposed: `{"data_bytes":0,"uptime_seconds":0}`},
	}
	for i, c := range cases {
		if _, err := a.Translate(c); err == nil {
			t.Fatalf("case %d must fail closed: %+v", i, c)
		}
	}
	if _, err := a.Translate(provisioner.Change{Action: provisioner.ActionRevokeGuestQuota, Target: ""}); err == nil {
		t.Fatal("revoke with empty identity must fail closed")
	}
}

func TestGuestUserName_normalizes(t *testing.T) {
	cases := map[string]string{
		"AA:BB:CC:DD:EE:FF": "mm-AA-BB-CC-DD-EE-FF",
		"09-123456789":      "mm-09-123456789",
		"mm-dup":            "mm-dup",
		"":                  "mm-guest",
		"???":               "mm----",
	}
	for in, want := range cases {
		if got := guestUserName(in); got != want {
			t.Fatalf("guestUserName(%q) = %q, want %q", in, got, want)
		}
	}
	// names must be RouterOS-safe (no colons/slashes/spaces survive)
	got := guestUserName("AA:BB/CC DD EE")
	for _, bad := range []string{":", "/", " "} {
		if strings.Contains(got, bad) {
			t.Fatalf("guestUserName left unsafe char %q in %q", bad, got)
		}
	}
}

func TestParseGuestQuota(t *testing.T) {
	if _, err := parseGuestQuota(""); err != nil {
		t.Fatal("empty payload should parse (translation fails closed later)")
	}
	q, err := parseGuestQuota(`{"data_bytes":1024,"uptime_seconds":60,"profile":"mb"}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if q.DataBytes != 1024 || q.UptimeSeconds != 60 || q.Profile != "mb" {
		t.Fatalf("parse = %+v", q)
	}
	if _, err := parseGuestQuota("{bad"); err == nil {
		t.Fatal("malformed payload must error")
	}
}

func TestParseUptimeSeconds(t *testing.T) {
	cases := map[string]int{
		"1h":       3600,
		"2h30m":    9000,
		"45s":      45,
		"3600":     3600,
		"":         0,
		"nonsense": 0,
	}
	for in, want := range cases {
		if got := parseUptimeSeconds(in); got != want {
			t.Fatalf("parseUptimeSeconds(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestProber_observeGuestQuota(t *testing.T) {
	// user present
	pr := NewProber(&fakeRunner{out: map[string]string{
		"hotspot user print": "name: mm-AA-BB-CC-DD-EE-FF\n  limit-uptime: 1h\n  limit-bytes-total: 1073741824\n  profile: default\n",
	}}, "wg-magichub", "", 0)
	st, err := pr.ObserveGuestQuota(context.Background(), "AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatalf("ObserveGuestQuota present: %v", err)
	}
	if !st.Exists || st.LimitBytes != 1073741824 || st.LimitUptime != 3600 {
		t.Fatalf("present state = %+v", st)
	}

	// user missing -> Exists=false
	pr2 := NewProber(&fakeRunner{out: map[string]string{
		"hotspot user print": "",
	}}, "wg-magichub", "", 0)
	st2, err := pr2.ObserveGuestQuota(context.Background(), "AA:BB")
	if err != nil {
		t.Fatalf("ObserveGuestQuota missing: %v", err)
	}
	if st2.Exists {
		t.Fatalf("missing user must report Exists=false, got %+v", st2)
	}

	// runner error propagates
	pr3 := NewProber(&fakeRunner{err: map[string]error{"hotspot": errors.New("ssh refused")}}, "wg-magichub", "", 0)
	if _, err := pr3.ObserveGuestQuota(context.Background(), "AA:BB"); err == nil {
		t.Fatal("runner error must propagate")
	}
}
