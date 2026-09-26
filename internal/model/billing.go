package model

import "time"

// PurchaseRequest is a device-anchored, operator-approved billing request: an
// anonymous guest on a device picks a plan, uploads a payment receipt, and the
// operator approves/rejects it. Approving grants the plan entitlement once.
//
// BankReference is REQUIRED (a real paid transfer) and is unique per business
// among non-rejected requests. DurationMinutes is captured from the plan at
// request-creation time and is what approve grants.
type PurchaseRequest struct {
	ID              string     `json:"id"`
	BusinessID      string     `json:"business_id"`
	DeviceID        *string    `json:"device_id,omitempty"`
	PlanKey         string     `json:"plan_key"`
	Amount          int        `json:"amount"`
	BankReference   string     `json:"bank_reference"`
	DurationMinutes int        `json:"duration_minutes"`
	ReceiptPath     *string    `json:"receipt_path,omitempty"`
	Status          string     `json:"status"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
