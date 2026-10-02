package handler

// Only redemption may mark a code used. Edits may revoke/ban/expire an unused
// code, but cannot resurrect any terminal or redeemed code.
func voucherStatusTransitionAllowed(from, to string) bool {
	switch from {
	case "active", "unused":
		return to == from || to == "revoked" || to == "banned" || to == "expired"
	case "used", "revoked", "banned", "expired":
		return to == from
	default:
		return false
	}
}
