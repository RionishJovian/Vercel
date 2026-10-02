package database

import (
	"context"
	"database/sql"

	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

type GuestProvisionRepository struct{ DB *DB }

var _ guestaccess.ProvisionStore = GuestProvisionRepository{}

func (r GuestProvisionRepository) transaction(ctx context.Context, biz string, fn func(*sql.Tx) error) error {
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

func (r GuestProvisionRepository) Reserve(ctx context.Context, req guestaccess.ProvisionRequest) (g model.GuestVoucherGrant, err error) {
	err = r.transaction(ctx, req.BusinessID, func(tx *sql.Tx) error {
		v, e := (GuestVoucherRepository{}).ReserveTXN(tx, req.BusinessID, req.DeviceID, req.Code, req.ClientMAC)
		if e != nil {
			return e
		}
		// A code proves first redemption, not ownership of a returning paid
		// connection. Never hand an active original-MAC grant to the provisioning
		// adapter: its allowance is not the current transferred binding's quota.
		// Deny ALL active code retries, including before a recovery row exists,
		// so credential issuance and transfer cannot race a stale provisioning
		// worker. Returning guests use cookie-authenticated binding recovery.
		if v.State == "active" {
			return ErrGuestVoucherUnavailable
		}
		g = *v
		return nil
	})
	return g, err
}

func (r GuestProvisionRepository) Finalize(ctx context.Context, g model.GuestVoucherGrant) (ok bool, err error) {
	err = r.transaction(ctx, g.BusinessID, func(tx *sql.Tx) error {
		if g.State != "active" {
			var e error
			ok, e = (GuestVoucherRepository{}).FinalizeTXN(tx, g.BusinessID, g.VoucherID, g.LeaseToken)
			return e
		}
		// Active retries do not reset usage, but must still serialize with revoke.
		var status string
		if e := tx.QueryRowContext(ctx, `SELECT status FROM vouchers WHERE id=$1 AND business_id=$2 FOR UPDATE`, g.VoucherID, g.BusinessID).Scan(&status); e != nil {
			return e
		}
		if status != "used" {
			return nil
		}
		return tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM guest_voucher_grants
 WHERE voucher_id=$1 AND business_id=$2 AND device_id=$3 AND state='active'
 AND client_mac=$4::macaddr AND expires_at=$5 AND expires_at>clock_timestamp()
 AND minutes=$6 AND data_mb=$7)`, g.VoucherID, g.BusinessID, g.DeviceID, g.ClientMAC, g.ExpiresAt, g.Minutes, g.DataMB).Scan(&ok)
	})
	return ok, err
}

func (r GuestProvisionRepository) Block(ctx context.Context, g model.GuestVoucherGrant) error {
	return r.transaction(ctx, g.BusinessID, func(tx *sql.Tx) error {
		// Match reserve/finalize lock order. Uncertain router writes permanently
		// fence this grant; repair must be an explicit audited workflow.
		var id string
		if e := tx.QueryRowContext(ctx, `SELECT id FROM vouchers WHERE id=$1 AND business_id=$2 FOR UPDATE`, g.VoucherID, g.BusinessID).Scan(&id); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, `UPDATE guest_voucher_grants SET state='revoked',lease_token=NULL,
 lease_until=NULL,updated_at=clock_timestamp() WHERE voucher_id=$1 AND business_id=$2
 AND device_id=$3 AND client_mac=$4::macaddr AND state IN ('provisioning','active')`, g.VoucherID, g.BusinessID, g.DeviceID, g.ClientMAC)
		return e
	})
}
