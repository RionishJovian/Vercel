package config

import (
	"strings"
	"testing"
)

func TestSubscriptionRolloutRequiresPaymentPolicy(t *testing.T) {
	for _, policy := range []string{"", "grandfather", "unknown"} {
		cfg := &Config{Environment: "test", Billing: BillingConfig{SubscriptionOnly: true, ExistingAccountPolicy: policy}}
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "must be require_payment") {
			t.Fatalf("policy %q accepted", policy)
		}
	}
	cfg := &Config{Environment: "test", Billing: BillingConfig{SubscriptionOnly: true, ExistingAccountPolicy: "require_payment"}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
