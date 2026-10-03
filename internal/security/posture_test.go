package security

import "testing"

func TestClassifyPosture(t *testing.T) {
	cases := []struct {
		version string
		want    string
		why     string
	}{
		// CVE-2018-14847 window: 6.29–6.42
		{"6.29 (stable)", PostureAffected, "lower bound of the 2018 WinBox window"},
		{"6.42.1", PostureAffected, "inside 2018 window and inside CVE-2024-54772 (<6.49.18)"},
		{"6.28", PostureAffected, "below the 2018 window but inside CVE-2024-54772 (<6.49.18)"},
		{"6.43", PostureAffected, "past the 2018 window, still inside CVE-2024-54772"},

		// CVE-2023-30799: 6.0–6.49.7 and 7.0–7.9.3
		{"6.49.7", PostureAffected, "top of the 2023 6.x window, still inside CVE-2024-54772"},
		{"7.9.3", PostureAffected, "top of the 2023 7.x window"},
		{"6.0", PostureAffected, "series floor of the 2023 6.x window"},

		// These three CHANGED when the table grew from 2 advisories to 10.
		// Each was "safe" under the old table and must now be flagged — this is
		// the regression the expansion exists to catch.
		{"6.49.10 (long-term)", PostureAffected, "was safe; now inside CVE-2024-54772 (<6.49.18) and MikroTrick (<6.49.21)"},
		{"7.10", PostureAffected, "was safe; now inside CVE-2024-54772 (<7.18) and MikroTrick (<7.23.4)"},
		{"7.15.3 (stable)", PostureAffected, "was safe; now inside CVE-2024-54772, CVE-2025-6443 (<7.20) and MikroTrick (<7.23.4)"},

		// fail-safe
		{"", PostureUnknown, "empty must never read as safe"},
		{"garbage", PostureUnknown, "unparseable must never read as safe"},
		{"RouterOS", PostureUnknown, "no version digits present"},
	}
	for _, c := range cases {
		if got := ClassifyPosture(c.version, ""); got != c.want {
			t.Fatalf("ClassifyPosture(%q) = %q, want %q (%s)", c.version, got, c.want, c.why)
		}
	}
}

// TestClassifyPosture_noVersionIsEverSafe pins the fail-safe direction: a device
// we could not read a version from must never be reported healthy.
func TestClassifyPosture_noVersionIsEverSafe(t *testing.T) {
	for _, v := range []string{"", "   ", "unknown", "7", "RouterOS 7", "n/a"} {
		if got := ClassifyPosture(v, ""); got == PostureSafe {
			t.Fatalf("ClassifyPosture(%q) = safe; an unread version must never be safe", v)
		}
	}
}

// TestAdvisoryTable_boundaries is the load-bearing test: for EVERY advisory in
// the table it asserts that versions the vendor placed INSIDE the affected
// window match, and that the first version the vendor declared fixed does NOT.
//
// It matches per-advisory (not via ClassifyPosture) so that one entry's bug
// cannot be hidden by another entry also flagging the same version.
//
// Sources for each window are in AdvisoryTable[i].Source. When you amend a
// range, amend its boundary samples here too — that is the point: a range
// cannot be edited without its published boundary being re-asserted.
func TestAdvisoryTable_boundaries(t *testing.T) {
	type boundary struct {
		cve        string
		inside     []string // must be matched by this advisory
		firstFixed []string // first version past the window; must NOT be matched
	}
	cases := []boundary{
		{
			cve: "CVE-2026-14227", // all versions, no fix published
			inside:     []string{"6.0", "6.49.21", "7.0", "7.23.4", "7.99"},
			firstFixed: nil,
		},
		{
			cve:        "CVE-2026-52346", // prior to 7.22.2
			inside:     []string{"7.0", "7.21.3", "7.22.1"},
			firstFixed: []string{"7.22.2", "7.23.4"},
		},
		{
			cve:        "CVE-2026-67276", // prior to 6.49.21 / 7.23.4
			inside:     []string{"6.0", "6.49.20", "7.0", "7.23.3"},
			firstFixed: []string{"6.49.21", "7.24.2", "7.25"},
		},
		{
			cve:        "CVE-2026-86060", // prior to 6.49.21 / 7.23.4
			inside:     []string{"6.0", "6.49.20", "7.0", "7.23.3"},
			firstFixed: []string{"6.49.21", "7.24.2"},
		},
		{
			cve:        "CVE-2026-67277", // prior to 6.49.21 / 7.23.4
			inside:     []string{"6.0", "6.49.20", "7.0", "7.23.3"},
			firstFixed: []string{"6.49.21", "7.24.2"},
		},
		{
			cve:        "CVE-2025-10948", // prior to 7.20.1
			inside:     []string{"7.0", "7.20", "7.20.0"},
			firstFixed: []string{"7.20.1", "7.21"},
		},
		{
			cve:        "CVE-2025-6443", // prior to 7.20
			inside:     []string{"7.0", "7.19.9"},
			firstFixed: []string{"7.20", "7.20.1"},
		},
		{
			cve:        "CVE-2024-54772", // prior to 6.49.18 / 7.18
			inside:     []string{"6.0", "6.49.17", "7.0", "7.17.9"},
			firstFixed: []string{"6.49.18", "7.18"},
		},
		{
			cve:        "CVE-2023-30799", // inclusive 6.0-6.49.7 / 7.0-7.9.3
			inside:     []string{"6.0", "6.49.7", "7.0", "7.9.3"},
			firstFixed: []string{"6.49.8", "7.9.4", "7.10"},
		},
		{
			cve:        "CVE-2018-14847", // inclusive 6.29-6.42
			inside:     []string{"6.29", "6.30", "6.42"},
			firstFixed: []string{"6.28", "6.42.1", "6.43"},
		},
	}

	for _, c := range cases {
		adv, ok := findAdvisory(c.cve)
		if !ok {
			t.Fatalf("advisory %s is asserted here but missing from AdvisoryTable", c.cve)
		}
		for _, v := range c.inside {
			if !matchesAdvisory(v, adv) {
				t.Errorf("%s: version %q should be INSIDE the affected window but did not match", c.cve, v)
			}
		}
		for _, v := range c.firstFixed {
			if matchesAdvisory(v, adv) {
				t.Errorf("%s: version %q is at/past the fixed release but still matched", c.cve, v)
			}
		}
	}
}

