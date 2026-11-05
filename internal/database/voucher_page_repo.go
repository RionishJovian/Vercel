package database

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"mega-magic/internal/model"
)

// VoucherListFilter narrows one page of a business's vouchers. Empty fields do
// not filter. Status never narrows StatusCounts, so a screen can show the count
// of every status while one is selected.
type VoucherListFilter struct {
	Status  string
	Query   string
	BatchID string
	Codes   []string
	Limit   int
	Offset  int
}

// VoucherPage is one page of vouchers plus the totals needed to page through them.
type VoucherPage struct {
	Vouchers     []*model.Voucher
	Total        int
	StatusCounts map[string]int
}

func likeContains(s string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s) + "%"
}

// ListVouchersPageTXN reads one page. The id tie-break keeps pages stable because
// every voucher of a batch shares its created_at.
func (r *VoucherRepository) ListVouchersPageTXN(tx *sql.Tx, businessID string, f VoucherListFilter) (*VoucherPage, error) {
	where := []string{"business_id = $1"}
	args := []any{businessID}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.Query != "" {
		args = append(args, likeContains(f.Query))
		where = append(where, fmt.Sprintf("(code ILIKE $%[1]d OR plan_label ILIKE $%[1]d)", len(args)))
	}
	if len(f.Codes) > 0 {
		add("code = ANY($%d)", pq.Array(f.Codes))
	}
	if f.BatchID != "" {
		var exists bool
		if err := tx.QueryRow(`SELECT to_regclass('public.guest_voucher_issue_items') IS NOT NULL`).Scan(&exists); err != nil {
			return nil, fmt.Errorf("check voucher batch items: %w", err)
		}
		if !exists {
			return &VoucherPage{Vouchers: []*model.Voucher{}, StatusCounts: map[string]int{}}, nil
		}
		add("id IN (SELECT voucher_id FROM guest_voucher_issue_items WHERE business_id = $1 AND batch_id = $%d)", f.BatchID)
	}

	page := &VoucherPage{Vouchers: []*model.Voucher{}, StatusCounts: map[string]int{}}
	rows, err := tx.Query(`SELECT status, count(*) FROM vouchers WHERE `+strings.Join(where, " AND ")+` GROUP BY status`, args...)
	if err != nil {
		return nil, fmt.Errorf("count vouchers: %w", err)
	}
	all := 0
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan voucher count: %w", err)
		}
		page.StatusCounts[status] = n
		all += n
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("iterate voucher counts: %w", err)
	}
	page.Total = all
	if f.Status != "" {
		page.Total = page.StatusCounts[f.Status]
		add("status = $%d", f.Status)
	}

	args = append(args, f.Limit, f.Offset)
	rows, err = tx.Query(`SELECT id, business_id, device_id, plan_key, plan_label, code,
		       price_mmk, duration_minutes, device_mac, status,
		       first_seen_at, expires_at, created_at, updated_at
		FROM vouchers WHERE `+strings.Join(where, " AND ")+
		fmt.Sprintf(` ORDER BY created_at DESC, id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list voucher page: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		v := &model.Voucher{}
		var deviceID, deviceMAC sql.NullString
		var durationMin sql.NullInt64
		var firstSeen, expires sql.NullTime
		if err := rows.Scan(&v.ID, &v.BusinessID, &deviceID, &v.PlanKey, &v.PlanLabel, &v.Code,
			&v.PriceMMK, &durationMin, &deviceMAC, &v.Status, &firstSeen, &expires, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan voucher row: %w", err)
		}
		v.DeviceID = scanNullableString(deviceID)
		v.DurationMinutes = scanNullableInt(durationMin)
		v.DeviceMAC = scanMACAddr(deviceMAC)
		v.FirstSeenAt = scanNullableTime(firstSeen)
		v.ExpiresAt = scanNullableTime(expires)
		page.Vouchers = append(page.Vouchers, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate voucher page: %w", err)
	}
	return page, nil
}

// UsageForVouchersTXN returns stored usage keyed by voucher id. It returns an
// empty map when the usage migration has not been applied.
func (GuestVoucherRepository) UsageForVouchersTXN(tx *sql.Tx, businessID string, ids []string) (map[string]GuestVoucherUsage, error) {
	out := map[string]GuestVoucherUsage{}
	if len(ids) == 0 || !GuestVoucherUsageTableExists(tx) {
		return out, nil
	}
	rows, err := tx.Query(`SELECT voucher_id, bytes_used, uptime_seconds, observed_at FROM guest_voucher_usage
		WHERE business_id = $1 AND voucher_id = ANY($2)`, businessID, pq.Array(ids))
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

// DataLimitsForVouchersTXN returns each guest voucher's data allowance in MB.
func (GuestVoucherRepository) DataLimitsForVouchersTXN(tx *sql.Tx, businessID string, ids []string) (map[string]int, error) {
	out := map[string]int{}
	var exists bool
	if err := tx.QueryRow(`SELECT to_regclass('public.guest_voucher_grants') IS NOT NULL`).Scan(&exists); err != nil {
		return nil, err
	}
	if len(ids) == 0 || !exists {
		return out, nil
	}
	rows, err := tx.Query(`SELECT voucher_id, data_mb FROM guest_voucher_grants
		WHERE business_id = $1 AND voucher_id = ANY($2)`, businessID, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var mb int
		if err := rows.Scan(&id, &mb); err != nil {
			return nil, err
		}
		out[id] = mb
	}
	return out, rows.Err()
}
