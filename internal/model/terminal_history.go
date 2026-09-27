package model

import (
	"regexp"
	"strings"
	"time"
)

// TerminalHistoryEntry is one command the platform ran against a router.
//
// The old app stored the request body and a slice of the response verbatim. Those
// are commands and replies, so they are surfaced — but see RedactSecrets: a script
// like `/user add name=x password=y` would land in `body` verbatim, and this log
// is rendered to tenants.
type TerminalHistoryEntry struct {
	ID                 string  `json:"id"`
	BusinessID         string  `json:"business_id"`
	UserID             *string `json:"user_id,omitempty"`
	SiteID             *string `json:"site_id,omitempty"`
	RouterConnectionID *string `json:"router_connection_id,omitempty"`
	Method             string  `json:"method"`
	Path               string  `json:"path"`
	Status             int     `json:"status"`
	// StatusKnown is false when the column was NULL. The schema allows it, so the
	// console shows "—" rather than a 0 that would read as a successful HTTP call.
	StatusKnown   bool `json:"status_known"`
	DurationMS    int  `json:"duration_ms"`
	DurationKnown bool `json:"duration_known"`
	// Script and Source are lifted out of `body` when it parses as JSON; otherwise
	// the raw body is exposed as Script so nothing is silently dropped.
	Script          string    `json:"script"`
	Source          string    `json:"source,omitempty"`
	InsightID       string    `json:"insight_id,omitempty"`
	BodyUnparsed    bool      `json:"body_unparsed,omitempty"`
	ResponseSnippet string    `json:"response_snippet,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// SecretKeys are the parameter names whose VALUES must never be serialised out of
// a terminal command. Matched case-insensitively.
var SecretKeys = []string{
	"password", "passphrase", "pwd", "secret", "token",
	"private-key", "private_key", "psk", "wpa2-pre-shared-key",
}

// secretValueRe matches `key=value`, `key: value`, `key="value"` plus the JSON
// forms `"key":"value"` / `'key':'value'`. The key may be wrapped in quotes, which
// is why there are two optional-quote groups (Go's RE2 has no backreferences, so
// the opening and closing quotes cannot be tied together with \1).
var secretValueRe = regexp.MustCompile(`(?i)(["']?)(` + strings.Join(SecretKeys, "|") + `)(["']?\s*[:=]\s*)("(?:[^"\\]|\\.)*"|'[^']*'|[^\s,"'}\]]+)`)

// RedactSecrets masks credential values in a command or response string so a
// terminal log can be rendered without leaking them.
//
// It is deliberately conservative: it rewrites the VALUE and keeps the key, so
// `/user add name=admin password=hunter2` becomes
// `/user add name=admin password=***`. The literal `***` is used rather than an
// empty string so it is obvious a value was removed rather than absent.
func RedactSecrets(s string) string {
	if s == "" {
		return s
	}
	return secretValueRe.ReplaceAllString(s, "${1}${2}${3}***")
}

// ContainsSecret reports whether a string appears to carry a credential value.
// Used by tests to prove redaction actually triggers.
func ContainsSecret(s string) bool {
	return secretValueRe.MatchString(s)
}
