package handler

import "testing"

func TestVoucherEditsCannotResurrectOrRedeem(t *testing.T) {
	for _, from := range []string{"used", "revoked", "banned", "expired"} {
		for _, to := range []string{"active", "unused", "used", "revoked", "banned", "expired"} {
			if got := voucherStatusTransitionAllowed(from, to); got != (from == to) {
				t.Fatalf("transition %s -> %s = %v", from, to, got)
			}
		}
	}
	for _, from := range []string{"active", "unused"} {
		if voucherStatusTransitionAllowed(from, "used") {
			t.Fatal("edit redeemed a voucher")
		}
		if !voucherStatusTransitionAllowed(from, "revoked") {
			t.Fatal("revocation refused")
		}
	}
	if voucherStatusTransitionAllowed("unknown", "active") {
		t.Fatal("unknown state accepted")
	}
}
