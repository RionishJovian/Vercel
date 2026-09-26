// Package billing holds the pure decision logic for device-anchored,
// operator-approved purchase requests: verdict parsing, plan resolution, and the
// single-wind status transitions. It reuses the money-path entitlement semantics
// (voucher.ResolvePlanEffect) so there is exactly one grant definition.
package billing

import (
	"errors"
	"fmt"
	"strings"

	"mega-magic/internal/voucher"
)

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
)

// ErrUnknownPlan is returned for a plan_key with no defined entitlement effect.
var ErrUnknownPlan = errors.New("billing: unknown plan")

// ErrAlreadyDecided is returned when a request is no longer pending.
var ErrAlreadyDecided = errors.New("billing: request already decided")

// ErrNoDuration is returned when a plan has no configured duration. Fail closed:
// a paid pass must never inherit a magic default.
var ErrNoDuration = errors.New("billing: plan has no duration configured")

// Decision is a parsed operator verdict.
type Decision struct {
	Approve bool
}

var approveWords = map[string]bool{"approve": true, "approved": true, "yes": true, "y": true, "ok": true, "accept": true}
var rejectWords = map[string]bool{"reject": true, "rejected": true, "no": true, "n": true, "deny": true, "cancel": true}

// ParseDecision parses an operator verdict. Strict and default-deny: blank,
// malformed, ambiguous, or unrecognised input yields recognized=false (which the
// caller treats as reject/deny).
func ParseDecision(text string) (Decision, bool) {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(tokens) != 1 {
		return Decision{}, false
	}
	switch {
	case approveWords[tokens[0]]:
		return Decision{Approve: true}, true
	case rejectWords[tokens[0]]:
		return Decision{Approve: false}, true
	default:
		return Decision{}, false
	}
}

// ResolvePlan maps a purchase plan_key to its entitlement, failing closed on an
// unknown plan (no state change may happen for an unknown plan).
func ResolvePlan(planKey string) (voucher.PlanEffect, error) {
	eff, err := voucher.ResolvePlanEffect(planKey)
	if err != nil {
		return voucher.PlanEffect{}, fmt.Errorf("%w: %q", ErrUnknownPlan, planKey)
	}
	return eff, nil
}

// PlanDurations is the config-driven plan -> duration (minutes) source. It is
// supplied by the operator's config (plan.<planKey>.duration_minutes), so the
// real number is set by the Lead, not a Go constant. A plan that is absent or
// whose duration is <= 0 FAILS CLOSED.
type PlanDurations map[string]int

// DurationMinutes returns the configured duration for a plan, failing closed on
// an unknown plan or an unset/zero duration.
func (p PlanDurations) DurationMinutes(planKey string) (int, error) {
	d, ok := p[planKey]
	if !ok || d <= 0 {
		return 0, fmt.Errorf("%w: %q", ErrNoDuration, planKey)
	}
	return d, nil
}

// NextStatus applies a decision to a current status. pending -> approved|rejected.
// Anything already decided is TERMINAL: changed=false with ErrAlreadyDecided, so
// a double-tap can never re-grant.
func NextStatus(current string, d Decision) (string, bool, error) {
	if current != StatusPending {
		return current, false, ErrAlreadyDecided
	}
	if d.Approve {
		return StatusApproved, true, nil
	}
	return StatusRejected, true, nil
}
