package routeros

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GuestQuota is the payload of an ActionEnforceGuestQuota Change.Proposed: the
// bought plan's enforceable limits on the router. DataBytes is the total data
// cap (bytes), UptimeSeconds the max session uptime (seconds); both ride the
// RouterOS hotspot-user limit-bytes-total / limit-uptime fields. Profile is the
// hotspot user profile (empty => default). Zero-but-positive rules are enforced
// in the translation so a plan that caps nothing is never applied.
type GuestQuota struct {
	DataBytes     int64  `json:"data_bytes"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	Profile       string `json:"profile,omitempty"`
}

// parseGuestQuota decodes a JSON GuestQuota payload. An empty payload is
// allowed here (translation fails closed on it later); a malformed payload is
// an error.
func parseGuestQuota(raw string) (GuestQuota, error) {
	if strings.TrimSpace(raw) == "" {
		return GuestQuota{}, nil
	}
	var q GuestQuota
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		return GuestQuota{}, fmt.Errorf("routeros: bad guest quota payload %q: %w", raw, err)
	}
	return q, nil
}

// guestUserName maps a guest identity (MAC address or phone number) to a stable,
// RouterOS-safe hotspot username. RouterOS usernames tolerate alphanumerics and
// a small set of punctuation; we normalize anything else to '-'.
func guestUserName(identity string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, identity)
	clean = strings.TrimPrefix(clean, "mm-")
	if len(clean) > 40 {
		clean = clean[:40]
	}
	if clean == "" {
		clean = "guest"
	}
	return "mm-" + clean
}
