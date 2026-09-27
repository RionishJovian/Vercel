package model

import "time"

// MagicCoinKind is a ledger entry kind. The values mirror the
// magic_coin_transactions.kind CHECK constraint exactly — a kind outside this set
// is rejected by the database, so they must stay in sync.
type MagicCoinKind string

const (
	CoinAgentEarn          MagicCoinKind = "agent_earn"
	CoinAgentReversal      MagicCoinKind = "agent_reversal"
	CoinServicePayment     MagicCoinKind = "service_payment"
	CoinServiceRefund      MagicCoinKind = "service_refund"
	CoinFeatureKeyPurchase MagicCoinKind = "feature_key_purchase"
	CoinPurchaseCredit     MagicCoinKind = "purchase_credit"
	CoinManualCredit       MagicCoinKind = "manual_credit"
	CoinManualDebit        MagicCoinKind = "manual_debit"
)

// Valid reports whether k is one of the kinds the database will accept.
func (k MagicCoinKind) Valid() bool {
	switch k {
	case CoinAgentEarn, CoinAgentReversal, CoinServicePayment, CoinServiceRefund,
		CoinFeatureKeyPurchase, CoinPurchaseCredit, CoinManualCredit, CoinManualDebit:
		return true
	}
	return false
}

// MagicCoinWallet is one user's coin balance within a business.
type MagicCoinWallet struct {
	UserID     string    `json:"user_id"`
	BusinessID string    `json:"business_id"`
	Balance    float64   `json:"balance"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// MagicCoinTransaction is a single, immutable ledger entry. The database forbids
// UPDATE and DELETE on this table, so these rows are the audit trail of every
// balance change.
type MagicCoinTransaction struct {
	ID                string    `json:"id"`
	BusinessID        string    `json:"business_id"`
	UserID            string    `json:"user_id"`
	Delta             float64   `json:"delta"`
	BalanceAfter      float64   `json:"balance_after"`
	Kind              string    `json:"kind"`
	Note              string    `json:"note"`
	ServicePurchaseID *string   `json:"service_purchase_id,omitempty"`
	ActorUserID       *string   `json:"actor_user_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// MagicCoinAdjustRequest is the body for a manual credit/debit. Only
// manual_credit / manual_debit are accepted — the other kinds are produced by
// their own flows, never by hand.
type MagicCoinAdjustRequest struct {
	UserID string  `json:"user_id" binding:"required"`
	Amount float64 `json:"amount" binding:"required"`
	Note   string  `json:"note"   binding:"required,min=3,max=500"`
	Kind   string  `json:"kind"   binding:"required,oneof=manual_credit manual_debit"`
}
