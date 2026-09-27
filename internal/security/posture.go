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
//
// SOURCING RULE — read before adding an entry:
//
//  1. Every entry cites the publication it came from in Source, and the range
//     lives in the vendor's OWN terms. Do not translate a fix note into a
//     guessed "last affected version".
//  2. MikroTik publishes fixes as "versions prior to <X>". That is why
//     VersionRange.MaxExclusive exists: it lets a range mean "everything below
//     the first fixed release". Setting Max to a made-up last-affected number
//     silently drops patch releases in between — e.g. Max "7.17" would wrongly
//     clear 7.17.5, because 7.17.5 > 7.17.
//  3. When a fix lands in two tracks (stable + long-term) and the classifier
//     cannot see the release channel, bound the range by the LATER track and
//     say so in Note. Erring toward "affected" is the fail-safe direction:
//     a false positive prompts an unnecessary upgrade, a false negative leaves
//     a known-vulnerable router looking healthy.
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
// range. Cite the advisory so the table stays auditable.
type Advisory struct {
	ID    string // e.g. "CVE-2023-30799"
	Title string
	// Severity is the published severity where one exists (CVSS base score as a
	// string), else "". Left empty rather than guessed.
	Severity string
	// Exploited marks advisories with confirmed in-the-wild exploitation. This
	// is the field a customer-facing "patch now" nudge should key on.
	Exploited bool
	// Note records the condition or caveat a reader needs — a config
	// precondition, a track ambiguity, or a "no fix exists" status. Empty when
	// the range speaks for itself.
	Note string
	// Source is the publication the range was taken from, for auditing.
	Source string
	// Ranges are the affected windows by series. Min/Max are inclusive unless
	// MaxExclusive is set; empty Max = no upper bound within the series.
	Ranges []VersionRange
}

// VersionRange is a version window within one series.
//
//	Series       "6" or "7"
//	Min          inclusive lower bound — MUST be a full "major.minor" string
//	             ("6.0" / "7.0" for the floor of a series). A bare "0" does not
//	             parse and silently makes the range match nothing.
//	Max          upper bound; "" means open-ended (still affected at HEAD)
//	MaxExclusive true when Max is the FIRST FIXED release rather than the last
//	             affected one, i.e. the window is everything strictly below Max.
//	             This is MikroTik's own framing ("versions prior to X").
type VersionRange struct {
	Series       string
	Min          string
	Max          string
	MaxExclusive bool
}

