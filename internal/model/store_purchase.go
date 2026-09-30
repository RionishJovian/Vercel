package model

import "time"

// StorePurchase is a platform product checkout. Prices and labels are resolved
// by the server; clients only select a product and supply payment evidence.
type StorePurchase struct {
	ID                      string     `json:"id"`
	BusinessID              string     `json:"business_id"`
	UserID                  string     `json:"user_id"`
	ServiceKey              string     `json:"service_key"`
	ServiceLabel            string     `json:"service_label"`
	PriceMMK                int        `json:"price_mmk"`
	MagicCoinPurchaseAmount *float64   `json:"magic_coin_purchase_amount,omitempty"`
	PaymentMethod           string     `json:"payment_method"`
	BankAccountID           *string    `json:"bank_account_id,omitempty"`
	BankReference           *string    `json:"bank_reference,omitempty"`
	Status                  string     `json:"status"`
	IdempotencyKey          string     `json:"idempotency_key"`
	DecidedBy               *string    `json:"decided_by,omitempty"`
	DecidedAt               *time.Time `json:"decided_at,omitempty"`
	RejectReason            *string    `json:"reject_reason,omitempty"`
	ActivatedExpiresAt      *time.Time `json:"activated_expires_at,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

type StoreBankAccount struct {
	ID            string `json:"id"`
	HolderName    string `json:"holder_name"`
	BankName      string `json:"bank_name"`
	AccountNumber string `json:"account_number"`
}
