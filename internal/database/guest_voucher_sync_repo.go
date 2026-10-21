package database

import (
	"database/sql"
	"time"

	"mega-magic/internal/model"
)

// Ground-operation voucher-login synchronization: the RouterBOARD validates
// issued codes locally and this sync applies its live accounting to the
// app's grant records. Each transition is guarded by the same CHECK and
// trigger contract as the hosted redemption flow.

// GetGrantTXN reads one business-scoped guest voucher grant.
func (GuestVoucherRepository) GetGrantTXN(tx *sql.Tx, businessID, voucherID string) (*model.GuestVoucherGrant, error) {
	row := tx.QueryRow(`SELECT `+guestVoucherCols+` FROM guest_voucher_grants WHERE business_id=$1 AND voucher_id=$2`, businessID, voucherID)
	g, err := scanGuestVoucher(row)
	if err != nil {
		return nil, err
	}
	return g, nil
}

// SyncClaimTXN moves an issued, never-used grant into its live provisioning
// claim: the router is serving this voucher's session from the observed
// client right now, so validity starts at this sync's observation.
func (GuestVoucherRepository) SyncClaimTXN(tx *sql.Tx, businessID, voucherID, clientMAC string, now time.Time, minutes int) error {
	if minutes < 1 || minutes > 525600 {
		return ErrGuestVoucherUnavailable
	}
	result, err := tx.Exec(`UPDATE guest_voucher_grants SET state='provisioning', client_mac=$3, first_used_at=$4::timestamptz,
		expires_at=$4::timestamptz + ($5::int * interval '1 minute'), lease_token=$2, lease_until=$4::timestamptz + interval '5 minutes', updated_at=now()
		WHERE business_id=$1 AND voucher_id=$2 AND state='unused' AND client_mac IS NULL AND first_used_at IS NULL`,
		businessID, voucherID, clientMAC, now, minutes)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrGuestVoucherUnavailable
	}
	return nil
}

// SyncActivateTXN anchors the immutable activation timestamp on a live
// provisioning claim. The activation guard enforces the same contract as a
// hosted redemption: only a provisioning -> active transition may set it,
// and only while the allowance is still live.
func (GuestVoucherRepository) SyncActivateTXN(tx *sql.Tx, businessID, voucherID string, now time.Time) error {
	result, err := tx.Exec(`UPDATE guest_voucher_grants SET state='active', activated_at=$3::timestamptz, lease_token=NULL, lease_until=NULL, updated_at=now()
		WHERE business_id=$1 AND voucher_id=$2 AND state='provisioning' AND activated_at IS NULL
		AND first_used_at IS NOT NULL AND expires_at IS NOT NULL AND $3::timestamptz < expires_at`,
		businessID, voucherID, now)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrGuestVoucherUnavailable
	}
	return nil
}

// SyncExpireTXN records router-observed expiry on an active grant without
// touching the immutable activation timestamp.
func (GuestVoucherRepository) SyncExpireTXN(tx *sql.Tx, businessID, voucherID string) error {
	result, err := tx.Exec(`UPDATE guest_voucher_grants SET state='expired', updated_at=now()
		WHERE business_id=$1 AND voucher_id=$2 AND state='active'`, businessID, voucherID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrGuestVoucherUnavailable
	}
	return nil
}

// ClaimVoucherByRouterTXN marks a locally validated voucher used from the
// router's own accounting observation (ground-operation sync). It is the
// router-observed equivalent of the hosted first-use claim: idempotent for an
// already-claimed voucher, refused for a voucher that expired before use.
func (r *VoucherRepository) ClaimVoucherByRouterTXN(tx *sql.Tx, businessID, id string, now time.Time) (bool, error) {
	result, err := tx.Exec(`UPDATE vouchers SET status='used', first_seen_at=COALESCE(first_seen_at,$3), updated_at=$3
		WHERE business_id=$1 AND id=$2 AND status IN ('unused','active') AND (expires_at IS NULL OR expires_at > $3)`,
		businessID, id, now)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
