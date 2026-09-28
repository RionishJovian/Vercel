package routeros

import (
	"strconv"
	"strings"
)

// parseValueList turns RouterOS "field: value" output into a map of trimmed
// pairs (last occurrence wins). Lines without a colon are ignored, as are blank
// keys.
//
// It is fed by two different command shapes, and both produce this format:
//   - `print value-list detail` on LIST menus (/interface, /ip hotspot user)
//   - plain `print` on SINGLETON menus (/system resource)
func parseValueList(out string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := strings.TrimSpace(line[i+1:])
		if k == "" {
			continue
		}
		m[k] = v
	}
	return m
}

// SystemResourceCommand reads the router's identity/version block.
//
// ⚠️ It must be EXACTLY this, with NO arguments. `/system resource` is a
// SINGLETON menu: unlike `/interface print`, it REJECTS `value-list` (and even
// `detail`) with "expected end of command". Verified against real RouterOS
// 7.15.3 — the previous `/system/resource print value-list detail` failed on
// every real device, so version capture could never have worked outside a test
// double that answered the invalid command.
const SystemResourceCommand = "/system/resource print"

// parseSystemResource extracts (version, channel) from `SystemResourceCommand`
// output.
//
// RouterOS renders BOTH in ONE field — "version: 7.15.3 (stable)" — and exposes
// no separate `channel` property. The channel is the parenthesised suffix, so
// splitting it out is the only way to report it.
func parseSystemResource(out string) (version, channel string) {
	raw := strings.TrimSpace(parseValueList(out)["version"])
	if raw == "" {
		return "", ""
	}
	if i := strings.IndexByte(raw, '('); i >= 0 {
		version = strings.TrimSpace(raw[:i])
		channel = strings.TrimSpace(strings.Trim(raw[i+1:], ") 	"))
		return version, channel
	}
	return raw, ""
}

// atoiOr parses an int, returning def for empty/garbage input.
func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}

// nonEmptyLines returns the trimmed non-empty lines of s.
func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// pingOK reports whether a RouterOS `/ping ... count=N` capture shows replies.
func pingOK(out string) bool {
	if strings.Contains(out, "packet-loss=100%") {
		return false
	}
	// Look for `received=<n>` and require n > 0.
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, "received="); i >= 0 {
			rest := line[i+len("received="):]
			end := strings.IndexAny(rest, " \t")
			if end < 0 {
				end = len(rest)
			}
			if n, err := strconv.Atoi(rest[:end]); err == nil && n > 0 {
				return true
			}
		}
	}
	return false
}

// pingSummary returns the RouterOS summary line (the one carrying sent=).
func pingSummary(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "sent=") {
			return strings.TrimSpace(line)
		}
	}
	return strings.TrimSpace(out)
}
