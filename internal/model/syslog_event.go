package model

import "time"

// SyslogSeverities are the severities the ported CHECK constraint allows. The
// filter validates against this list so an invalid value is a 400 rather than an
// empty result that looks like "no events".
var SyslogSeverities = []string{"info", "warning", "critical"}

// IsSyslogSeverity reports whether s is a valid severity.
func IsSyslogSeverity(s string) bool {
	for _, v := range SyslogSeverities {
		if v == s {
			return true
		}
	}
	return false
}

// SyslogEvent is one router log line, ported from Mikromagic.
//
// ai_summary/ai_severity are carried because the schema has them, but in the
// migrated data EVERY ai_summary is NULL — so the UI renders the AI badge only
// when it is actually present rather than showing a permanently empty column.
type SyslogEvent struct {
	ID                 string    `json:"id"`
	BusinessID         string    `json:"business_id"`
	RouterConnectionID *string   `json:"router_connection_id,omitempty"`
	SiteID             *string   `json:"site_id,omitempty"`
	SourceIP           *string   `json:"source_ip,omitempty"`
	Facility           *string   `json:"facility,omitempty"`
	Severity           string    `json:"severity"`
	Program            *string   `json:"program,omitempty"`
	Message            string    `json:"message"`
	AISummary          *string   `json:"ai_summary,omitempty"`
	AISeverity         *string   `json:"ai_severity,omitempty"`
	ReceivedAt         time.Time `json:"received_at"`
}

// SyslogFilter selects a page of the log. Newest-first is the only sensible
// default for an event log, so ordering is fixed rather than caller-controlled.
type SyslogFilter struct {
	Severity string
	Program  string
	Query    string // substring match on the message
	Limit    int
	Offset   int
}
