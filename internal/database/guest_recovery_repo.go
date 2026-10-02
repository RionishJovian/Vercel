package database

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"net"
	"strings"
	"time"

	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

type GuestRecoveryRepository struct{ DB *DB }

var _ guestaccess.RecoveryStore = GuestRecoveryRepository{}
var _ guestaccess.RecoveryCredentialIssuer = GuestRecoveryRepository{}

// Issue serializes with reserve/revoke/finalize and returns plaintext only once
// commit is confirmed. No router I/O occurs under this tenant-scoped lock.
func (r GuestRecoveryRepository) Issue(ctx context.Context, g model.GuestVoucherGrant) (string, error) {
	if r.DB == nil || g.BusinessID == "" || g.DeviceID == "" || g.VoucherID == "" || g.State != "active" || g.ExpiresAt == nil || g.DataMB < 1 || g.Minutes < 1 {
		return "", guestaccess.ErrRecoveryDenied
	}
	var secret string
	err := r.transaction(ctx, g.BusinessID, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM vouchers WHERE id=$1 AND business_id=$2 FOR UPDATE`, g.VoucherID, g.BusinessID).Scan(&status); err != nil {
			return err
		}
		var valid bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM guest_voucher_grants
 WHERE voucher_id=$1 AND business_id=$2 AND device_id=$3 AND state='active'
 AND client_mac=$4::macaddr AND expires_at=$5 AND expires_at>clock_timestamp()
 AND minutes=$6 AND data_mb=$7)`, g.VoucherID, g.BusinessID, g.DeviceID, g.ClientMAC, g.ExpiresAt, g.Minutes, g.DataMB).Scan(&valid); err != nil {
			return err
		}
		if status != "used" || !valid {
			return guestaccess.ErrRecoveryDenied
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM guest_access_recovery WHERE voucher_id=$1 AND business_id=$2 AND device_id=$3)`, g.VoucherID, g.BusinessID, g.DeviceID).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil // Lost cookies require owner repair, not credential replacement.
		}
		var err error
		secret, err = r.IssueTXN(tx, g.BusinessID, g.VoucherID, g.DeviceID)
		return err
	})
	if err != nil {
		return "", err // Never disclose plaintext after an ambiguous commit.
	}
	return secret, nil
}

func (r GuestRecoveryRepository) transaction(ctx context.Context, biz string, fn func(*sql.Tx) error) error {
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = WithTenant(tx, biz, func() error { return fn(tx) }); err != nil {
		return err
	}
	return tx.Commit()
}

// IssueTXN is called only after the original router grant is verified. It never
// issues a credential for a pending, revoked, expired or foreign purchase.
func (r GuestRecoveryRepository) IssueTXN(tx *sql.Tx, biz, voucherID, deviceID string) (string, error) {
	secret, err := guestaccess.RecoverySecret()
	if err != nil {
		return "", err
	}
	hash, _ := guestaccess.HashSecret(secret)
	result, err := tx.Exec(`INSERT INTO guest_access_recovery(voucher_id,business_id,device_id,secret_hash,current_mac)
 SELECT g.voucher_id,g.business_id,g.device_id,$4,g.client_mac FROM guest_voucher_grants g
 JOIN vouchers v ON v.id=g.voucher_id AND v.business_id=g.business_id
 WHERE g.voucher_id=$1 AND g.business_id=$2 AND g.device_id=$3 AND g.state='active'
 AND g.expires_at>clock_timestamp() AND v.status='used'`, voucherID, biz, deviceID, hash)
	if err != nil {
		return "", err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return "", guestaccess.ErrRecoveryDenied
	}
	return secret, nil
}

func (r GuestRecoveryRepository) Reserve(ctx context.Context, req guestaccess.RecoveryRequest, hash []byte) (g guestaccess.RecoveryClaim, err error) {
	mac, e := net.ParseMAC(req.NewMAC)
	if e != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" || len(hash) != 32 {
		return g, guestaccess.ErrRecoveryDenied
	}
	req.NewMAC = strings.ToUpper(mac.String())
	err = r.transaction(ctx, req.BusinessID, func(tx *sql.Tx) error {
		var stored []byte
		var state, voucherState, grantState string
		var count int
		var window, now time.Time
		// Lock the voucher explicitly first, as issuance/revoke/finalize do.
		if e := tx.QueryRowContext(ctx, `SELECT status FROM vouchers WHERE id=$1 AND business_id=$2 FOR UPDATE`, req.VoucherID, req.BusinessID).Scan(&voucherState); e != nil {
			return e
		}
		e := tx.QueryRowContext(ctx, `SELECT r.secret_hash,r.current_mac::text,r.state,r.used_bytes,r.transfer_count,r.window_started_at,
 g.expires_at,g.data_mb::bigint*1000000,g.state,clock_timestamp()
 FROM guest_voucher_grants g
 JOIN guest_access_recovery r ON r.voucher_id=g.voucher_id AND r.business_id=g.business_id AND r.device_id=g.device_id
 WHERE g.voucher_id=$1 AND g.business_id=$2 AND g.device_id=$3 FOR UPDATE OF g,r`, req.VoucherID, req.BusinessID, req.DeviceID).
			Scan(&stored, &g.OldMAC, &state, &g.UsedBytes, &count, &window, &g.ExpiresAt, &g.TotalBytes, &grantState, &now)
		if e != nil {
			return e
		}
		g.OldMAC = strings.ToUpper(g.OldMAC)
		if subtle.ConstantTimeCompare(stored, hash) != 1 || state != "active" || grantState != "active" || voucherState != "used" || !now.Before(g.ExpiresAt) || g.UsedBytes >= g.TotalBytes || g.OldMAC == req.NewMAC {
			return guestaccess.ErrRecoveryDenied
		}
		if !now.Before(window.Add(6 * time.Hour)) {
			count = 0
			window = now
		}
		if count >= 3 {
			return guestaccess.ErrRecoveryDenied
		}
		g.BusinessID, g.DeviceID, g.VoucherID, g.NewMAC = req.BusinessID, req.DeviceID, req.VoucherID, req.NewMAC
		return tx.QueryRowContext(ctx, `UPDATE guest_access_recovery SET state='transferring',pending_mac=$3::macaddr,
 transfer_id=gen_random_uuid(),transfer_count=$4,window_started_at=$5,updated_at=clock_timestamp()
 WHERE voucher_id=$1 AND business_id=$2 RETURNING transfer_id::text`, g.VoucherID, g.BusinessID, g.NewMAC, count+1, window).Scan(&g.TransferID)
	})
	if err != nil {
		return guestaccess.RecoveryClaim{}, guestaccess.ErrRecoveryDenied
	}
	return g, nil
}

func (r GuestRecoveryRepository) FreezeUsage(ctx context.Context, g guestaccess.RecoveryClaim, consumed int64) (guestaccess.RecoveryClaim, error) {
	if consumed < 0 || consumed >= g.TotalBytes-g.UsedBytes {
		return g, guestaccess.ErrRecoveryDenied
	}
	err := r.transaction(ctx, g.BusinessID, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `UPDATE guest_access_recovery r SET state='frozen',used_bytes=r.used_bytes+$4,updated_at=clock_timestamp()
 FROM guest_voucher_grants g,vouchers v WHERE r.voucher_id=$1 AND r.business_id=$2 AND r.transfer_id=$3::uuid
 AND r.state='transferring' AND r.used_bytes=$5 AND r.current_mac=$6::macaddr AND r.pending_mac=$7::macaddr
 AND g.voucher_id=r.voucher_id AND g.business_id=r.business_id AND g.device_id=$8 AND g.state='active'
 AND g.expires_at=$9 AND g.expires_at>clock_timestamp() AND g.data_mb::bigint*1000000=$10
 AND r.used_bytes+$4<g.data_mb::bigint*1000000 AND v.id=g.voucher_id AND v.business_id=g.business_id AND v.status='used'
 RETURNING r.used_bytes`, g.VoucherID, g.BusinessID, g.TransferID, consumed, g.UsedBytes, g.OldMAC, g.NewMAC, g.DeviceID, g.ExpiresAt, g.TotalBytes).Scan(&g.UsedBytes)
	})
	return g, err
}

func (r GuestRecoveryRepository) Complete(ctx context.Context, g guestaccess.RecoveryClaim) error {
	return r.transaction(ctx, g.BusinessID, func(tx *sql.Tx) error {
		// Match reserve lock order; serialize the final commit against revocation.
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM vouchers WHERE id=$1 AND business_id=$2 FOR UPDATE`, g.VoucherID, g.BusinessID).Scan(&status); err != nil {
			return err
		}
		if status != "used" {
			return guestaccess.ErrRecoveryDenied
		}
		result, err := tx.ExecContext(ctx, `UPDATE guest_access_recovery r SET state='active',completed_transfer_id=transfer_id,current_mac=pending_mac,pending_mac=NULL,transfer_id=NULL,updated_at=clock_timestamp()
 FROM guest_voucher_grants g WHERE r.voucher_id=$1 AND r.business_id=$2 AND r.transfer_id=$3::uuid AND r.state='frozen'
 AND r.used_bytes=$4 AND r.current_mac=$5::macaddr AND r.pending_mac=$6::macaddr
 AND g.voucher_id=r.voucher_id AND g.business_id=r.business_id AND g.device_id=$7 AND g.state='active'
 AND g.expires_at=$8 AND g.expires_at>clock_timestamp() AND g.data_mb::bigint*1000000=$9`, g.VoucherID, g.BusinessID, g.TransferID, g.UsedBytes, g.OldMAC, g.NewMAC, g.DeviceID, g.ExpiresAt, g.TotalBytes)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return guestaccess.ErrRecoveryDenied
		}
		return nil
	})
}

