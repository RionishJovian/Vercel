package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var ErrBatchAssignmentChanged = errors.New("batch shop assignment changed; refresh before retrying")
var ErrBatchResellerUnavailable = errors.New("active reseller not found in this workspace")

type GuestBatchReseller struct {
	BatchID    string     `json:"batch_id"`
	ResellerID string     `json:"reseller_id"`
	ShopName   string     `json:"shop_name"`
	CodePrefix string     `json:"code_prefix"`
	AssignedAt *time.Time `json:"assigned_at,omitempty"`
}
type GuestBatchStock struct {
	GuestBatchReseller
	DeviceID       string    `json:"device_id"`
	Label          string    `json:"plan_label"`
	TotalCount     int       `json:"total_count"`
	InstalledCount int       `json:"installed_count"`
	ActivatedCount int       `json:"activated_count"`
	CreatedAt      time.Time `json:"created_at"`
}

func GetGuestBatchResellerTXN(tx *sql.Tx, biz, batch string) (GuestBatchReseller, error) {
	out := GuestBatchReseller{BatchID: batch}
	var stamp time.Time
	e := tx.QueryRow(`SELECT reseller_id,shop_name,code_prefix,assigned_at FROM guest_voucher_batch_resellers WHERE business_id=$1 AND batch_id=$2`, biz, batch).Scan(&out.ResellerID, &out.ShopName, &out.CodePrefix, &stamp)
	if errors.Is(e, sql.ErrNoRows) {
		return out, nil
	}
	if e == nil {
		out.AssignedAt = &stamp
	}
	return out, e
}

// Assignment is inventory metadata: never rewrites codes, first-use anchors,
// counters, installation state, prices or payment/activation ledgers.
func AssignGuestBatchResellerTXN(tx *sql.Tx, biz, batch, reseller, expected, actor string) (GuestBatchReseller, error) {
	var id string
	e := tx.QueryRow(`SELECT id FROM guest_voucher_issue_batches WHERE business_id=$1 AND id=$2 FOR UPDATE`, biz, batch).Scan(&id)
	if e != nil {
		return GuestBatchReseller{}, e
	}
	current, e := GetGuestBatchResellerTXN(tx, biz, batch)
	if e != nil {
		return current, e
	}
	if current.ResellerID == reseller {
		return current, nil
	}
	if current.ResellerID != expected {
		return current, ErrBatchAssignmentChanged
	}
	var activeActor bool
	if e = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE business_id=$1 AND user_id=$2 AND is_active)`, biz, actor).Scan(&activeActor); e != nil {
		return current, e
	}
	if !activeActor {
		return current, ErrBatchResellerUnavailable
	}
	name, prefix := "", ""
	if reseller != "" {
		e = tx.QueryRow(`SELECT shop_name,code_prefix FROM voucher_resellers WHERE business_id=$1 AND id=$2 AND active FOR SHARE`, biz, reseller).Scan(&name, &prefix)
		if errors.Is(e, sql.ErrNoRows) {
			return current, ErrBatchResellerUnavailable
		}
		if e != nil {
			return current, e
		}
		_, e = tx.Exec(`INSERT INTO guest_voucher_batch_resellers(business_id,batch_id,reseller_id,shop_name,code_prefix,assigned_by) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(business_id,batch_id) DO UPDATE SET reseller_id=excluded.reseller_id,shop_name=excluded.shop_name,code_prefix=excluded.code_prefix,assigned_by=excluded.assigned_by,assigned_at=clock_timestamp()`, biz, batch, reseller, name, prefix, actor)
	} else {
		_, e = tx.Exec(`DELETE FROM guest_voucher_batch_resellers WHERE business_id=$1 AND batch_id=$2`, biz, batch)
	}
	if e != nil {
		return current, e
	}
	detail, _ := json.Marshal(map[string]string{"action": "assign_shop", "previous_shop": current.ShopName, "shop_name": name, "shop_prefix": prefix})
	if _, e = tx.Exec(`INSERT INTO guest_voucher_batch_reseller_events(business_id,batch_id,actor_id,previous_reseller_id,reseller_id,details) VALUES($1,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6)`, biz, batch, actor, current.ResellerID, reseller, detail); e != nil {
		return current, e
	}
	return GetGuestBatchResellerTXN(tx, biz, batch)
}
func ListGuestBatchStockTXN(tx *sql.Tx, biz string) ([]GuestBatchStock, error) {
	rows, e := tx.Query(`SELECT b.id,b.device_id,b.plan_label,b.total_count,b.created_at,COALESCE(a.reseller_id::text,''),COALESCE(a.shop_name,''),COALESCE(a.code_prefix,''),a.assigned_at,count(i.voucher_id) FILTER(WHERE i.install_state='verified'),count(g.voucher_id) FILTER(WHERE g.activated_at IS NOT NULL)
 FROM guest_voucher_issue_batches b LEFT JOIN guest_voucher_batch_resellers a ON a.business_id=b.business_id AND a.batch_id=b.id LEFT JOIN guest_voucher_issue_items i ON i.business_id=b.business_id AND i.batch_id=b.id LEFT JOIN guest_voucher_grants g ON g.business_id=i.business_id AND g.voucher_id=i.voucher_id WHERE b.business_id=$1 GROUP BY b.business_id,b.id,a.business_id,a.batch_id ORDER BY b.created_at DESC,b.id LIMIT 100`, biz)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []GuestBatchStock{}
	for rows.Next() {
		var b GuestBatchStock
		var at sql.NullTime
		if e = rows.Scan(&b.BatchID, &b.DeviceID, &b.Label, &b.TotalCount, &b.CreatedAt, &b.ResellerID, &b.ShopName, &b.CodePrefix, &at, &b.InstalledCount, &b.ActivatedCount); e != nil {
			return nil, e
		}
		if at.Valid {
			b.AssignedAt = &at.Time
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func CheckGuestBatchResellerSchema(db *DB) error {
	var n int
	e := db.QueryRow(`SELECT count(*) FROM pg_class WHERE relname IN ('guest_voucher_batch_resellers','guest_voucher_batch_reseller_events') AND relnamespace='public'::regnamespace AND relrowsecurity AND relforcerowsecurity`).Scan(&n)
	if e != nil || n != 2 {
		return errors.New("reseller batch schema unavailable")
	}
	return nil
}
