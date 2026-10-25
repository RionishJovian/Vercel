package database

import (
	"database/sql"
	"fmt"
	"net"
	"time"

	"mega-magic/internal/model"
)

// ---------------------------------------------------------------------------
// BusinessTelegramBotRepository — each business's OWN guest-wifi bot binding.
// ---------------------------------------------------------------------------
type BusinessTelegramBotRepository struct {
	db *DB
}

func NewBusinessTelegramBotRepository(db *DB) *BusinessTelegramBotRepository {
	return &BusinessTelegramBotRepository{db: db}
}

const botCols = `business_id, bot_token, target_chat_id, approver_user_id, enabled, created_at, updated_at`

func scanBot(row interface{ Scan(...interface{}) error }) (*model.BusinessTelegramBot, error) {
	b := &model.BusinessTelegramBot{}
	err := row.Scan(&b.BusinessID, &b.BotToken, &b.TargetChatID, &b.ApproverUserID, &b.Enabled, &b.CreatedAt, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan telegram bot: %w", err)
	}
	return b, nil
}

// GetByBusiness returns a business's own bot binding (nil if none configured).
// Tenant-scoped: the caller supplies the business (from the owner JWT / tenant).
func (r *BusinessTelegramBotRepository) GetByBusiness(businessID string) (*model.BusinessTelegramBot, error) {
	return scanBot(r.db.QueryRow(`SELECT `+botCols+` FROM business_telegram_bots WHERE business_id = $1`, businessID))
}

func (r *BusinessTelegramBotRepository) GetByBusinessTXN(tx *sql.Tx, businessID string) (*model.BusinessTelegramBot, error) {
	return scanBot(tx.QueryRow(`SELECT `+botCols+` FROM business_telegram_bots WHERE business_id = $1`, businessID))
}

// Upsert binds/writes a business's own bot. IF NOT EXISTS semantics via upsert.
func (r *BusinessTelegramBotRepository) Upsert(b *model.BusinessTelegramBot) error {
	return upsertBot(r.db, b)
}

func (r *BusinessTelegramBotRepository) UpsertTXN(tx *sql.Tx, b *model.BusinessTelegramBot) error {
	return upsertBot(tx, b)
}

func upsertBot(exec interface {
	Exec(string, ...any) (sql.Result, error)
}, b *model.BusinessTelegramBot) error {
	_, err := exec.Exec(
		`INSERT INTO business_telegram_bots (business_id, bot_token, target_chat_id, approver_user_id, enabled, updated_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 ON CONFLICT (business_id) DO UPDATE SET
			bot_token = EXCLUDED.bot_token,
			target_chat_id = EXCLUDED.target_chat_id,
			approver_user_id = EXCLUDED.approver_user_id,
			enabled = EXCLUDED.enabled,
			updated_at = now()`,
		b.BusinessID, b.BotToken, b.TargetChatID, b.ApproverUserID, b.Enabled)
	return err
}