// TestAdvisoryTable_isAuditable asserts the table keeps the properties that make
// it trustworthy: cited sources, parseable bounds, and no contradictory range.
func TestAdvisoryTable_isAuditable(t *testing.T) {
	seen := map[string]bool{}
	for _, adv := range AdvisoryTable {
		if adv.ID == "" {
			t.Error("advisory with empty ID")
		}
		if seen[adv.ID] {
			t.Errorf("duplicate advisory %s", adv.ID)
		}
		seen[adv.ID] = true

		if adv.Title == "" {
			t.Errorf("%s: empty title", adv.ID)
		}
		if adv.Source == "" {
			t.Errorf("%s: empty Source — every entry must cite where its range came from", adv.ID)
		}
		if len(adv.Ranges) == 0 {
			t.Errorf("%s: no ranges", adv.ID)
		}

		for i, r := range adv.Ranges {
			if r.Series == "" {
				t.Errorf("%s range %d: empty series", adv.ID, i)
			}
			minMaj, minMin, minPat, okMin := parseVersion(r.Min)
			if !okMin {
				t.Errorf("%s range %d: unparseable Min %q", adv.ID, i, r.Min)
				continue
			}
			if r.Max == "" {
				continue // open-ended is legitimate (still affected at HEAD)
			}
			maxMaj, maxMin, maxPat, okMax := parseVersion(r.Max)
			if !okMax {
				t.Errorf("%s range %d: unparseable Max %q", adv.ID, i, r.Max)
				continue
			}
			// Max must not sit below Min, which would make the range dead.
			if cmpVersion(maxMaj, maxMin, maxPat, minMaj, minMin, minPat) < 0 {
				t.Errorf("%s range %d: Max %q is below Min %q (dead range)", adv.ID, i, r.Max, r.Min)
			}
		}
	}

	// The table must actually claim something, or the feature is theatre.
	if len(AdvisoryTable) < 2 {
		t.Fatalf("advisory table has %d entries; expected the curated set", len(AdvisoryTable))
	}
}

// TestExclusiveMaxIsNotInclusive is a focused regression for the bug class that
// motivated MaxExclusive: treating the FIRST FIXED release as the LAST affected
// one silently clears it.
func TestExclusiveMaxIsNotInclusive(t *testing.T) {
	table := []Advisory{{
		ID:     "TEST-EXCL",
		Ranges: []VersionRange{{Series: "7", Min: "7.0", Max: "7.23.4", MaxExclusive: true}},
	}}
	if got := ClassifyPostureWith("7.23.3", table); got != PostureAffected {
		t.Errorf("just below the fix: got %q, want affected", got)
	}
	if got := ClassifyPostureWith("7.23.4", table); got != PostureSafe {
		t.Errorf("the fixed release itself: got %q, want safe", got)
	}

	// And the inclusive form must still behave inclusively.
	incl := []Advisory{{
		ID:     "TEST-INCL",
		Ranges: []VersionRange{{Series: "7", Min: "7.0", Max: "7.9.3"}},
	}}
	if got := ClassifyPostureWith("7.9.3", incl); got != PostureAffected {
		t.Errorf("inclusive max: got %q, want affected", got)
	}
	if got := ClassifyPostureWith("7.9.4", incl); got != PostureSafe {
		t.Errorf("just above inclusive max: got %q, want safe", got)
	}
}

