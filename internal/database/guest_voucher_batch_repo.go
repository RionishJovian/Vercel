package database

import (
	"database/sql"
	"errors"
	"time"

	"mega-magic/internal/model"
)

// Installation state is independent of the guest's unused/active allowance.
// An uncertain router write must never remove its durable voucher identity.
type GuestVoucherIssueBatch struct {
	ID, BusinessID, DeviceID, RequestHash, Label string
	PriceMMK, Minutes, DataMB, TotalCount        int
	LeaseToken                                   string
	LeaseUntil                                   *time.Time
	CreatedAt                                    time.Time
}

type GuestVoucherIssueItem struct {
	Voucher      *model.Voucher
	InstallState string
}

type GuestVoucherIssueSummary struct {
	BatchID       string    `json:"batch_id"`
	DeviceID      string    `json:"device_id"`
	State         string    `json:"state"`
	TotalCount    int       `json:"total_count"`
	VerifiedCount int       `json:"verified_count"`
	CreatedAt     time.Time `json:"created_at"`
}

type GuestVoucherBatchRepository struct{}

const issueBatchColumns = `id,business_id,device_id,request_hash,plan_label,price_mmk,minutes,data_mb,total_count,lease_token,lease_until,created_at`

func (GuestVoucherBatchRepository) CreateTXN(tx *sql.Tx, b GuestVoucherIssueBatch) (bool, error) {
	r, err := tx.Exec(`INSERT INTO guest_voucher_issue_batches(business_id,id,device_id,request_hash,plan_label,price_mmk,minutes,data_mb,total_count)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(business_id,id) DO NOTHING`,
		b.BusinessID, b.ID, b.DeviceID, b.RequestHash, b.Label, b.PriceMMK, b.Minutes, b.DataMB, b.TotalCount)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}

