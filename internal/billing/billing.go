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

// PlanDurationMinutes returns the pass duration a plan grants. Product default:
// the Sovereign pass is 30 days.
func PlanDurationMinutes(planKey string) (int, error) {
	if _, err := ResolvePlan(planKey); err != nil {
		return 0, err
	}
	return 30 * 24 * 60, nil
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