// ListEnabled returns every business that has an enabled bot — used by the
// multi-tenant bridge supervisor to know which owner loops to spawn.
func (r *BusinessTelegramBotRepository) ListEnabled() ([]*model.BusinessTelegramBot, error) {
	rows, err := r.db.Query(`SELECT ` + botCols + ` FROM business_telegram_bots WHERE enabled = true`)
	if err != nil {
		return nil, fmt.Errorf("list enabled bots: %w", err)
	}
	defer rows.Close()
	var out []*model.BusinessTelegramBot
	for rows.Next() {
		b, err := scanBot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// GuestPurchaseRequestRepository — owner-approved online-banking wifi submits.
// ---------------------------------------------------------------------------
type GuestPurchaseRequestRepository struct {
	db *DB
}

func NewGuestPurchaseRequestRepository(db *DB) *GuestPurchaseRequestRepository {
	return &GuestPurchaseRequestRepository{db: db}
}

const guestRequestCols = `id, business_id, device_id, client_mac, client_ip, plan_key, amount,
	data_mb, paid_phone, tx_last6, bank_reference, receipt_path, grace_started_at, grace_expires_at, grace_data_mb, status,
	decided_at, created_at, updated_at, purchased_minutes`

// scanGuestRequest is a shared row-scanner for guest purchase requests.
type guestRequestScan interface {
	Scan(dest ...any) error
}

func scanGuestRequest(row guestRequestScan) (*model.GuestPurchaseRequest, error) {
	r := &model.GuestPurchaseRequest{}
	var deviceID, planKey, paidPhone, txLast6, bankRef, receipt, status, clientIP sql.NullString
	var graceStart, graceExpire, decided sql.NullTime
	var amount, dataMB, graceDataMB int
	var purchasedMinutes sql.NullInt64
	if err := row.Scan(&r.ID, &r.BusinessID, &deviceID, &r.ClientMAC, &clientIP, &planKey,
		&amount, &dataMB, &paidPhone, &txLast6, &bankRef, &receipt, &graceStart, &graceExpire, &graceDataMB, &status, &decided,
		&r.CreatedAt, &r.UpdatedAt, &purchasedMinutes); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan guest purchase request: %w", err)
	}
	if purchasedMinutes.Valid {
		v := int(purchasedMinutes.Int64)
		r.PurchasedMinutes = &v
	}
	r.DeviceID = scanNullableString(deviceID)
	r.PlanKey = planKey.String
	r.Amount = amount
	r.DataMB = dataMB
	r.PaidPhone = paidPhone.String
	r.TxLast6 = txLast6.String
	r.BankReference = scanNullableString(bankRef)
	r.ReceiptPath = scanNullableString(receipt)
	r.ClientIP = scanIP(clientIP)
	r.GraceStartedAt = scanNullableTime(graceStart)
	r.GraceExpiresAt = scanNullableTime(graceExpire)
	r.GraceDataMB = graceDataMB
	r.Status = status.String
	r.DecidedAt = scanNullableTime(decided)
	return r, nil
}

func (r *GuestPurchaseRequestRepository) CreateTXN(tx *sql.Tx, req *model.GuestPurchaseRequest) (*model.GuestPurchaseRequest, error) {
	if req.GraceDataMB == 0 {
		req.GraceDataMB = 50
	}
	row := tx.QueryRow(
		`INSERT INTO guest_purchase_requests
		   (business_id, device_id, client_mac, client_ip, plan_key, amount,
			data_mb, bank_reference, receipt_path, grace_started_at, grace_expires_at, grace_data_mb, status, purchased_minutes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 RETURNING `+guestRequestCols,
		req.BusinessID, req.DeviceID, req.ClientMAC, ipString(req.ClientIP), req.PlanKey,
		req.Amount, req.DataMB, req.BankReference, req.ReceiptPath, req.GraceStartedAt, req.GraceExpiresAt,
		req.GraceDataMB, req.Status, req.PurchasedMinutes)
	return scanGuestRequest(row)
}

// GetByIDOwnerTXN looks up a request by id without a business filter — used by
// the OWNER's bridge to resolve the owning business before deciding.
func (r *GuestPurchaseRequestRepository) GetByIDOwnerTXN(tx *sql.Tx, id string) (*model.GuestPurchaseRequest, error) {
	return scanGuestRequest(tx.QueryRow(`SELECT `+guestRequestCols+` FROM guest_purchase_requests WHERE id = $1`, id))
}

func (r *GuestPurchaseRequestRepository) GetByIDTXN(tx *sql.Tx, businessID, id string) (*model.GuestPurchaseRequest, error) {
	return scanGuestRequest(tx.QueryRow(`SELECT `+guestRequestCols+` FROM guest_purchase_requests WHERE id = $1 AND business_id = $2`, id, businessID))
}

func (r *GuestPurchaseRequestRepository) GetForDecisionTXN(tx *sql.Tx, businessID, id string) (*model.GuestPurchaseRequest, error) {
	return scanGuestRequest(tx.QueryRow(`SELECT `+guestRequestCols+` FROM guest_purchase_requests WHERE id=$1 AND business_id=$2 FOR UPDATE`, id, businessID))
}

// LatestGraceTXN prevents a guest resetting the bank-transfer allowance through
// repeated purchase requests. Tenant and router are both explicit anchors.
func (r *GuestPurchaseRequestRepository) LatestGraceTXN(tx *sql.Tx, businessID, deviceID, mac string) (*model.GuestPurchaseRequest, error) {
	return scanGuestRequest(tx.QueryRow(`SELECT `+guestRequestCols+` FROM guest_purchase_requests
	 WHERE business_id=$1 AND device_id=$2 AND upper(client_mac)=$3
	 ORDER BY created_at DESC, id DESC LIMIT 1`, businessID, deviceID, mac))
}

func (r *GuestPurchaseRequestRepository) ListByBusinessTXN(tx *sql.Tx, businessID string, limit int) ([]*model.GuestPurchaseRequest, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := tx.Query(`SELECT `+guestRequestCols+` FROM guest_purchase_requests WHERE business_id = $1 ORDER BY created_at DESC LIMIT $2`, businessID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.GuestPurchaseRequest
	for rows.Next() {
		r, err := scanGuestRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ApproveRequestTXN flips pending -> approved exactly once (single-wind).
func (r *GuestPurchaseRequestRepository) ApproveRequestTXN(tx *sql.Tx, businessID, id string, now time.Time) (*model.GuestPurchaseRequest, error) {
	return scanGuestRequest(tx.QueryRow(
		`UPDATE guest_purchase_requests SET status='approved', decided_at=$3, updated_at=now()
		 WHERE id=$1 AND business_id=$2 AND status='pending'
		 RETURNING `+guestRequestCols, id, businessID, now))
}

func (r *GuestPurchaseRequestRepository) RejectRequestTXN(tx *sql.Tx, businessID, id string, now time.Time) (*model.GuestPurchaseRequest, error) {
	return scanGuestRequest(tx.QueryRow(
		`UPDATE guest_purchase_requests SET status='rejected', decided_at=$3, updated_at=now()
		 WHERE id=$1 AND business_id=$2 AND status='pending'
		 RETURNING `+guestRequestCols, id, businessID, now))
}

func (r *GuestPurchaseRequestRepository) SetReceiptPathTXN(tx *sql.Tx, businessID, id, path string) error {
	_, err := tx.Exec(`UPDATE guest_purchase_requests SET receipt_path=$3, updated_at=now() WHERE id=$1 AND business_id=$2`, id, businessID, path)
	return err
}

// SubmitPaymentDetailsTXN atomically checks that the request is still awaiting
// payment inside its deadline. A decision or expiry racing an upload wins.
func (r *GuestPurchaseRequestRepository) SubmitPaymentDetailsTXN(tx *sql.Tx, businessID, id, phone, last6, path string) (bool, error) {
	result, err := tx.Exec(`UPDATE guest_purchase_requests
	 SET paid_phone=$3, tx_last6=$4, receipt_path=COALESCE(NULLIF($5,''),receipt_path), updated_at=now()
	 WHERE id=$1 AND business_id=$2 AND status='pending' AND grace_expires_at > clock_timestamp()`, id, businessID, phone, last6, path)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// SetMMQRDetailsTXN records the payer's phone + last-6 transaction digits after
// they complete the MMQR bank transfer, so the OWNER can verify before deciding.
func (r *GuestPurchaseRequestRepository) SetMMQRDetailsTXN(tx *sql.Tx, businessID, id, phone, last6 string) error {
	_, err := tx.Exec(`UPDATE guest_purchase_requests SET paid_phone=$3, tx_last6=$4, updated_at=now() WHERE id=$1 AND business_id=$2`, id, businessID, phone, last6)
	return err
}

// helper: encode a *net.IP into a scan-friendly value.
func ipString(ip *net.IP) interface{} {
	if ip == nil {
		return nil
	}
	return ip.String()
}