func (GuestVoucherBatchRepository) GetTXN(tx *sql.Tx, businessID, batchID string, lock bool) (*GuestVoucherIssueBatch, error) {
	q := `SELECT ` + issueBatchColumns + ` FROM guest_voucher_issue_batches WHERE business_id=$1 AND id=$2`
	if lock {
		q += ` FOR UPDATE`
	}
	b := &GuestVoucherIssueBatch{}
	var token sql.NullString
	var until sql.NullTime
	err := tx.QueryRow(q, businessID, batchID).Scan(&b.ID, &b.BusinessID, &b.DeviceID, &b.RequestHash,
		&b.Label, &b.PriceMMK, &b.Minutes, &b.DataMB, &b.TotalCount, &token, &until, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.LeaseToken, b.LeaseUntil = token.String, scanNullableTime(until)
	return b, nil
}

func (GuestVoucherBatchRepository) AddItemTXN(tx *sql.Tx, businessID, batchID, voucherID string, position int) error {
	_, err := tx.Exec(`INSERT INTO guest_voucher_issue_items(business_id,batch_id,voucher_id,position) VALUES($1,$2,$3,$4)`,
		businessID, batchID, voucherID, position)
	return err
}

func (GuestVoucherBatchRepository) ItemsTXN(tx *sql.Tx, businessID, batchID string) ([]GuestVoucherIssueItem, error) {
	rows, err := tx.Query(`SELECT voucher_id,install_state FROM guest_voucher_issue_items WHERE business_id=$1 AND batch_id=$2 ORDER BY position`, businessID, batchID)
	if err != nil {
		return nil, err
	}
	var ids, states []string
	for rows.Next() {
		var id, state string
		if err := rows.Scan(&id, &state); err != nil {
			rows.Close()
			return nil, err
		}
		ids, states = append(ids, id), append(states, state)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	items := make([]GuestVoucherIssueItem, 0, len(ids))
	for i, id := range ids {
		v, err := (&VoucherRepository{}).GetVoucherByIDTXN(tx, id)
		if err != nil {
			return nil, err
		}
		if v == nil || v.BusinessID != businessID || v.PlanKey != "guest-internet" {
			return nil, ErrGuestVoucherUnavailable
		}
		v.IssueBatchID, v.InstallState = batchID, states[i]
		items = append(items, GuestVoucherIssueItem{Voucher: v, InstallState: states[i]})
	}
	return items, nil
}

// ClaimTXN fences concurrent HTTP retries. Router retries are idempotent even if
// an old disconnected worker finishes remotely after its lease has elapsed.
func (GuestVoucherBatchRepository) ClaimTXN(tx *sql.Tx, businessID, batchID, token string) (bool, error) {
	r, err := tx.Exec(`UPDATE guest_voucher_issue_batches SET lease_token=$3,lease_until=clock_timestamp()+interval '90 seconds',updated_at=clock_timestamp()
 WHERE business_id=$1 AND id=$2 AND (lease_until IS NULL OR lease_until<=clock_timestamp())
 AND EXISTS(SELECT 1 FROM guest_voucher_issue_items WHERE business_id=$1 AND batch_id=$2 AND install_state<>'verified')`, businessID, batchID, token)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n == 1, err
}

// FinishTXN records verification only after the entire batch was read back.
// On any uncertainty every unconfirmed item stays durable and unprintable.
func (r GuestVoucherBatchRepository) FinishTXN(tx *sql.Tx, businessID, batchID, token string, verified bool) error {
	b, err := r.GetTXN(tx, businessID, batchID, true)
	if err != nil {
		return err
	}
	if b == nil || b.LeaseToken != token {
		return ErrGuestVoucherBusy
	}
	state := "unknown"
	if verified {
		state = "verified"
	}
	if _, err = tx.Exec(`UPDATE guest_voucher_issue_items SET install_state=$3,
 verified_at=CASE WHEN $3='verified' THEN clock_timestamp() ELSE NULL END,updated_at=clock_timestamp()
 WHERE business_id=$1 AND batch_id=$2 AND install_state<>'verified'`, businessID, batchID, state); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE guest_voucher_issue_batches SET lease_token=NULL,lease_until=NULL,updated_at=clock_timestamp() WHERE business_id=$1 AND id=$2 AND lease_token=$3`, businessID, batchID, token)
	return err
}

func (GuestVoucherBatchRepository) ListIncompleteTXN(tx *sql.Tx, businessID string) ([]GuestVoucherIssueSummary, error) {
	rows, err := tx.Query(`SELECT b.id,b.device_id,b.total_count,b.created_at,
 count(*) FILTER(WHERE i.install_state='verified'),
 CASE WHEN b.lease_until>clock_timestamp() THEN 'installing'
 WHEN count(*) FILTER(WHERE i.install_state='unknown')>0 THEN 'unknown' ELSE 'pending' END
 FROM guest_voucher_issue_batches b JOIN guest_voucher_issue_items i ON i.business_id=b.business_id AND i.batch_id=b.id
 WHERE b.business_id=$1 GROUP BY b.business_id,b.id
 HAVING count(*) FILTER(WHERE i.install_state='verified')<b.total_count
 ORDER BY b.created_at DESC LIMIT 100`, businessID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]GuestVoucherIssueSummary, 0)
	for rows.Next() {
		var b GuestVoucherIssueSummary
		if err := rows.Scan(&b.BatchID, &b.DeviceID, &b.TotalCount, &b.CreatedAt, &b.VerifiedCount, &b.State); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// AnnotateTXN tolerates older schemas while the router-local feature is off.
// If metadata exists but cannot be read, fail closed instead of hiding failures.
func (GuestVoucherBatchRepository) AnnotateTXN(tx *sql.Tx, businessID string, vouchers []*model.Voucher) error {
	var exists bool
	if err := tx.QueryRow(`SELECT to_regclass('public.guest_voucher_issue_items') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		return err
	}
	rows, err := tx.Query(`SELECT voucher_id,batch_id,install_state FROM guest_voucher_issue_items WHERE business_id=$1`, businessID)
	if err != nil {
		return err
	}
	defer rows.Close()
	byID := make(map[string]*model.Voucher, len(vouchers))
	for _, v := range vouchers {
		byID[v.ID] = v
	}
	for rows.Next() {
		var id, batch, state string
		if err := rows.Scan(&id, &batch, &state); err != nil {
			return err
		}
		if v := byID[id]; v != nil {
			v.IssueBatchID, v.InstallState = batch, state
		}
	}
	return rows.Err()
}
