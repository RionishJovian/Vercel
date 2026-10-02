package subscription

import (
	"testing"
	"time"
)

func TestExpiryAndExistingPolicy(t *testing.T) {
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	for _, c := range []struct {
		name        string
		trial, paid *time.Time
		policy      string
		access      bool
		state       string
	}{
		{"trial", &future, nil, "require_payment", true, "trial"},
		{"exact expiry", &now, nil, "grandfather", false, "payment_required"},
		{"paid", &now, &future, "require_payment", true, "paid"},
		{"expired paid", &now, &now, "require_payment", false, "payment_required"},
		{"existing unpaid", nil, nil, "require_payment", false, "payment_required"},
		{"existing grandfather", nil, nil, "grandfather", true, "existing_account"},
		{"unspecified fails closed", nil, nil, "", false, "payment_required"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := Evaluate(now, c.trial, c.paid, c.policy)
			if s.Access != c.access || s.State != c.state {
				t.Fatalf("%+v", s)
			}
		})
	}
}

func TestRecoveryRoutes(t *testing.T) {
	for _, p := range []string{"/api/v1/subscription", "/api/v1/me/password", "/api/v1/store/purchases"} {
		if !RecoveryRoute(p) {
			t.Fatal(p)
		}
	}
	for _, p := range []string{"/api/v1/routers", "/api/v1/store/purchases/extra", "/api/v1/media", ""} {
		if RecoveryRoute(p) {
			t.Fatal(p)
		}
	}
}
