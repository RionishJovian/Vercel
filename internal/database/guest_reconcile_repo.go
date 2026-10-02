package database

import (
	"context"
	"database/sql"
	"strings"

	"mega-magic/internal/guestaccess"
)

var _ guestaccess.RepairStore = GuestRecoveryRepository{}

// FenceInterrupted never replenishes usage or extends validity. Tenant RLS and
// voucher-first locking match reserve/complete/revoke. Recently updated transfers
// are left alone. A blocked row is retried even after purchase expiry so an
// offline router cannot leave an uncertain binding silently uncontained.
func (r GuestRecoveryRepository) FenceInterrupted(ctx context.Context, biz string, limit int) ([]guestaccess.RecoveryClaim, error) {
	if r.DB == nil || biz == "" || limit < 1 || limit > 25 {
		return nil, guestaccess.ErrRecoveryDenied
	}
	var claims []guestaccess.RecoveryClaim
	err := r.transaction(ctx, biz, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT r.voucher_id::text FROM guest_access_recovery r
 WHERE r.business_id=$1 AND (r.state='blocked' OR
 (r.state IN ('transferring','frozen') AND r.updated_at<clock_timestamp()-interval '2 minutes'))
 ORDER BY r.updated_at,r.voucher_id LIMIT $2`, biz, limit)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			var locked string
			// Skip another worker's locked voucher rather than holding the whole
			// batch behind a concurrent purchase operation.
			err := tx.QueryRowContext(ctx, `SELECT id::text FROM vouchers WHERE id=$1 AND business_id=$2 FOR UPDATE SKIP LOCKED`, id, biz).Scan(&locked)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return err
			}
			var g guestaccess.RecoveryClaim
			err = tx.QueryRowContext(ctx, `UPDATE guest_access_recovery r SET state='blocked',updated_at=clock_timestamp()
 FROM guest_voucher_grants g WHERE r.voucher_id=$1 AND r.business_id=$2
 AND g.voucher_id=r.voucher_id AND g.business_id=r.business_id AND g.device_id=r.device_id
 AND (r.state='blocked' OR (r.state IN ('transferring','frozen') AND r.updated_at<clock_timestamp()-interval '2 minutes'))
 RETURNING r.business_id::text,r.device_id::text,r.voucher_id::text,r.transfer_id::text,
 r.current_mac::text,r.pending_mac::text,g.expires_at,g.data_mb::bigint*1000000,r.used_bytes`, id, biz).
				Scan(&g.BusinessID, &g.DeviceID, &g.VoucherID, &g.TransferID, &g.OldMAC, &g.NewMAC, &g.ExpiresAt, &g.TotalBytes, &g.UsedBytes)
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return err
			}
			g.OldMAC, g.NewMAC = strings.ToUpper(g.OldMAC), strings.ToUpper(g.NewMAC)
			claims = append(claims, g)
		}
		return nil
	})
	if err != nil {
		return nil, err
	} // No remote authority after an uncertain commit.
	return claims, nil
}
