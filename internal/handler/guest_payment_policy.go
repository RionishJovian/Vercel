package handler

import (
	"net/http"
	"time"

	"mega-magic/internal/model"
)

const failedBankTransferCooldown = 6 * time.Hour

// A failed grace session cannot be renewed until six hours AFTER its deadline.
// Repeated HTTP clicks inside the same live window resume, never replenish it.
// A paid session is not a failed transfer and must never be removed for retry.
func guestGraceRetry(previous *model.GuestPurchaseRequest, now time.Time) (resume bool, err error) {
	if previous == nil {
		return false, nil
	}
	if previous.Status == "approved" {
		return false, &httpError{status: http.StatusConflict, msg: "this client already has an approved purchase; contact the hotspot owner to renew"}
	}
	if previous.GraceExpiresAt == nil {
		return false, &httpError{status: http.StatusConflict, msg: "the previous purchase has no payment deadline; contact the hotspot owner"}
	}
	if previous.Status == "pending" && now.Before(*previous.GraceExpiresAt) {
		return true, nil
	}
	retryAt := previous.GraceExpiresAt.Add(failedBankTransferCooldown)
	if now.Before(retryAt) {
		return false, &httpError{status: http.StatusTooManyRequests, msg: "bank-transfer temporary access can be retried after " + retryAt.UTC().Format(time.RFC3339)}
	}
	return false, nil
}