// AdvisoryTable is the curated table.
//
// Every range below was read from the cited publication. Newest first so the
// most urgent entries are the first thing a reader sees.
var AdvisoryTable = []Advisory{
	{
		ID:       "CVE-2026-14227",
		Title:    "WireGuard private key disclosure via low-privilege API session",
		Severity: "4.9",
		Note:     "affects EVERY version — CISA lists no fixed release. Exploitation needs a reachable RouterOS API session (port 8728/8729); the mitigation is to close the API, not to upgrade.",
		Source:   "CISA ICSA-26-211-01 (2026-07-30)",
		Ranges: []VersionRange{
			{Series: "6", Min: "6.0"},
			{Series: "7", Min: "7.0"},
		},
	},
	{
		ID:    "CVE-2026-52346",
		Title: "Out-of-bounds read inspecting TLS traffic in firewall tls-host rules",
		Note:  "only active where a firewall rule matches TLS connections (tls-host). Fix is 7.22.2 (stable) or 7.21.4 (long-term); bounded here by the stable track, so a patched long-term 7.21.4 still reads affected.",
		Source: "MikroTik supportsec cve-2026-52346 (2026-09-16)",
		Ranges: []VersionRange{
			{Series: "7", Min: "7.0", Max: "7.22.2", MaxExclusive: true},
		},
	},
	{
		ID:        "CVE-2026-67276",
		Title:     "SSH authentication bypass — public key not fully verified",
		Severity:  "9.2",
		Exploited: true,
		Note:      "part of the actively-exploited MikroTrick chain; chained with CVE-2026-86060 it yields unauthenticated full device control wherever SSH is internet-reachable.",
		Source:    "MikroTik Sept 2026 bulletin + CERT Polska (2026-09-05)",
		Ranges: []VersionRange{
			{Series: "6", Min: "6.0", Max: "6.49.21", MaxExclusive: true},
			{Series: "7", Min: "7.0", Max: "7.23.4", MaxExclusive: true},
		},
	},
	{
		ID:        "CVE-2026-86060",
		Title:     "SSH session privilege escalation via crafted username",
		Severity:  "9.2",
		Exploited: true,
		Note:      "part of the actively-exploited MikroTrick chain; the resulting session has full administrative privileges.",
		Source:    "MikroTik Sept 2026 bulletin + CERT Polska (2026-09-05)",
		Ranges: []VersionRange{
			{Series: "6", Min: "6.0", Max: "6.49.21", MaxExclusive: true},
			{Series: "7", Min: "7.0", Max: "7.23.4", MaxExclusive: true},
		},
	},
	{
		ID:        "CVE-2026-67277",
		Title:     "Bandwidth-test memory disclosure and crash (unauthenticated)",
		Severity:  "8.8",
		Exploited: true,
		Note:      "part of the MikroTrick chain; reachable without authentication on the bandwidth-test service.",
		Source:    "MikroTik Sept 2026 bulletin + CERT Polska (2026-09-05)",
		Ranges: []VersionRange{
			{Series: "6", Min: "6.0", Max: "6.49.21", MaxExclusive: true},
			{Series: "7", Min: "7.0", Max: "7.23.4", MaxExclusive: true},
		},
	},
	{
		ID:        "CVE-2025-10948",
		Title:     "Buffer overflow in libjson (parse_json_element) via /rest/ip/address/print",
		Exploited: true,
		Note:      "vendor states the exploit has been publicly disclosed and may be actively used; triggered through the REST API.",
		Source:    "MikroTik supportsec cve-2025-10948 (2025-09-25)",
		Ranges: []VersionRange{
			{Series: "7", Min: "7.0", Max: "7.20.1", MaxExclusive: true},
		},
	},
	{
		ID:    "CVE-2025-6443",
		Title: "VXLAN source-IP validation bypass (unauthenticated ingress)",
		Note:  "remote and unauthenticated; fixed by upgrading to 7.20 or later.",
		Source: "MikroTik supportsec cve-2025-6443 (2025-06-25)",
		Ranges: []VersionRange{
			{Series: "7", Min: "7.0", Max: "7.20", MaxExclusive: true},
		},
	},
	{
		ID:    "CVE-2024-54772",
		Title: "WinBox username enumeration via response-size discrepancy",
		Note:  "lower severity — confirms whether an account exists, but the password must still be guessed.",
		Source: "MikroTik supportsec cve-2024-54772 (2025-02-18)",
		Ranges: []VersionRange{
			{Series: "6", Min: "6.0", Max: "6.49.18", MaxExclusive: true},
			{Series: "7", Min: "7.0", Max: "7.18", MaxExclusive: true},
		},
	},
	{
		ID:     "CVE-2023-30799",
		Title:  "RouterOS administrator privilege escalation",
		Note:   "affects the whole 6.x line up to 6.49.7 and early 7.x.",
		Source: "MikroTik 2023-07 advisory",
		Ranges: []VersionRange{
			{Series: "6", Min: "6.0", Max: "6.49.7"},
			{Series: "7", Min: "7.0", Max: "7.9.3"},
		},
	},
	{
		ID:     "CVE-2018-14847",
		Title:  "WinBox credential disclosure / authentication bypass",
		Source: "MikroTik 2018-08 advisory",
		Ranges: []VersionRange{
			{Series: "6", Min: "6.29", Max: "6.42"},
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

// rangeMatches reports whether an already-parsed version falls inside r.
// Fails closed: an unparseable bound means "does not match" rather than a panic.
// Min "0" is the documented sentinel for "from the start of the series".
func rangeMatches(r VersionRange, series string, vMajor, vMinor, vPatch int) bool {
	if r.Series != series {
		return false
	}
	minMajor, minMinor, minPatch, okMin := parseVersion(r.Min)
	if !okMin {
		return false
	}
	if cmpVersion(vMajor, vMinor, vPatch, minMajor, minMinor, minPatch) < 0 {
		return false
	}
	if r.Max == "" {
		return true // open-ended: still affected at HEAD
	}
	maxMajor, maxMinor, maxPatch, okMax := parseVersion(r.Max)
	if !okMax {
		return false
	}
	cmp := cmpVersion(vMajor, vMinor, vPatch, maxMajor, maxMinor, maxPatch)
	if r.MaxExclusive {
		return cmp < 0
	}
	return cmp <= 0
}

// MatchAdvisories returns every advisory in table that the version falls into.
// This is what lets a caller EXPLAIN a verdict instead of merely asserting one.
// An unparseable or absent version matches nothing — callers must handle
// "unknown" separately, so that "we couldn't tell" never reads as "safe".
func MatchAdvisories(version string, table []Advisory) []Advisory {
	vMajor, vMinor, vPatch, ok := parseVersion(version)
	if !ok {
		return nil
	}
	series := strconv.Itoa(vMajor)

	var out []Advisory
	for _, adv := range table {
		for _, r := range adv.Ranges {
			if rangeMatches(r, series, vMajor, vMinor, vPatch) {
				out = append(out, adv)
				break // one matching range is enough to flag the advisory
			}
		}
	}
	return out
}

// ClassifyPosture returns safe|affected|unknown for a RouterOS version string.
// Fail-safe: anything missing or unparseable is "unknown".
func ClassifyPosture(version, _ string) string {
	return ClassifyPostureWith(version, AdvisoryTable)
}

// ClassifyPostureWith is ClassifyPosture against an explicit table (tests /
// alternate tables). channel is accepted for future channel-specific rules and
// currently unused — see SOURCING RULE 3 on the stable/long-term ambiguity.
func ClassifyPostureWith(version string, table []Advisory) string {
	if _, _, _, ok := parseVersion(version); !ok {
		return PostureUnknown
	}
	if len(MatchAdvisories(version, table)) > 0 {
		return PostureAffected
	}
	return PostureSafe
}
