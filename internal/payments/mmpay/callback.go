package mmpay

import (
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// Expected must come from a stored order AND its provider merchant mapping.
// Select this Client's credentials from that mapping, not callback input.
type Expected struct {
	OrderID       string
	Amount        int64
	VendorQRRefID string
}

type Event struct {
	Payment
	Condition string `json:"condition"`
}

// VerifyCallback authenticates the exact received bytes before decoding. It does
// not enforce nonce age: the provider's callback retry/timestamp contract still
// needs confirmation. A persistent unique transaction reference and atomic order
// transition MUST deduplicate replays before any future entitlement fulfillment.
// Calling this method alone must never grant Internet or imply bank settlement.
func (c *Client) VerifyCallback(body []byte, nonce, signed string, expected Expected) (Event, error) {
	deny := errors.New("mmpay: callback rejected")
	if len(body) == 0 || len(body) > maxBody || len(signed) != 64 || !validID(nonce) || !validID(expected.OrderID) || expected.Amount < 1 || expected.VendorQRRefID == "" {
		return Event{}, deny
	}
	received, err := hex.DecodeString(signed)
	want, _ := hex.DecodeString(signature(c.config.SecretKey, nonce, body))
	if err != nil || !hmac.Equal(received, want) {
		return Event{}, deny
	}
	var event Event
	if json.Unmarshal(body, &event) != nil || event.OrderID != expected.OrderID || event.Amount != expected.Amount || event.Currency != "MMK" || event.VendorQRRefID != expected.VendorQRRefID || !validStatus(event.Status) {
		return Event{}, deny
	}
	if (event.Status == "SUCCESS" || event.Status == "REFUNDED") && !validID(event.TransactionRefID) {
		return Event{}, deny
	}
	return event, nil
}
