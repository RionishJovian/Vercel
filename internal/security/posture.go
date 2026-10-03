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
//  3. Record published stable, long-term and testing fixes separately in
//     FixedByChannel. Resolve the installed release branch before the selected
//     update channel; unknown channels fall back to the stable target. Never
//     assume a newer minor release contains an older branch's patched build.
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
	// FixUnavailable marks an advisory the vendor has published NO fix for.
	// Upgrading does not resolve it — the mitigation is environmental (close the
	// reachable service). Kept separate because the customer's action differs
	// completely from an "upgrade to X" advisory, and collapsing the two makes
	// the whole audit read as a wall of indistinguishable "affected".
	FixUnavailable bool
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
	// Published fixed releases by update track. Installed release branch takes
	// precedence over the selected update channel (which can be changed before
	// an upgrade). Unknown channels use the stable recommendation.
	FixedByChannel map[string]string
}

// AdvisoryTable is the curated table.
//
// Every range below was read from the cited publication. Newest first so the
// most urgent entries are the first thing a reader sees.
var AdvisoryTable = []Advisory{
	{
		ID:             "CVE-2026-14227",
		Title:          "WireGuard private key disclosure via low-privilege API session",
		Severity:       "4.9",
		FixUnavailable: true,
		Note:           "affects EVERY version — CISA lists no fixed release, so upgrading does NOT resolve it. Exploitation needs a reachable RouterOS API session (port 8728/8729); the mitigation is to close the API, not to patch.",
		Source:         "CISA ICSA-26-211-01 (2026-07-30)",
		Ranges: []VersionRange{
			{Series: "6", Min: "6.0"},
			{Series: "7", Min: "7.0"},
		},
	},
	{
		ID:     "CVE-2026-52346",
		Title:  "Out-of-bounds read inspecting TLS traffic in firewall tls-host rules",
		Note:   "only active where a firewall rule matches TLS connections (tls-host). Fix is 7.22.2 (stable) or 7.21.4 (long-term).",
		Source: "MikroTik supportsec cve-2026-52346 (2026-09-16)",
		Ranges: []VersionRange{
			{Series: "7", Min: "7.0", Max: "7.22.2", MaxExclusive: true, FixedByChannel: map[string]string{"stable": "7.22.2", "long-term": "7.21.4"}},
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
			{Series: "7", Min: "7.0", Max: "7.24.2", MaxExclusive: true, FixedByChannel: mikroTrickFixes()},
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
			{Series: "7", Min: "7.0", Max: "7.24.2", MaxExclusive: true, FixedByChannel: mikroTrickFixes()},
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
			{Series: "7", Min: "7.0", Max: "7.24.2", MaxExclusive: true, FixedByChannel: mikroTrickFixes()},
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
		ID:     "CVE-2025-6443",
		Title:  "VXLAN source-IP validation bypass (unauthenticated ingress)",
		Note:   "remote and unauthenticated; fixed by upgrading to 7.20 or later.",
		Source: "MikroTik supportsec cve-2025-6443 (2025-06-25)",
		Ranges: []VersionRange{
			{Series: "7", Min: "7.0", Max: "7.20", MaxExclusive: true},
		},
	},
	{
		ID:     "CVE-2024-54772",
		Title:  "WinBox username enumeration via response-size discrepancy",
		Note:   "lower severity — confirms whether an account exists, but the password must still be guessed.",
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

func mikroTrickFixes() map[string]string {
	return map[string]string{"stable": "7.24.2", "long-term": "7.23.4", "testing": "7.25beta3", "development": "7.25beta3"}
}

// Resolve each advisory on the installed release branch, then use the selected
// track for older versions. Copy ranges so request-time resolution cannot mutate
// the global table or race other device checks.
func tableForChannel(version, channel string, table []Advisory) []Advisory {
	major, minor, _, _ := parseVersion(version)
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel == "longterm" {
		channel = "long-term"
	}
	out := append([]Advisory(nil), table...)
	for i := range out {
		out[i].Ranges = append([]VersionRange(nil), out[i].Ranges...)
		for j := range out[i].Ranges {
			r := &out[i].Ranges[j]
			if len(r.FixedByChannel) == 0 {
				continue
			}
			fix := r.FixedByChannel[channel]
			if fix == "" {
				fix = r.FixedByChannel["stable"]
			}
			for _, branchFix := range r.FixedByChannel {
				m, n, _, ok := parseVersion(branchFix)
				if ok && m == major && n == minor {
					fix = branchFix
					break
				}
			}
			if fix != "" {
				r.Max = fix
			}
			r.FixedByChannel = nil // resolved; nested helpers must not change tracks
		}
	}
	return out
}

var prereleaseRe = regexp.MustCompile(`(?i)(beta|rc)(\d+)`)

func releaseRank(raw string) (int, int) {
	m := prereleaseRe.FindStringSubmatch(raw)
	if m == nil {
		return 2, 0
	}
	n, _ := strconv.Atoi(m[2])
	if strings.EqualFold(m[1], "beta") {
		return 0, n
	}
	return 1, n
}

func prereleaseBefore(version, bound string) bool {
	vr, vn := releaseRank(version)
	br, bn := releaseRank(bound)
	return vr < br || (vr == br && vn < bn)
}

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
	table = tableForChannel(version, "", table)
	vMajor, vMinor, vPatch, ok := parseVersion(version)
	if !ok {
		return nil
	}
	series := strconv.Itoa(vMajor)

	var out []Advisory
	for _, adv := range table {
		for _, r := range adv.Ranges {
			matches := rangeMatches(r, series, vMajor, vMinor, vPatch)
			// beta/rc is older than the final release with the same base version.
			if r.Series == series && r.MaxExclusive {
				m, n, p, ok := parseVersion(r.Max)
				if ok && cmpVersion(vMajor, vMinor, vPatch, m, n, p) == 0 {
					matches = prereleaseBefore(version, r.Max)
				}
			}
			if matches {
				out = append(out, adv)
				break // one matching range is enough to flag the advisory
			}
		}
	}
	return out
}

// ClassifyPosture returns safe|affected|unknown for a RouterOS version string.
// Fail-safe: anything missing or unparseable is "unknown".
func ClassifyPosture(version, channel string) string {
	return ClassifyPostureWith(version, tableForChannel(version, channel, AdvisoryTable))
}

// ClassifyPostureWith is ClassifyPosture against an explicit table (tests /
// alternate tables). This low-level function uses supplied bounds; production
// callers resolve release-track bounds through ClassifyPosture or Summarize.
func ClassifyPostureWith(version string, table []Advisory) string {
	if _, _, _, ok := parseVersion(version); !ok {
		return PostureUnknown
	}
	if len(MatchAdvisories(version, table)) > 0 {
		return PostureAffected
	}
	return PostureSafe
}

// PostureSummary is the full, explainable result of classifying one device.
// Posture answers "are we OK?"; the rest answers "why, and what do we do?".
type PostureSummary struct {
	Posture    string     // safe | affected | unknown
	Advisories []Advisory // every advisory this version falls into
	// Unpatchable: at least one match has NO vendor fix. Upgrading cannot clear
	// it; the fix is environmental. This is the difference between "patch it"
	// and "you cannot patch your way out of this", which is why it is surfaced
	// separately rather than folded into Posture.
	Unpatchable bool
	// Exploited: at least one match is confirmed exploited in the wild.
	Exploited bool
	// FixFirst is the lowest release that clears every FIXABLE match on this
	// device's series. Empty when nothing fixable matched (i.e. safe, unknown,
	// or only unfixable advisories apply).
	FixFirst string
}

// fixedIn returns the first release that resolves r, and whether one exists.
func fixedIn(r VersionRange) (string, bool) {
	if r.Max == "" {
		return "", false // no upper bound published — nothing to upgrade to
	}
	if r.MaxExclusive {
		return r.Max, true // Max IS the first fixed release
	}
	// Inclusive Max is the last affected release, so the fix is the next patch.
	maj, min, pat, ok := parseVersion(r.Max)
	if !ok {
		return "", false
	}
	return strconv.Itoa(maj) + "." + strconv.Itoa(min) + "." + strconv.Itoa(pat+1), true
}

// Summarize classifies a version AND explains the verdict: which advisories
// matched, whether any is unfixable, whether any is being exploited, and the
// lowest release that would clear the fixable ones.
func Summarize(version string, table []Advisory) PostureSummary {
	return SummarizeForChannel(version, "", table)
}

func SummarizeForChannel(version, channel string, table []Advisory) PostureSummary {
	table = tableForChannel(version, channel, table)
	s := PostureSummary{Posture: ClassifyPostureWith(version, table)}
	s.Advisories = MatchAdvisories(version, table)

	vMaj, _, _, ok := parseVersion(version)
	if !ok {
		return s // unknown: no version, so nothing to explain or act on
	}
	series := strconv.Itoa(vMaj)
	bestMaj, bestMin, bestPat := -1, -1, -1

	for _, a := range s.Advisories {
		if a.FixUnavailable {
			s.Unpatchable = true
		}
		if a.Exploited {
			s.Exploited = true
		}
		for _, r := range a.Ranges {
			if r.Series != series {
				continue
			}
			fx, hasFix := fixedIn(r)
			if !hasFix {
				continue
			}
			fMaj, fMin, fPat, okF := parseVersion(fx)
			if !okF {
				continue
			}
			if bestMaj < 0 || cmpVersion(fMaj, fMin, fPat, bestMaj, bestMin, bestPat) > 0 {
				bestMaj, bestMin, bestPat = fMaj, fMin, fPat
			}
		}
	}

	if bestMaj >= 0 {
		s.FixFirst = strconv.Itoa(bestMaj) + "." + strconv.Itoa(bestMin) + "." + strconv.Itoa(bestPat)
	}
	return s
}
