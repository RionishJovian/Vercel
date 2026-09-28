package handler

import (
	"testing"

	"mega-magic/internal/model"
)

// TestTierListExcludesOperatorTier proves the pricing filter is on the
// ADVERTISEMENT, not the vocabulary: the public price list omits the
// platform-operator tier while tenant creation still accepts it. Removing
// GLOBAL_ADMIN from model.AccessTiers instead would have silently broken tenant
// provisioning (validTiers is derived from it), which is why the check is here.
func TestTierListExcludesOperatorTier(t *testing.T) {
	tiers := tierList()

	if len(tiers) != 2 {
		t.Fatalf("public tier list has %d entries, want 2 (Small WISP, Enterprise ISP): %v",
			len(tiers), tiers)
	}
	for _, row := range tiers {
		key, _ := row["key"].(string)
		if key == model.AccessTierGlobalAdmin {
			t.Fatal("GLOBAL_ADMIN is advertised on the public price list")
		}
		quota, ok := row["device_quota"].(int)
		if !ok || quota <= 0 {
			t.Fatalf("tier %q advertised with a missing or non-positive device quota %v", key, row["device_quota"])
		}
	}

	// The operator tier must STILL be creatable.
	if !validTiers[model.AccessTierGlobalAdmin] {
		t.Fatal("GLOBAL_ADMIN is no longer accepted for tenant creation — " +
			"the pricing filter must not remove the product's vocabulary")
	}
	if !validTiers[model.AccessTierSmallWISP] || !validTiers[model.AccessTierEnterpriseISP] {
		t.Fatal("a sellable tier dropped out of validTiers")
	}
}
