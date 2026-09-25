package database

import (
	"database/sql"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

// VoucherRepository handles voucher database operations
type VoucherRepository struct {
	db *DB
}

// NewVoucherRepository creates a new voucher repository
func NewVoucherRepository(db *DB) *VoucherRepository {
	return &VoucherRepository{db: db}
}

// CreateVoucher inserts a new voucher record
func (r *VoucherRepository) CreateVoucher(voucher *model.Voucher) (*model.Voucher, error) {
	query := `
		INSERT INTO vouchers (business_id, device_id, plan_key, plan_label, code,
		                      price_mmk, duration_minutes, device_mac, status,
		                      first_seen_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, business_id, device_id, plan_key, plan_label, code,
		          price_mmk, duration_minutes, device_mac, status,
		          first_seen_at, expires_at, created_at, updated_at`

	v := &model.Voucher{}
	var deviceID, deviceMAC sql.NullString
	var durationMin sql.NullInt64
	var firstSeen, expires sql.NullTime

	err := r.db.QueryRow(query,
		voucher.BusinessID,
		nullString(voucher.DeviceID),
		voucher.PlanKey,
		voucher.PlanLabel,
		voucher.Code,
		voucher.PriceMMK,
		nullableInt(voucher.DurationMinutes),
		macAddrStr(voucher.DeviceMAC),
		voucher.Status,
		nullTime(voucher.FirstSeenAt),
		nullTime(voucher.ExpiresAt),
	).Scan(
		&v.ID, &v.BusinessID, &deviceID, &v.PlanKey, &v.PlanLabel, &v.Code,
		&v.PriceMMK, &durationMin, &deviceMAC, &v.Status,
		&firstSeen, &expires, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create voucher: %w", err)
	}

	v.DeviceID = scanNullableString(deviceID)
	v.DurationMinutes = scanNullableInt(durationMin)
	v.DeviceMAC = scanMACAddr(deviceMAC)
	v.FirstSeenAt = scanNullableTime(firstSeen)
	v.ExpiresAt = scanNullableTime(expires)

	return v, nil
}

// GetVoucherByID retrieves a voucher by its ID
func (r *VoucherRepository) GetVoucherByID(id string) (*model.Voucher, error) {
	query := `
		SELECT id, business_id, device_id, plan_key, plan_label, code,
		       price_mmk, duration_minutes, device_mac, status,
		       first_seen_at, expires_at, created_at, updated_at
		FROM vouchers
		WHERE id = $1`

	v := &model.Voucher{}
	var deviceID, deviceMAC sql.NullString
	var durationMin sql.NullInt64
	var firstSeen, expires sql.NullTime

	err := r.db.QueryRow(query, id).Scan(
		&v.ID, &v.BusinessID, &deviceID, &v.PlanKey, &v.PlanLabel, &v.Code,
		&v.PriceMMK, &durationMin, &deviceMAC, &v.Status,
		&firstSeen, &expires, &v.CreatedAt, &v.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get voucher by id: %w", err)
	}

	v.DeviceID = scanNullableString(deviceID)
	v.DurationMinutes = scanNullableInt(durationMin)
	v.DeviceMAC = scanMACAddr(deviceMAC)
	v.FirstSeenAt = scanNullableTime(firstSeen)
	v.ExpiresAt = scanNullableTime(expires)

	return v, nil
}

// ListVouchersByBusiness retrieves all vouchers for a given business
func (r *VoucherRepository) ListVouchersByBusiness(businessID string) ([]*model.Voucher, error) {
	query := `
		SELECT id, business_id, device_id, plan_key, plan_label, code,
		       price_mmk, duration_minutes, device_mac, status,
		       first_seen_at, expires_at, created_at, updated_at
		FROM vouchers
		WHERE business_id = $1
		ORDER BY created_at DESC`

	rows, err := r.db.Query(query, businessID)
	if err != nil {
		return nil, fmt.Errorf("list vouchers by business: %w", err)
	}
	defer rows.Close()

	var vouchers []*model.Voucher
	for rows.Next() {
		v := &model.Voucher{}
		var deviceID, deviceMAC sql.NullString
		var durationMin sql.NullInt64
		var firstSeen, expires sql.NullTime

		err := rows.Scan(
			&v.ID, &v.BusinessID, &deviceID, &v.PlanKey, &v.PlanLabel, &v.Code,
			&v.PriceMMK, &durationMin, &deviceMAC, &v.Status,
			&firstSeen, &expires, &v.CreatedAt, &v.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan voucher row: %w", err)
		}

		v.DeviceID = scanNullableString(deviceID)
		v.DurationMinutes = scanNullableInt(durationMin)
		v.DeviceMAC = scanMACAddr(deviceMAC)
		v.FirstSeenAt = scanNullableTime(firstSeen)
		v.ExpiresAt = scanNullableTime(expires)

		vouchers = append(vouchers, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate voucher rows: %w", err)
	}

	return vouchers, nil
}

// GetVoucherByCode retrieves a voucher by business and code
func (r *VoucherRepository) GetVoucherByCode(businessID, code string) (*model.Voucher, error) {
	query := `
		SELECT id, business_id, device_id, plan_key, plan_label, code,
		       price_mmk, duration_minutes, device_mac, status,
		       first_seen_at, expires_at, created_at, updated_at
		FROM vouchers
		WHERE business_id = $1 AND code = $2`

	v := &model.Voucher{}
	var deviceID, deviceMAC sql.NullString
	var durationMin sql.NullInt64
	var firstSeen, expires sql.NullTime

	err := r.db.QueryRow(query, businessID, code).Scan(
		&v.ID, &v.BusinessID, &deviceID, &v.PlanKey, &v.PlanLabel, &v.Code,
		&v.PriceMMK, &durationMin, &deviceMAC, &v.Status,
		&firstSeen, &expires, &v.CreatedAt, &v.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get voucher by code: %w", err)
	}

	v.DeviceID = scanNullableString(deviceID)
	v.DurationMinutes = scanNullableInt(durationMin)
	v.DeviceMAC = scanMACAddr(deviceMAC)
	v.FirstSeenAt = scanNullableTime(firstSeen)
	v.ExpiresAt = scanNullableTime(expires)

	return v, nil
}

// UpdateVoucherStatus updates the status and optionally first_seen_at for a voucher
func (r *VoucherRepository) UpdateVoucherStatus(id, status string, firstSeenAt *time.Time) (*model.Voucher, error) {
	query := `
		UPDATE vouchers
		SET status = $2, first_seen_at = COALESCE($3, first_seen_at), updated_at = $4
		WHERE id = $1
		RETURNING id, business_id, device_id, plan_key, plan_label, code,
		          price_mmk, duration_minutes, device_mac, status,
		          first_seen_at, expires_at, created_at, updated_at`

	v := &model.Voucher{}
	var deviceID, deviceMAC sql.NullString
	var durationMin sql.NullInt64
	var firstSeen, expires sql.NullTime

	err := r.db.QueryRow(query, id, status, nullTime(firstSeenAt), time.Now().UTC()).Scan(
		&v.ID, &v.BusinessID, &deviceID, &v.PlanKey, &v.PlanLabel, &v.Code,
		&v.PriceMMK, &durationMin, &deviceMAC, &v.Status,
		&firstSeen, &expires, &v.CreatedAt, &v.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update voucher status: %w", err)
	}

	v.DeviceID = scanNullableString(deviceID)
	v.DurationMinutes = scanNullableInt(durationMin)
	v.DeviceMAC = scanMACAddr(deviceMAC)
	v.FirstSeenAt = scanNullableTime(firstSeen)
	v.ExpiresAt = scanNullableTime(expires)

	return v, nil
}

// ClaimVoucherTXN atomically redeems a voucher inside a caller-supplied
// transaction: it flips status 'active' -> 'used' and stamps first_seen_at, but
// ONLY if the code is active and unexpired. It returns nil (no error) when
// nothing was claimed — already used, expired, cancelled, or not found — so a
// concurrent double-redeem can never both win. Running it in the caller's tx
// keeps the claim and the entitlement write (quota) in ONE atomic unit.
func (r *VoucherRepository) ClaimVoucherTXN(tx *sql.Tx, businessID, code string, now time.Time) (*model.Voucher, error) {
	query := `
		UPDATE vouchers
		SET status = 'used', first_seen_at = COALESCE(first_seen_at, $3), updated_at = $3
		WHERE business_id = $1 AND code = $2
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > $3)
		RETURNING id, business_id, device_id, plan_key, plan_label, code,
		          price_mmk, duration_minutes, device_mac, status,
		          first_seen_at, expires_at, created_at, updated_at`

	v := &model.Voucher{}
	var deviceID, deviceMAC sql.NullString
	var durationMin sql.NullInt64
	var firstSeen, expires sql.NullTime

	err := tx.QueryRow(query, businessID, code, now).Scan(
		&v.ID, &v.BusinessID, &deviceID, &v.PlanKey, &v.PlanLabel, &v.Code,
		&v.PriceMMK, &durationMin, &deviceMAC, &v.Status,
		&firstSeen, &expires, &v.CreatedAt, &v.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim voucher: %w", err)
	}

	v.DeviceID = scanNullableString(deviceID)
	v.DurationMinutes = scanNullableInt(durationMin)
	v.DeviceMAC = scanMACAddr(deviceMAC)
	v.FirstSeenAt = scanNullableTime(firstSeen)
	v.ExpiresAt = scanNullableTime(expires)

	return v, nil
}

// UpdateVoucherStatusByBusiness is the tenant-scoped variant of
// UpdateVoucherStatus: the business_id filter prevents cross-tenant edits by id.
func (r *VoucherRepository) UpdateVoucherStatusByBusiness(businessID, id, status string, firstSeenAt *time.Time) (*model.Voucher, error) {
	query := `
		UPDATE vouchers
		SET status = $3, first_seen_at = COALESCE($4, first_seen_at), updated_at = $5
		WHERE id = $1 AND business_id = $2
		RETURNING id, business_id, device_id, plan_key, plan_label, code,
		          price_mmk, duration_minutes, device_mac, status,
		          first_seen_at, expires_at, created_at, updated_at`

	v := &model.Voucher{}
	var deviceID, deviceMAC sql.NullString
	var durationMin sql.NullInt64
	var firstSeen, expires sql.NullTime

	err := r.db.QueryRow(query, id, businessID, status, nullTime(firstSeenAt), time.Now().UTC()).Scan(
		&v.ID, &v.BusinessID, &deviceID, &v.PlanKey, &v.PlanLabel, &v.Code,
		&v.PriceMMK, &durationMin, &deviceMAC, &v.Status,
		&firstSeen, &expires, &v.CreatedAt, &v.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update voucher status: %w", err)
	}

	v.DeviceID = scanNullableString(deviceID)
	v.DurationMinutes = scanNullableInt(durationMin)
	v.DeviceMAC = scanMACAddr(deviceMAC)
	v.FirstSeenAt = scanNullableTime(firstSeen)
	v.ExpiresAt = scanNullableTime(expires)

	return v, nil
}

// DeleteVoucherIfUnused deletes a voucher only when it has NOT been redeemed
// (status <> 'used'). Redeemed records are never destroyed. Returns false when
// nothing was deleted (not found, or already used).
func (r *VoucherRepository) DeleteVoucherIfUnused(businessID, id string) (bool, error) {
	var deletedID string
	err := r.db.QueryRow(
		`DELETE FROM vouchers WHERE id = $1 AND business_id = $2 AND status <> 'used' RETURNING id`,
		id, businessID,
	).Scan(&deletedID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("delete voucher: %w", err)
	}
	return true, nil
}

// nullableInt converts *int to *int for SQL binding (nil passthrough)
func nullableInt(i *int) *int {
	if i == nil {
		return nil
	}
	return i
}
