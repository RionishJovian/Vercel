package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"time"

	"mega-magic/internal/model"
)

var ErrGuestVoucherUnavailable = errors.New("guest voucher is unavailable for this client and gateway")
var ErrGuestVoucherBusy = errors.New("guest voucher provisioning is already in progress")

const guestVoucherCols = `voucher_id,business_id,device_id,minutes,data_mb,state,
 client_mac::text,first_used_at,expires_at,lease_token,lease_until`

// All methods require a transaction already scoped with WithTenant.
type GuestVoucherRepository struct{}

func scanGuestVoucher(row guestRequestScan) (*model.GuestVoucherGrant, error) {
	g := &model.GuestVoucherGrant{}
	var mac, token sql.NullString
	var first, expires, lease sql.NullTime
	err := row.Scan(&g.VoucherID, &g.BusinessID, &g.DeviceID, &g.Minutes, &g.DataMB, &g.State,
		&mac, &first, &expires, &token, &lease)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.ClientMAC = strings.ToUpper(mac.String)
	g.LeaseToken = token.String
	g.FirstUsedAt = scanNullableTime(first)
	g.ExpiresAt = scanNullableTime(expires)
	g.LeaseUntil = scanNullableTime(lease)
	return g, nil
}

func (GuestVoucherRepository) IssueTXN(tx *sql.Tx, businessID, voucherID, deviceID string, minutes, dataMB int) error {
	if minutes < 1 || minutes > 525600 || dataMB < 1 || dataMB > 1048576 {
		return ErrGuestVoucherUnavailable
	}
	result, err := tx.Exec(`INSERT INTO guest_voucher_grants(voucher_id,business_id,device_id,minutes,data_mb)
 SELECT v.id,v.business_id,d.device_id,$4,$5 FROM vouchers v JOIN devices d ON d.business_id=v.business_id
 WHERE v.id=$1 AND v.business_id=$2 AND d.device_id=$3
 AND v.status IN ('active','unused') AND v.first_seen_at IS NULL AND v.expires_at IS NULL`, voucherID, businessID, deviceID, minutes, dataMB)
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

// ReserveTXN serializes on the code, commits a durable claim/short worker lease,
// then the caller must COMMIT before touching the router. Router failure must
// not release this claim for another MAC or replenish its allowance.
func (GuestVoucherRepository) ReserveTXN(tx *sql.Tx, businessID, deviceID, code, clientMAC string) (*model.GuestVoucherGrant, error) {
	mac, err := net.ParseMAC(clientMAC)
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
		return nil, ErrGuestVoucherUnavailable
	}
	clientMAC = strings.ToUpper(mac.String())
	var id, status string
	err = tx.QueryRow(`SELECT id,status FROM vouchers WHERE business_id=$1 AND code=$2 FOR UPDATE`, businessID, strings.ToUpper(strings.TrimSpace(code))).Scan(&id, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGuestVoucherUnavailable
	}
	if err != nil {
		return nil, err
	}
	g, err := scanGuestVoucher(tx.QueryRow(`SELECT `+guestVoucherCols+` FROM guest_voucher_grants
 WHERE voucher_id=$1 AND business_id=$2 AND device_id=$3 FOR UPDATE`, id, businessID, deviceID))
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, ErrGuestVoucherUnavailable
	}
	var now time.Time
	if err = tx.QueryRow(`SELECT clock_timestamp()`).Scan(&now); err != nil {
		return nil, err
	}
	if err = guestVoucherClaimAllowed(g, status, clientMAC, now); err != nil {
		return nil, err
	}
	if g.State == "active" {
		return g, nil
	}
	var token [16]byte
	if _, err = rand.Read(token[:]); err != nil {
		return nil, err
	}
	if g.State == "unused" {
		if _, err = tx.Exec(`UPDATE vouchers SET status='used',first_seen_at=$3::timestamptz,
 expires_at=$3::timestamptz+$4::integer*interval '1 minute',updated_at=$3::timestamptz WHERE id=$1 AND business_id=$2`, id, businessID, now, g.Minutes); err != nil {
			return nil, err
		}
	}
	return scanGuestVoucher(tx.QueryRow(`UPDATE guest_voucher_grants SET state='provisioning',
 client_mac=COALESCE(client_mac,$3::macaddr),first_used_at=COALESCE(first_used_at,$4::timestamptz),
 expires_at=COALESCE(expires_at,$4::timestamptz+minutes*interval '1 minute'),lease_token=$5,
 lease_until=$4::timestamptz+interval '2 minutes',updated_at=$4::timestamptz
 WHERE voucher_id=$1 AND business_id=$2 RETURNING `+guestVoucherCols, id, businessID, clientMAC, now, hex.EncodeToString(token[:])))
}

func guestVoucherClaimAllowed(g *model.GuestVoucherGrant, status, mac string, now time.Time) error {
	if g.State == "unused" && (status == "active" || status == "unused") {
		return nil
	}
	if status != "used" || g.ClientMAC != mac || g.ExpiresAt == nil || !now.Before(*g.ExpiresAt) {
		return ErrGuestVoucherUnavailable
	}
	if g.State == "active" {
		return nil
	}
	if g.State != "provisioning" {
		return ErrGuestVoucherUnavailable
	}
	if g.LeaseUntil != nil && now.Before(*g.LeaseUntil) {
		return ErrGuestVoucherBusy
	}
	return nil
}

// ReleaseWorkerTXN permits the bound client to retry after a confirmed failed
// attempt without waiting for the whole lease (which may exceed a short
// voucher's validity). It does NOT unclaim the voucher, move its deadline or
// change its MAC. A stale worker cannot release a replacement worker's lease.
func (GuestVoucherRepository) ReleaseWorkerTXN(tx *sql.Tx, businessID, voucherID, leaseToken string) (bool, error) {
	result, err := tx.Exec(`UPDATE guest_voucher_grants SET lease_until=clock_timestamp(),updated_at=clock_timestamp()
 WHERE voucher_id=$1 AND business_id=$2 AND state='provisioning' AND lease_token=$3`, voucherID, businessID, leaseToken)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// FinalizeTXN must only be called after quota, client binding and expiry were
// read back from the router. A revoked/expired code or stale worker cannot win.
func (GuestVoucherRepository) FinalizeTXN(tx *sql.Tx, businessID, voucherID, leaseToken string) (bool, error) {
	// Match ReserveTXN's lock order and serialize with revoke/delete operations.
	var status string
	err := tx.QueryRow(`SELECT status FROM vouchers WHERE id=$1 AND business_id=$2 FOR UPDATE`, voucherID, businessID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) || status != "used" && err == nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	result, err := tx.Exec(`UPDATE guest_voucher_grants g SET state='active',activated_at=COALESCE(g.activated_at,clock_timestamp()),lease_token=NULL,lease_until=NULL,updated_at=clock_timestamp()
 FROM vouchers v WHERE g.voucher_id=$1 AND g.business_id=$2 AND g.lease_token=$3
 AND g.state='provisioning' AND g.expires_at>clock_timestamp() AND g.lease_until>clock_timestamp()
 AND v.id=g.voucher_id AND v.business_id=g.business_id AND v.status='used'`, voucherID, businessID, leaseToken)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
