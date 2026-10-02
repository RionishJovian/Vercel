package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubscriptionRolloutEnvironmentWithoutYAMLKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("environment: test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BILLING_SUBSCRIPTION_ONLY", "true")
	t.Setenv("BILLING_EXISTING_ACCOUNT_POLICY", "require_payment")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Billing.SubscriptionOnly || cfg.Billing.ExistingAccountPolicy != "require_payment" {
		t.Fatal("subscription environment override ignored")
	}
}

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