// TestPatchReleaseIsNotClearedByShortMax guards the exact trap documented in the
// package comment: a Max of "7.17" parses to 7.17.0, so 7.17.5 is GREATER and
// would be wrongly cleared unless the window uses MaxExclusive correctly.
func TestPatchReleaseIsNotClearedByShortMax(t *testing.T) {
	adv, ok := findAdvisory("CVE-2024-54772")
	if !ok {
		t.Fatal("CVE-2024-54772 missing")
	}
	for _, v := range []string{"7.17.5", "7.17.99", "7.0.1"} {
		if !matchesAdvisory(v, adv) {
			t.Errorf("%q is below 7.18 and must still be affected", v)
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

func TestMatchAdvisories_explainsTheVerdict(t *testing.T) {
	got := MatchAdvisories("7.15.3", AdvisoryTable)
	if len(got) == 0 {
		t.Fatal("7.15.3 must match at least one advisory, or the UI has nothing to explain")
	}
	for _, a := range got {
		if a.ID == "" || a.Source == "" {
			t.Errorf("matched advisory %+v lacks an ID/source for the UI to show", a)
		}
	}
	// A version matching nothing must return nil, not an empty-but-non-nil slice
	// that a caller could mistake for "matched nothing".
	if m := MatchAdvisories("99.99", AdvisoryTable); m != nil {
		t.Errorf("no-match should be nil, got %#v", m)
	}
	if m := MatchAdvisories("garbage", AdvisoryTable); m != nil {
		t.Errorf("unparseable should be nil, got %#v", m)
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

// --- helpers ---

func findAdvisory(id string) (Advisory, bool) {
	for _, a := range AdvisoryTable {
		if a.ID == id {
			return a, true
		}
	}
	return Advisory{}, false
}

func matchesAdvisory(version string, adv Advisory) bool {
	for _, m := range MatchAdvisories(version, []Advisory{adv}) {
		if m.ID == adv.ID {
			return true
		}
	}
	return false
}

// TestSummarize_unpatchableIsItsOwnSignal pins the distinction the UI needs:
// "affected, upgrade to X" and "affected, there is no X" must not collapse into
// one indistinguishable state.
func TestSummarize_unpatchableIsItsOwnSignal(t *testing.T) {
	// A version whose ONLY outstanding advisory is the unfixable one: upgrading
	// cannot help, so there must be no fix target to offer.
	only := Summarize("7.25", AdvisoryTable)
	if only.Posture != PostureAffected {
		t.Errorf("7.25 posture = %q, want affected", only.Posture)
	}
	if !only.Unpatchable {
		t.Error("7.25 must be flagged unpatchable (CVE-2026-14227 has no fix)")
	}
	if only.FixFirst != "" {
		t.Errorf("7.25 has nothing fixable to upgrade to, got FixFirst=%q", only.FixFirst)
	}

	// A version with fixable advisories MUST offer a target, even though it is
	// also unpatchable — the operator has two different actions to take.
	mixed := Summarize("7.15.3", AdvisoryTable)
	if !mixed.Unpatchable {
		t.Error("7.15.3 must be flagged unpatchable")
	}
	if !mixed.Exploited {
		t.Error("7.15.3 must be flagged exploited (MikroTrick + CVE-2025-10948)")
	}
	// With an unknown update channel, recommend the fixed stable release.
	if mixed.FixFirst != "7.24.2" {
		t.Errorf("7.15.3 FixFirst = %q, want 7.24.2", mixed.FixFirst)
	}
	if len(mixed.Advisories) == 0 {
		t.Error("7.15.3 must expose its matched advisories so the UI can explain")
	}
}

// TestSummarize_fixFirstIsTheHighestBound guards against offering a target that
// clears one advisory but leaves another open.
func TestSummarize_fixFirstIsTheHighestBound(t *testing.T) {
	cases := map[string]string{
		// 6.49.7 matches MikroTrick (6.49.21), CVE-2024-54772 (6.49.18) and
		// CVE-2023-30799 (inclusive max 6.49.7 -> fix 6.49.8). Highest wins.
		"6.49.7": "6.49.21",
		"7.9.3":  "7.24.2",
		"7.20":   "7.24.2",
	}
	for v, want := range cases {
		if got := Summarize(v, AdvisoryTable).FixFirst; got != want {
			t.Errorf("Summarize(%q).FixFirst = %q, want %q", v, got, want)
		}
	}
}

// TestSummarize_unknownOffersNothing keeps "we couldn't tell" from looking
// actionable: no advisories, no fix target, and never reported safe.
func TestSummarize_unknownOffersNothing(t *testing.T) {
	for _, v := range []string{"", "garbage", "no version here"} {
		s := Summarize(v, AdvisoryTable)
		if s.Posture != PostureUnknown {
			t.Errorf("Summarize(%q).Posture = %q, want unknown", v, s.Posture)
		}
		if len(s.Advisories) != 0 || s.FixFirst != "" || s.Unpatchable {
			t.Errorf("Summarize(%q) invented detail for an unreadable version: %+v", v, s)
		}
	}
}

// TestSummarize_everyAffectedVersionIsExplained: any affected verdict must carry
// at least one advisory, or the UI has a red badge it cannot justify.
func TestSummarize_everyAffectedVersionIsExplained(t *testing.T) {
	for _, v := range []string{"6.0", "6.49.7", "6.49.21", "7.0", "7.15.3", "7.23.4", "7.99"} {
		s := Summarize(v, AdvisoryTable)
		if s.Posture == PostureAffected && len(s.Advisories) == 0 {
			t.Errorf("%q is affected but lists no advisory", v)
		}
	}
}
