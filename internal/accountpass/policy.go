// Package accountpass governs individual web-app access, never router sessions.
package accountpass

import (
	"errors"
	"time"
	// Keep calendar approval available in minimal production containers.
	_ "time/tzdata"
)

// CalendarExpiry preserves the approval's local clock time and clamps month-end
// dates (Jan 31 -> Feb 28/29, Feb 29 + year -> Feb 28).
func CalendarExpiry(approved time.Time, plan string) (time.Time, error) {
	loc, err := time.LoadLocation("Asia/Yangon")
	if err != nil {
		return time.Time{}, err
	}
	start := approved.In(loc)
	year, month, day := start.Date()
	switch plan {
	case "monthly":
		month++
		if month > 12 {
			month = 1
			year++
		}
	case "annual":
		year++
	default:
		return time.Time{}, errors.New("monthly or annual plan required")
	}
	last := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	if day > last {
		day = last
	}
	return time.Date(year, month, day, start.Hour(), start.Minute(), start.Second(), start.Nanosecond(), loc).UTC(), nil
}

type Status struct {
	Authorized          bool       `json:"authorized_login"`
	Kind                string     `json:"kind"`
	Mode                string     `json:"mode"`
	Scope               string     `json:"scope"`
	Authority           string     `json:"authority"`
	State               string     `json:"state"`
	Access              bool       `json:"access"`
	Enforced            bool       `json:"enforced"`
	ActivatedAt         *time.Time `json:"activated_at,omitempty"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	TransitionExpiresAt *time.Time `json:"transition_expires_at,omitempty"`
	CapacityExempt      bool       `json:"capacity_exempt"`
	CapacityScope       string     `json:"capacity_scope"`
	RouterLimit         int        `json:"router_limit"`
	SiteLimit           int        `json:"site_limit"`
	UnlimitedVouchers   bool       `json:"unlimited_vouchers"`
	UnlimitedAI         bool       `json:"unlimited_ai"`
	UnlimitedJellyfin   bool       `json:"unlimited_jellyfin"`
}

func Evaluate(now time.Time, enabled, operator bool, start, end, revoked, transition *time.Time) Status {
	s := Status{Mode: "individual_pass", Scope: "account", Authority: "app_account_passes", Enforced: enabled, Authorized: true, ActivatedAt: start, ExpiresAt: end, TransitionExpiresAt: transition, State: "pass_required", CapacityScope: "business", RouterLimit: 1, SiteLimit: 1, UnlimitedVouchers: true, UnlimitedAI: true, UnlimitedJellyfin: true}
	if !enabled {
		s.Mode = "legacy_account_pass"
	}
	if operator {
		s.State = "platform_exempt"
		s.CapacityExempt = true
		s.Access = true
		return s
	}
	if revoked == nil && start != nil && end != nil && !start.After(now) && end.After(now) {
		s.State = "active"
		s.Access = true
		return s
	}
	if revoked == nil && transition != nil && transition.After(now) {
		s.State = "transition"
		s.Access = true
		return s
	}
	if revoked != nil {
		s.State = "revoked"
	} else if end != nil && !end.After(now) {
		s.State = "expired"
	} else if start != nil && start.After(now) {
		s.State = "scheduled"
	}
	if !enabled {
		s.Mode = "legacy_account_pass"
		s.Access = true
	}
	return s
}

// Exact method + route allowlist: expiry keeps identity, security and renewal
// available. Public guest auth, router enforcement and sync workers never use it.
func RecoveryRoute(method, path string) bool {
	switch method + " " + path {
	case "GET /api/v1/me", "GET /api/v1/me/profile", "GET /api/v1/me/app-pass", "GET /api/v1/subscription", "POST /api/v1/me/presence", "POST /api/v1/me/password", "GET /api/v1/me/app-pass/renewals", "POST /api/v1/me/app-pass/renewals":
		return true
	}
	return false
}
