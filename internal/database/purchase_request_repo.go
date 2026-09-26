package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"mega-magic/internal/model"
)

// PurchaseRequestRepository handles device-anchored, operator-approved billing
// requests. Every method is tenant-scoped (business_id in the WHERE clause) and
// runs on the caller's *sql.Tx so it can run under WithTenant.
type PurchaseRequestRepository struct {
	db *DB
}

func NewPurchaseRequestRepository(db *DB) *PurchaseRequestRepository {
	return &PurchaseRequestRepository{db: db}
}

const purchaseRequestCols = `id, business_id, device_id, plan_key, amount, bank_reference, duration_minutes, receipt_path, status, decided_at, created_at, updated_at`

func scanPurchaseRequest(row interface{ Scan(...interface{}) error }) (*model.PurchaseRequest, error) {
	p := &model.PurchaseRequest{}
	var deviceID, receipt sql.NullString
	var decided sql.NullTime
	err := row.Scan(&p.ID, &p.BusinessID, &deviceID, &p.PlanKey, &p.Amount, &p.BankReference, &p.DurationMinutes, &receipt, &p.Status, &decided, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan purchase request: %w", err)
	}
	if deviceID.Valid {
		p.DeviceID = &deviceID.String
	}
	if receipt.Valid {
		p.ReceiptPath = &receipt.String
	}
	if decided.Valid {
		p.DecidedAt = &decided.Time
	}
	return p, nil
}

// CreateRequestTXN inserts a pending purchase request. bank_reference and
// duration_minutes are required (NOT NULL); the per-business bank-reference
// uniqueness index rejects a duplicate live reference (23505) at INSERT.
func (r *PurchaseRequestRepository) CreateRequestTXN(tx *sql.Tx, req *model.PurchaseRequest) (*model.PurchaseRequest, error) {
	return scanPurchaseRequest(tx.QueryRow(
		`INSERT INTO purchase_requests (business_id, device_id, plan_key, amount, bank_reference, duration_minutes, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+purchaseRequestCols,
		req.BusinessID, req.DeviceID, req.PlanKey, req.Amount,
		strings.ToLower(strings.TrimSpace(req.BankReference)), req.DurationMinutes, req.Status))
}

// FindLiveByBankReferenceTXN returns a non-rejected request with the same
// (business_id, lower(bank_reference)), or nil — the guarded pre-check that lets
// the handler return a clear 409 before attempting the INSERT.
func (r *PurchaseRequestRepository) FindLiveByBankReferenceTXN(tx *sql.Tx, businessID, bankReference string) (*model.PurchaseRequest, error) {
	return scanPurchaseRequest(tx.QueryRow(
		`SELECT `+purchaseRequestCols+` FROM purchase_requests
		 WHERE business_id = $1 AND lower(bank_reference) = lower($2) AND status <> 'rejected'
		 LIMIT 1`, businessID, strings.TrimSpace(bankReference)))
}

// SetReceiptPathTXN binds a stored receipt to its request.
func (r *PurchaseRequestRepository) SetReceiptPathTXN(tx *sql.Tx, businessID, id, path string) error {
	_, err := tx.Exec(
		`UPDATE purchase_requests SET receipt_path = $3, updated_at = now() WHERE id = $1 AND business_id = $2`,
		id, businessID, path)
	if err != nil {
		return fmt.Errorf("set receipt path: %w", err)
	}
	return nil
}

// GetRequestByIDTXN returns a request scoped to the tenant.
func (r *PurchaseRequestRepository) GetRequestByIDTXN(tx *sql.Tx, businessID, id string) (*model.PurchaseRequest, error) {
	return scanPurchaseRequest(tx.QueryRow(
		`SELECT `+purchaseRequestCols+` FROM purchase_requests WHERE id = $1 AND business_id = $2`, id, businessID))
}

// ListRequestsByBusinessTXN lists a tenant's requests.
func (r *PurchaseRequestRepository) ListRequestsByBusinessTXN(tx *sql.Tx, businessID string) ([]*model.PurchaseRequest, error) {
	rows, err := tx.Query(
		`SELECT `+purchaseRequestCols+` FROM purchase_requests WHERE business_id = $1 ORDER BY created_at DESC`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list purchase requests: %w", err)
	}
	defer rows.Close()

	var out []*model.PurchaseRequest
	for rows.Next() {
		p, err := scanPurchaseRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ApproveRequestTXN atomically transitions pending -> approved (single wind).
// Returns nil (no error) when nothing transitioned — already decided, or not
// this tenant — so a double-approve can never grant twice.
func (r *PurchaseRequestRepository) ApproveRequestTXN(tx *sql.Tx, businessID, id string, now time.Time) (*model.PurchaseRequest, error) {
	return scanPurchaseRequest(tx.QueryRow(
		`UPDATE purchase_requests SET status = 'approved', decided_at = $3, updated_at = $3
		 WHERE id = $1 AND business_id = $2 AND status = 'pending'
		 RETURNING `+purchaseRequestCols, id, businessID, now))
}

// RejectRequestTXN atomically transitions pending -> rejected (single wind).
// Returns nil when nothing transitioned.
func (r *PurchaseRequestRepository) RejectRequestTXN(tx *sql.Tx, businessID, id string, now time.Time) (*model.PurchaseRequest, error) {
	return scanPurchaseRequest(tx.QueryRow(
		`UPDATE purchase_requests SET status = 'rejected', decided_at = $3, updated_at = $3
		 WHERE id = $1 AND business_id = $2 AND status = 'pending'
		 RETURNING `+purchaseRequestCols, id, businessID, now))
}
