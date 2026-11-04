package broadband

import (
	"strings"
	"testing"
)

func TestProfileNamesRemainLiteralLabels(t *testing.T) {
	for _, name := range []string{"default", "Home 20M", "Plan (30 days)", "မြန်မာ Plan", `Plan: "Gold" $limit`, strings.Repeat("p", 64), strings.Repeat("é", 64)} {
		if !ValidProfileName(name) {
			t.Errorf("valid profile label rejected: %q", name)
		}
	}
	for _, name := range []string{"", " ", " Home", "Home ", "Home\nPlan", "Home\tPlan", "Home\x00Plan", "Home\x1bPlan", strings.Repeat("p", 65), strings.Repeat("é", 65), string([]byte{0xff})} {
		if ValidProfileName(name) {
			t.Errorf("invalid profile label accepted: %q", name)
		}
	}
}
