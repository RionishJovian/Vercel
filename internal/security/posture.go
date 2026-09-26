// Package security holds the security-posture audit: it classifies a device's
// RouterOS version against a curated, Lead-tunable table of REAL advisories.
//
// Posture values:
//
//	"affected" — the version falls inside a tracked advisory range
//	"safe"     — the version parsed and matches NO tracked advisory
//	             (NOT "immune" — simply "nothing we track applies")
//	"unknown"  — version missing/unparseable (fail-safe: never assumed safe)
//
// The table is data, not policy hardcoded in control flow: amend AdvisoryTable
// to add ranges; nothing else changes.
package security

import (
	"regexp"
	"strconv"
	"strings"
)

// Posture values.
const (
	PostureSafe     = "safe"
	PostureAffected = "affected"
	PostureUnknown  = "unknown"
)

// Advisory is one tracked RouterOS security advisory with the affected version
// range (inclusive). Cite the advisory so the table stays auditable.
type Advisory struct {
	ID    string // e.g. "CVE-2023-30799"
	Title string
	// Affected ranges by series. Min/Max are inclusive; empty Max = open-ended.
	Ranges []VersionRange
}

// VersionRange is an inclusive [Min, Max] version window within one series.
type VersionRange struct {
	Series string // "6" or "7"
	Min    string // e.g. "6.29"
	Max    string // e.g. "6.42"; "" = no upper bound within the series
}

// AdvisoryTable is the curated table. Seeded from named MikroTik advisories:
//
//   - CVE-2018-14847 — Winbox credential-disclosure / auth bypass,
//     RouterOS 6.29 through 6.42 (MikroTik 2018-08 advisory).
//   - CVE-2023-30799 — RouterOS admin privilege escalation,
//     RouterOS 6.0–6.49.7 and 7.0–7.9.3 (MikroTik 2023-07 advisory).
//
// Amend as the Lead directs; keep each entry cited.
var AdvisoryTable = []Advisory{
	{
		ID:    "CVE-2018-14847",
		Title: "Winbox credential disclosure / authentication bypass",
		Ranges: []VersionRange{
			{Series: "6", Min: "6.29", Max: "6.42"},
		},
	},
	{
		ID:    "CVE-2023-30799",
		Title: "RouterOS admin privilege escalation",
		Ranges: []VersionRange{
			{Series: "6", Min: "6.0", Max: "6.49.7"},
			{Series: "7", Min: "7.0", Max: "7.9.3"},
		},
	},
}

var rosVersionRe = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

// parseVersion extracts major/minor/patch from a RouterOS version string such as
// "7.15.3 (stable)" or "6.49.7". ok=false when no version is present.
func parseVersion(raw string) (major, minor, patch int, ok bool) {
	m := rosVersionRe.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return 0, 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	if m[3] != "" {
		patch, _ = strconv.Atoi(m[3])
	}
	return major, minor, patch, true
}

func cmpVersion(aMajor, aMinor, aPatch, bMajor, bMinor, bPatch int) int {
	if aMajor != bMajor {
		return sign(aMajor - bMajor)
	}
	if aMinor != bMinor {
		return sign(aMinor - bMinor)
	}
	return sign(aPatch - bPatch)
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

// ClassifyPosture returns safe|affected|unknown for a RouterOS version string.
// Fail-safe: anything missing or unparseable is "unknown".
func ClassifyPosture(version, _ string) string {
	return ClassifyPostureWith(version, AdvisoryTable)
}

// ClassifyPostureWith is ClassifyPosture against an explicit table (tests /
// alternate tables). channel is accepted for future channel-specific rules and
// currently unused.
func ClassifyPostureWith(version string, table []Advisory) string {
	vMajor, vMinor, vPatch, ok := parseVersion(version)
	if !ok {
		return PostureUnknown
	}
	series := strconv.Itoa(vMajor)

	for _, adv := range table {
		for _, r := range adv.Ranges {
			if r.Series != series {
				continue
			}
			minMajor, minMinor, minPatch, okMin := parseVersion(r.Min)
			if !okMin {
				continue
			}
			if cmpVersion(vMajor, vMinor, vPatch, minMajor, minMinor, minPatch) < 0 {
				continue
			}
			if r.Max == "" {
				return PostureAffected
			}
			maxMajor, maxMinor, maxPatch, okMax := parseVersion(r.Max)
			if !okMax {
				continue
			}
			if cmpVersion(vMajor, vMinor, vPatch, maxMajor, maxMinor, maxPatch) <= 0 {
				return PostureAffected
			}
		}
	}
	return PostureSafe
}