func (r GuestRecoveryRepository) Block(ctx context.Context, g guestaccess.RecoveryClaim) error {
	return r.transaction(ctx, g.BusinessID, func(tx *sql.Tx) error {
		// Serialize with reserve/complete; no router I/O under this lock. Receipt
		// equality prevents delayed cleanup from fencing a newer completed transfer.
		var id string
		if err := tx.QueryRowContext(ctx, `SELECT id::text FROM vouchers WHERE id=$1 AND business_id=$2 FOR UPDATE`, g.VoucherID, g.BusinessID).Scan(&id); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE guest_access_recovery r SET state='blocked',
 pending_mac=CASE WHEN r.state='active' THEN $5::macaddr ELSE r.pending_mac END,
 transfer_id=CASE WHEN r.state='active' THEN $3::uuid ELSE r.transfer_id END,updated_at=clock_timestamp()
 FROM guest_voucher_grants g WHERE r.voucher_id=$1 AND r.business_id=$2 AND r.device_id=$4
 AND g.voucher_id=r.voucher_id AND g.business_id=r.business_id AND g.device_id=r.device_id
 AND g.expires_at=$8 AND g.data_mb::bigint*1000000=$9
 AND ((r.transfer_id=$3::uuid AND r.state IN ('transferring','frozen') AND r.current_mac=$5::macaddr AND r.pending_mac=$6::macaddr)
 OR (r.state='active' AND r.completed_transfer_id=$3::uuid AND r.current_mac=$6::macaddr AND r.used_bytes=$7))`,
			g.VoucherID, g.BusinessID, g.TransferID, g.DeviceID, g.OldMAC, g.NewMAC, g.UsedBytes, g.ExpiresAt, g.TotalBytes)
		return err
	})
}
