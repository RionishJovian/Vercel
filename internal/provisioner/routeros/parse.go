package routeros

import (
	"strconv"
	"strings"
)

// parseValueList turns RouterOS `print value-list detail` output into a map of
// trimmed "field: value" pairs (last occurrence wins). Lines without a colon are
// ignored, as are blank keys.
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
