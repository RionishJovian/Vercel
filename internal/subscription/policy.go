// Package subscription implements webapp access only, not guest internet/media sales.
package subscription

import "time"

type Status struct {
	Mode                       string     `json:"mode"`
	State                      string     `json:"state"`
	Access                     bool       `json:"access"`
	TrialExpiresAt             *time.Time `json:"trial_expires_at,omitempty"`
	PaidExpiresAt              *time.Time `json:"paid_expires_at,omitempty"`
	FeatureRequirementsEnabled bool       `json:"feature_requirements_enabled"`
}

// Evaluate never renews a trial. A missing trial identifies a pre-rollout tenant.
func Evaluate(now time.Time, trial, paid *time.Time, existingPolicy string) Status {
	s := Status{Mode: "subscription_only", State: "payment_required", TrialExpiresAt: trial, PaidExpiresAt: paid}
	if paid != nil && paid.After(now) {
		s.State, s.Access = "paid", true
	} else if trial != nil && trial.After(now) {
		s.State, s.Access = "trial", true
	} else if trial == nil && existingPolicy == "grandfather" {
		s.State, s.Access = "existing_account", true
	}
	return s
}

// RecoveryRoute keeps renewal and account security usable after expiry. Exact
// paths prevent a similarly named feature route from bypassing subscription checks.
func RecoveryRoute(path string) bool {
	switch path {
	case "/api/v1/me", "/api/v1/me/profile", "/api/v1/me/password", "/api/v1/subscription",
		"/api/v1/store/purchases", "/api/v1/store/purchases/:purchaseId/approve", "/api/v1/store/purchases/:purchaseId/reject",
		"/api/v1/store/checkout-options", "/api/v1/pricing":
		return true
	}
	return false
}
