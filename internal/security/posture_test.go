package security

import "testing"

func TestClassifyPosture(t *testing.T) {
	cases := []struct {
		version string
		want    string
	}{
		// CVE-2018-14847 window: 6.29–6.42
		{"6.29 (stable)", PostureAffected},
		{"6.42.1", PostureAffected},
		{"6.28", PostureAffected}, // inside CVE-2023-30799 6.0–6.49.7
		{"6.43", PostureAffected}, // also inside 6.0–6.49.7

		// CVE-2023-30799: 6.0–6.49.7 and 7.0–7.9.3
		{"6.49.7", PostureAffected},
		{"6.49.10 (long-term)", PostureSafe},
		{"7.9.3", PostureAffected},
		{"7.10", PostureSafe},
		{"7.15.3 (stable)", PostureSafe},
		{"6.0", PostureAffected}, // series floor of the 6.x advisory window

		// fail-safe
		{"", PostureUnknown},
		{"garbage", PostureUnknown},
		{"RouterOS", PostureUnknown},
	}
	for _, c := range cases {
		if got := ClassifyPosture(c.version, ""); got != c.want {
			t.Fatalf("ClassifyPosture(%q) = %q, want %q", c.version, got, c.want)
		}
	}
}

func TestClassifyPostureWith_customTable(t *testing.T) {
	table := []Advisory{{
		ID:     "TEST-1",
		Ranges: []VersionRange{{Series: "7", Min: "7.0", Max: ""}}, // open-ended
	}}
	if got := ClassifyPostureWith("7.99", table); got != PostureAffected {
		t.Fatalf("open-ended range: got %q, want affected", got)
	}
	if got := ClassifyPostureWith("6.99", table); got != PostureSafe {
		t.Fatalf("other series: got %q, want safe", got)
	}
}

func TestParseVersion(t *testing.T) {
	maj, min, pat, ok := parseVersion("7.15.3 (stable)")
	if !ok || maj != 7 || min != 15 || pat != 3 {
		t.Fatalf("parse = %d.%d.%d ok=%v", maj, min, pat, ok)
	}
	if _, _, _, ok := parseVersion("no digits"); ok {
		t.Fatal("non-version must not parse")
	}
}
