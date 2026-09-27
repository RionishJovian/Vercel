package model

import "time"

// UnlockKeyActivation is a key a business bought with Magic Coins to unlock a
// capability (WebFig access, Magic Dude, a router, a reseller, a switch peer).
//
// `attributes` is a human-readable description, NOT a credential — despite the
// name it holds strings like "Unlock adding a CRS switch as its own Magic Hub
// peer". Verified against the migrated rows: no secret-shaped content.
type UnlockKeyActivation struct {
	ID              string     `json:"id"`
	BusinessID      string     `json:"business_id"`
	UserID          string     `json:"user_id"`
	KeyKind         string     `json:"key_kind"`
	KeyID           *int       `json:"key_id,omitempty"`
	KeyName         string     `json:"key_name,omitempty"`
	PriceCoins      float64    `json:"price_coins"`
	Attributes      string     `json:"attributes"`
	AccountBound    bool       `json:"account_bound"`
	NonTransferable bool       `json:"non_transferable"`
	UniqueRouterKey bool       `json:"unique_router_key"`
	GraceDays       *int       `json:"grace_days,omitempty"`
	ConsumedAt      *time.Time `json:"consumed_at,omitempty"`
	ConsumedRefID   *string    `json:"consumed_ref_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	ExpiresAt       time.Time  `json:"expires_at"`
	// Status is derived, never stored: consumed wins over expired, because a key
	// that was used and later lapsed was still used.
	Status string `json:"status"`
}

// UnlockStatuses is the closed set of derived statuses. Because it is a closed
// set, an unknown `?status=` value is a 400 rather than an empty page — an empty
// page would read as "this business has no keys", which is a different claim.
var UnlockStatuses = []string{"active", "expired", "consumed"}

// IsUnlockStatus reports whether s is a known derived status.
func IsUnlockStatus(s string) bool {
	for _, v := range UnlockStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// UnlockKeySummary is the tenant-wide view over the key ledger.
type UnlockKeySummary struct {
	Activations int              `json:"activations"`
	CoinsSpent  float64          `json:"coins_spent"`
	Active      int              `json:"active"`
	Expired     int              `json:"expired"`
	Consumed    int              `json:"consumed"`
	ByKind      []UnlockKeyGroup `json:"by_kind"`
}

// UnlockKeyGroup is one row of the per-kind breakdown.
type UnlockKeyGroup struct {
	Name       string  `json:"name"`
	Count      int     `json:"count"`
	CoinsSpent float64 `json:"coins_spent"`
}
