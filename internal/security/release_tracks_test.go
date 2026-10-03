package security

import "testing"

func TestPublishedReleaseTracks(t *testing.T) {
	for _, tc := range []struct {
		version, channel, id string
		affected             bool
	}{
		{"7.24.1", "stable", "CVE-2026-67276", true},
		{"7.24.2", "stable", "CVE-2026-67276", false},
		{"7.23.3", "long-term", "CVE-2026-67276", true},
		{"7.23.4", "long-term", "CVE-2026-67276", false},
		{"7.24.1", "long-term", "CVE-2026-67276", true},
		{"7.23.4", "stable", "CVE-2026-67276", false},
		{"7.24.1", "", "CVE-2026-67276", true},
		{"7.25beta2", "testing", "CVE-2026-67276", true},
		{"7.25beta3", "testing", "CVE-2026-67276", false},
		{"7.25rc1", "testing", "CVE-2026-67276", false},
		{"7.24.2rc1", "stable", "CVE-2026-67276", true},
		{"7.21.3", "long-term", "CVE-2026-52346", true},
		{"7.21.4", "long-term", "CVE-2026-52346", false},
		{"7.22.1", "stable", "CVE-2026-52346", true},
		{"7.22.2", "stable", "CVE-2026-52346", false},
	} {
		s := SummarizeForChannel(tc.version, tc.channel, AdvisoryTable)
		found := false
		for _, a := range s.Advisories {
			if a.ID == tc.id {
				found = true
			}
		}
		if found != tc.affected {
			t.Errorf("%s %s: %s affected=%v want %v", tc.version, tc.channel, tc.id, found, tc.affected)
		}
		if s.Posture != ClassifyPosture(tc.version, tc.channel) {
			t.Error("capture and live summary disagree")
		}
	}
	if got := SummarizeForChannel("7.15.3", "stable", AdvisoryTable).FixFirst; got != "7.24.2" {
		t.Errorf("stable fix=%s", got)
	}
	if got := SummarizeForChannel("7.15.3", "long-term", AdvisoryTable).FixFirst; got != "7.23.4" {
		t.Errorf("long-term fix=%s", got)
	}
	if AdvisoryTable[2].Ranges[1].Max != "7.24.2" {
		t.Error("global advisory table mutated")
	}
}
