// Package discovery turns local network announcements into unverified observations.
// It only receives: nothing here connects to a router, logs in, or changes configuration.
package discovery

import (
	"net/netip"
	"time"
)

// Status of every observation this package produces. A discovered device is not a connected,
// managed or online one; only the existing Magic Hub enrollment can establish that.
const StatusNearbyUnverified = "nearby_unverified"

// Observation is one device announcement. Empty strings mean "not advertised", never a guess.
type Observation struct {
	Brand     string `json:"brand"`
	Method    string `json:"method"`
	Status    string `json:"status"`
	MAC       string `json:"mac"`
	Identity  string `json:"identity,omitempty"`
	Board     string `json:"board,omitempty"`
	Platform  string `json:"platform,omitempty"`
	Version   string `json:"version,omitempty"`
	Interface string `json:"interface,omitempty"`
	ClaimedIP string `json:"claimed_ip,omitempty"`
	SourceIP  string `json:"source_ip,omitempty"`
	// IPMismatch is set when the address inside the packet differs from the address it came from.
	IPMismatch bool `json:"ip_mismatch,omitempty"`
	// IPChanged is set when the same MAC was heard from a different address during this run.
	IPChanged bool      `json:"ip_changed,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Count     int       `json:"count"`
}

// Capabilities is what an adapter declares up front, so the app never assumes one brand behaves like another.
type Capabilities struct {
	Brand          string   `json:"brand"`
	Method         string   `json:"method"`
	Models         string   `json:"models"`
	Evidence       []string `json:"evidence"`
	Authentication string   `json:"authentication"`
}

// Adapter decodes one brand's announcements. It has no write or connect methods by design.
type Adapter interface {
	Capabilities() Capabilities
	// Port is the UDP port the announcements arrive on.
	Port() int
	// Parse validates and decodes one packet. Anything malformed is an error, never a partial guess.
	Parse(packet []byte, from netip.AddrPort, now time.Time) (Observation, error)
}
