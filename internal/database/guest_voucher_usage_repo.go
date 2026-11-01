package database

import (
	"database/sql"
	"time"
)

// GuestVoucherUsage is the latest router-observed usage for one voucher.
type GuestVoucherUsage struct {
	VoucherID     string    `json:"voucher_id"`
	BytesUsed     int64     `json:"bytes_used"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	ObservedAt    time.Time `json:"observed_at"`
}

// GuestVoucherUsageTableExists reports whether the operator-applied usage
// migration is present. Callers skip usage storage when it is not, so the
// release can ship before the migration is applied.
func GuestVoucherUsageTableExists(tx *sql.Tx) bool {
	var ok bool
	if err := tx.QueryRow(`SELECT to_regclass('public.guest_voucher_usage') IS NOT NULL`).Scan(&ok); err != nil {
		return false
	}
	return ok
}

// UpsertUsageTXN records the router's counters for a voucher. Counters never
// move backwards: a user recreated on the router with zeroed counters must not
// erase usage that was already observed.
func (GuestVoucherRepository) UpsertUsageTXN(tx *sql.Tx, businessID, voucherID, deviceID string, bytesUsed, uptimeSeconds int64, observedAt time.Time) error {
	_, err := tx.Exec(`INSERT INTO guest_voucher_usage(business_id,voucher_id,device_id,bytes_used,uptime_seconds,observed_at)
		VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT(business_id,voucher_id) DO UPDATE SET
		 device_id=EXCLUDED.device_id,
		 bytes_used=GREATEST(guest_voucher_usage.bytes_used,EXCLUDED.bytes_used),
		 uptime_seconds=GREATEST(guest_voucher_usage.uptime_seconds,EXCLUDED.uptime_seconds),
		 observed_at=EXCLUDED.observed_at, updated_at=clock_timestamp()`,
		businessID, voucherID, deviceID, bytesUsed, uptimeSeconds, observedAt)
	return err
}

// UsageForBatchTXN returns stored usage keyed by voucher id for one issue batch.
// It returns an empty map when the migration has not been applied.
func (GuestVoucherRepository) UsageForBatchTXN(tx *sql.Tx, businessID, batchID string) (map[string]GuestVoucherUsage, error) {
	out := map[string]GuestVoucherUsage{}
	if !GuestVoucherUsageTableExists(tx) {
		return out, nil
	}
	rows, err := tx.Query(`SELECT u.voucher_id,u.bytes_used,u.uptime_seconds,u.observed_at
		FROM guest_voucher_usage u JOIN guest_voucher_issue_items i ON i.business_id=u.business_id AND i.voucher_id=u.voucher_id
		WHERE u.business_id=$1 AND i.batch_id=$2`, businessID, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var u GuestVoucherUsage
		if err := rows.Scan(&u.VoucherID, &u.BytesUsed, &u.UptimeSeconds, &u.ObservedAt); err != nil {
			return nil, err
		}
		out[u.VoucherID] = u
	}
	return out, rows.Err()
}
