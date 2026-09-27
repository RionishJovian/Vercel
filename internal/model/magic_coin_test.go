package model

import "testing"

// The kind vocabulary must match magic_coin_transactions.kind CHECK exactly —
// a kind the DB rejects would surface as a 500 at runtime instead of being
// caught here.
func TestMagicCoinKindValid(t *testing.T) {
	valid := []MagicCoinKind{
		CoinAgentEarn, CoinAgentReversal, CoinServicePayment, CoinServiceRefund,
		CoinFeatureKeyPurchase, CoinPurchaseCredit, CoinManualCredit, CoinManualDebit,
	}
	if len(valid) != 8 {
		t.Fatalf("expected 8 coin kinds, got %d — the DB CHECK allows exactly 8", len(valid))
	}
	for _, k := range valid {
		if !k.Valid() {
			t.Errorf("kind %q should be valid", k)
		}
	}

	invalid := []MagicCoinKind{"", "credit", "MANUAL_CREDIT", "manualcredit", "agent_earned"}
	for _, k := range invalid {
		if k.Valid() {
			t.Errorf("kind %q should be rejected", k)
		}
	}
}

func TestMagicCoinKindsAreDistinct(t *testing.T) {
	seen := map[MagicCoinKind]bool{}
	for _, k := range []MagicCoinKind{
		CoinAgentEarn, CoinAgentReversal, CoinServicePayment, CoinServiceRefund,
		CoinFeatureKeyPurchase, CoinPurchaseCredit, CoinManualCredit, CoinManualDebit,
	} {
		if seen[k] {
			t.Errorf("duplicate coin kind %q", k)
		}
		seen[k] = true
	}
}
