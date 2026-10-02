package model

import "time"

// GuestVoucherGrant holds the immutable purchased allowance. Reserving first
// use consumes the code before router I/O, and retries keep the same deadline.
type GuestVoucherGrant struct {
	VoucherID   string     `json:"voucher_id"`
	BusinessID  string     `json:"business_id"`
	DeviceID    string     `json:"device_id"`
	Minutes     int        `json:"minutes"`
	DataMB      int        `json:"data_mb"`
	State       string     `json:"state"`
	ClientMAC   string     `json:"client_mac,omitempty"`
	FirstUsedAt *time.Time `json:"first_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LeaseToken  string     `json:"-"`
	LeaseUntil  *time.Time `json:"-"`
	// Returned once to the HTTP-only cookie boundary, never API JSON or logs.
	RecoverySecret string `json:"-"`
}
