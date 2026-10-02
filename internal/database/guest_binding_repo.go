package database

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"net"
	"strings"
	"time"

	"mega-magic/internal/guestaccess"
)

var _ guestaccess.BindingStore = GuestRecoveryRepository{}

// Lookup does not reserve a transfer or modify its window/count. Current MAC
// comes from recovery state; the grant's original MAC remains audit evidence.
func (r GuestRecoveryRepository) Lookup(ctx context.Context, req guestaccess.RecoveryRequest, hash []byte) (g guestaccess.BindingSnapshot, err error) {
	mac, e := net.ParseMAC(req.NewMAC)
	if r.DB == nil || len(hash) != 32 || e != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" || req.BusinessID == "" || req.DeviceID == "" || req.VoucherID == "" {
		return g, guestaccess.ErrRecoveryDenied
	}
	req.NewMAC = strings.ToUpper(mac.String())
	err = r.transaction(ctx, req.BusinessID, func(tx *sql.Tx) error {
		var stored []byte
		var now time.Time
		var state, grantState, voucherState string
		if e := tx.QueryRowContext(ctx, `SELECT r.secret_hash,r.current_mac::text,r.used_bytes,r.updated_at,
 g.expires_at,g.data_mb::bigint*1000000,r.state,g.state,v.status,clock_timestamp()
 FROM guest_access_recovery r JOIN guest_voucher_grants g
 ON g.voucher_id=r.voucher_id AND g.business_id=r.business_id AND g.device_id=r.device_id
 JOIN vouchers v ON v.id=g.voucher_id AND v.business_id=g.business_id
 WHERE r.voucher_id=$1 AND r.business_id=$2 AND r.device_id=$3`, req.VoucherID, req.BusinessID, req.DeviceID).
			Scan(&stored, &g.ClientMAC, &g.PriorUsedBytes, &g.Revision, &g.ExpiresAt, &g.TotalBytes, &state, &grantState, &voucherState, &now); e != nil {
			return e
		}
		g.ClientMAC = strings.ToUpper(g.ClientMAC)
		if subtle.ConstantTimeCompare(stored, hash) != 1 || state != "active" || grantState != "active" || voucherState != "used" || g.ClientMAC != req.NewMAC || !now.Before(g.ExpiresAt) || g.PriorUsedBytes < 0 || g.PriorUsedBytes >= g.TotalBytes {
			return guestaccess.ErrRecoveryDenied
		}
		g.BusinessID, g.DeviceID, g.VoucherID = req.BusinessID, req.DeviceID, req.VoucherID
		return nil
	})
	if err != nil {
		return guestaccess.BindingSnapshot{}, guestaccess.ErrRecoveryDenied
	}
	return g, nil
}
