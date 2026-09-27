package model

import (
	"strings"
	"time"
)

// ScanRunKinds are the values the ported CHECK allows. Note that in the migrated
// data EVERY row is "ai" — "security" is permitted by the schema but unused. The
// filter is still offered so a future security scan would surface without a code
// change.
var ScanRunKinds = []string{"ai", "security"}

// ScanRunSeverities are the values the ported CHECK allows, worst first.
var ScanRunSeverities = []string{"critical", "warning", "info", "ok"}

// IsScanRunSeverity reports whether s is an allowed max_severity.
func IsScanRunSeverity(s string) bool {
	for _, v := range ScanRunSeverities {
		if v == s {
			return true
		}
	}
	return false
}

// IsScanRunKind reports whether k is an allowed kind.
func IsScanRunKind(k string) bool {
	for _, v := range ScanRunKinds {
		if v == k {
			return true
		}
	}
	return false
}

// jsErrorMarkers identify findings whose "subtitle" is actually a JavaScript
// runtime error that the OLD app persisted instead of a real observation.
//
// 1,102 of the 1,156 migrated rows contain
// "Cannot read properties of undefined (reading 'rest')". Those appear under the
// title "Router unreachable" — but the router evidently responded (the old code
// read its interface list before throwing), so these are the old app's crash, not
// fleet outages. The rows are left untouched (the live DB is authoritative); the
// API simply labels them so a console does not present 1,100 crashes as live
// critical alerts.
var jsErrorMarkers = []string{
	"Cannot read properties of",
	"Cannot read property",
	"is not a function",
	"undefined is not an object",
	"Unexpected token",
	"TypeError",
	"ReferenceError",
}

// RuntimeErrorMarkers exposes the markers so SQL-side metrics (for example the
// count of scans whose findings are crash artifacts) can be built from the SAME
// list the decoder uses, instead of a duplicated SQL literal drifting out of sync.
func RuntimeErrorMarkers() []string {
	out := make([]string, len(jsErrorMarkers))
	copy(out, jsErrorMarkers)
	return out
}

// LooksLikeRuntimeError reports whether s is a persisted JS/runtime error rather
// than a real finding description.
func LooksLikeRuntimeError(s string) bool {
	for _, m := range jsErrorMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// ScanFinding is one entry from payload.insights, normalised.
type ScanFinding struct {
	ID         string `json:"id,omitempty"`
	Title      string `json:"title"`
	Router     string `json:"router,omitempty"`
	RouterID   string `json:"router_id,omitempty"`
	Severity   string `json:"severity,omitempty"`
	Subtitle   string `json:"subtitle,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
	// RuntimeError is true when Subtitle is a persisted program error rather than
	// an observation about the network.
	RuntimeError bool `json:"runtime_error"`
}

// ScanNode is the OTHER payload shape: a topology/connectivity event carrying
// source/node_id/site_id/router_id/node_status instead of insights. Payload is
// genuinely heterogeneous, so both shapes are exposed rather than assuming one.
type ScanNode struct {
	Source     string `json:"source,omitempty"`
	NodeID     string `json:"node_id,omitempty"`
	SiteID     string `json:"site_id,omitempty"`
	RouterID   string `json:"router_id,omitempty"`
	NodeStatus string `json:"node_status,omitempty"`
	Provider   string `json:"provider,omitempty"`
}

// FleetScanRun is one AI/security fleet scan.
type FleetScanRun struct {
	ID           string        `json:"id"`
	BusinessID   string        `json:"business_id"`
	Kind         string        `json:"kind"`
	MaxSeverity  string        `json:"max_severity"`
	RouterCount  int           `json:"router_count"`
	GeneratedAt  time.Time     `json:"generated_at"`
	TriggeredBy  *string       `json:"triggered_by,omitempty"`
	Findings     []ScanFinding `json:"findings"`
	FindingCount int           `json:"finding_count"`
	// ErrorFindings counts findings that are really program errors.
	ErrorFindings      int       `json:"error_findings"`
	Node               *ScanNode `json:"node,omitempty"`
	DeterministicCount int       `json:"deterministic_count"`
	Snapshot           *string   `json:"snapshot,omitempty"`
}
