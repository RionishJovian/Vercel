package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"mega-magic/internal/model"
	"mega-magic/internal/store"
)

var ErrStorePurchaseNotFound = errors.New("store purchase not found")

type StorePurchaseRepository struct{ db *DB }

func NewStorePurchaseRepository(db *DB) *StorePurchaseRepository {
	return &StorePurchaseRepository{db: db}
}

func (r *StorePurchaseRepository) ListEnabledBankAccounts() ([]model.StoreBankAccount, error) {
	rows, err := r.db.Query(`SELECT id,holder_name,bank_name,account_number FROM platform_service_bank_accounts WHERE enabled=true ORDER BY sort,slot`)
	if err != nil {
		return nil, fmt.Errorf("list store bank accounts: %w", err)
	}
	defer rows.Close()
	out := []model.StoreBankAccount{}
	for rows.Next() {
		var a model.StoreBankAccount
		if err := rows.Scan(&a.ID, &a.HolderName, &a.BankName, &a.AccountNumber); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

const storePurchaseColumns = `id, business_id, user_id, service_key, service_label,
 price_mmk, magic_coin_purchase_amount, payment_method, bank_account_id,
 bank_reference, status, idempotency_key, decided_by, decided_at, reject_reason,
 activated_expires_at, created_at, updated_at`

func scanStorePurchase(s interface{ Scan(...any) error }) (*model.StorePurchase, error) {
	p := &model.StorePurchase{}
	var coins sql.NullFloat64
	var bankID, bankRef, decidedBy, rejectReason sql.NullString
	var decidedAt, expiresAt sql.NullTime
	err := s.Scan(&p.ID, &p.BusinessID, &p.UserID, &p.ServiceKey, &p.ServiceLabel,
		&p.PriceMMK, &coins, &p.PaymentMethod, &bankID, &bankRef, &p.Status,
		&p.IdempotencyKey, &decidedBy, &decidedAt, &rejectReason, &expiresAt,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if coins.Valid {
		p.MagicCoinPurchaseAmount = &coins.Float64
	}
	if bankID.Valid {
		p.BankAccountID = &bankID.String
	}
	if bankRef.Valid {
		p.BankReference = &bankRef.String
	}
	if decidedBy.Valid {
		p.DecidedBy = &decidedBy.String
	}
	if decidedAt.Valid {
		p.DecidedAt = &decidedAt.Time
	}
	if rejectReason.Valid {
		p.RejectReason = &rejectReason.String
	}
	if expiresAt.Valid {
		p.ActivatedExpiresAt = &expiresAt.Time
	}
	return p, nil
}

func (r *StorePurchaseRepository) CreateTXN(tx *sql.Tx, businessID, userID string, product store.Product, bankAccountID, bankReference, idempotencyKey string) (*model.StorePurchase, error) {
	var coins any
	if product.CoinCount > 0 {
		coins = product.CoinCount
	}
	row := tx.QueryRow(`INSERT INTO service_purchases
		(business_id,user_id,service_key,service_label,price_mmk,magic_coin_purchase_amount,
		 payment_method,bank_account_id,bank_reference,idempotency_key)
		SELECT $1,$2,$3,$4,$5,$6,'bank_transfer',a.id,$8,$9
		  FROM platform_service_bank_accounts a WHERE a.id=$7 AND a.enabled=true
		RETURNING `+storePurchaseColumns,
		businessID, userID, product.Key, product.Label, product.PriceMMK, coins,
		bankAccountID, bankReference, idempotencyKey)
	p, err := scanStorePurchase(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("bank account is unavailable")
	}
	if err != nil {
		return nil, fmt.Errorf("create store purchase: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO service_purchase_audit
		(business_id,purchase_id,actor_user_id,action,note) VALUES ($1,$2,$3,'submitted','bank transfer submitted')`,
		businessID, p.ID, userID); err != nil {
		return nil, fmt.Errorf("audit store purchase: %w", err)
	}
	return p, nil
}

func (r *StorePurchaseRepository) ListTXN(tx *sql.Tx, businessID string) ([]*model.StorePurchase, error) {
	rows, err := tx.Query(`SELECT `+storePurchaseColumns+` FROM service_purchases WHERE business_id=$1 ORDER BY created_at DESC LIMIT 200`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list store purchases: %w", err)
	}
	defer rows.Close()
	out := []*model.StorePurchase{}
	for rows.Next() {
		p, err := scanStorePurchase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *StorePurchaseRepository) ResolveBusiness(requestID string) (string, error) {
	var businessID string
	err := r.db.QueryRow(`SELECT business_id FROM service_purchases WHERE id=$1`, requestID).Scan(&businessID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrStorePurchaseNotFound
	}
	return businessID, err
}

// DecideTXN locks and decides exactly once, then activates the purchased product
// before returning. Any error rolls back both the decision and activation.
func (r *StorePurchaseRepository) DecideTXN(tx *sql.Tx, businessID, requestID, actorUserID string, approve bool, rejectReason string, now time.Time) (*model.StorePurchase, error) {
	return r.decideTXN(tx, businessID, requestID, actorUserID, approve, rejectReason, now, false)
}

// DecideSubscriptionTXN serializes approvals across every purchaser in a tenant.
// The durable purchase ledger, not a user's legacy entitlement, owns the deadline.
func (r *StorePurchaseRepository) DecideSubscriptionTXN(tx *sql.Tx, businessID, requestID, actorUserID string, approve bool, rejectReason string, now time.Time) (*model.StorePurchase, error) {
	// Acquire before purchase/account locks; never hold this transaction over I/O.
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended('subscription:' || $1::text, 0))`, businessID); err != nil {
		return nil, fmt.Errorf("lock business subscription: %w", err)
	}
	return r.decideTXN(tx, businessID, requestID, actorUserID, approve, rejectReason, now, true)
}

func (r *StorePurchaseRepository) decideTXN(tx *sql.Tx, businessID, requestID, actorUserID string, approve bool, rejectReason string, now time.Time, subscriptionOnly bool) (*model.StorePurchase, error) {
	p, err := scanStorePurchase(tx.QueryRow(`SELECT `+storePurchaseColumns+` FROM service_purchases WHERE business_id=$1 AND id=$2 FOR UPDATE`, businessID, requestID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrStorePurchaseNotFound
	}
	if err != nil {
		return nil, err
	}
	if p.Status != "pending" {
		return nil, errors.New("store purchase already decided")
	}
	if subscriptionOnly && p.ServiceKey != store.ProductMonthly && p.ServiceKey != store.ProductAnnual {
		return nil, errors.New("only monthly or annual subscriptions are available")
	}
	status, action := "rejected", "rejected"
	var expires any
	if approve {
		status, action = "approved", "approved_and_activated"
		switch p.ServiceKey {
		case store.ProductMonthly, store.ProductAnnual:
			days := 30
			if p.ServiceKey == store.ProductAnnual {
				days = 365
			}
			var expiry time.Time
			if subscriptionOnly {
				err = tx.QueryRow(`SELECT GREATEST(clock_timestamp(),
					COALESCE((SELECT trial_expires_at FROM business_subscription_trials WHERE business_id=$1), '-infinity'::timestamptz),
					COALESCE((SELECT max(activated_expires_at) FROM service_purchases
					 WHERE business_id=$1 AND status='approved' AND service_key IN ('monthly','annual')), '-infinity'::timestamptz))
					+ ($2 * interval '24 hours')`, businessID, days).Scan(&expiry)
				if err != nil {
					return nil, fmt.Errorf("calculate business renewal: %w", err)
				}
				var result sql.Result
				result, err = tx.Exec(`INSERT INTO account_entitlements
					(user_id,business_id,tier,tier_expires_at,plus,plus_since,created_at,updated_at)
					SELECT $1,$2,$3,$4,true,now(),now(),now()
					WHERE EXISTS (SELECT 1 FROM users WHERE user_id=$1 AND business_id=$2)
					ON CONFLICT (user_id) DO UPDATE SET tier=EXCLUDED.tier,tier_expires_at=EXCLUDED.tier_expires_at,
					plus=true,plus_since=COALESCE(account_entitlements.plus_since,now()),updated_at=now()
					WHERE account_entitlements.business_id=EXCLUDED.business_id`, p.UserID, businessID, p.ServiceKey, expiry)
				if err == nil {
					var affected int64
					affected, err = result.RowsAffected()
					if err == nil && affected != 1 {
						err = errors.New("entitlement tenant mismatch")
					}
				}
			} else {
				err = tx.QueryRow(`INSERT INTO account_entitlements
				(user_id,business_id,tier,tier_expires_at,plus,plus_since,created_at,updated_at)
				VALUES ($1,$2,$4,now()+($3||' days')::interval,true,now(),now(),now())
				ON CONFLICT (user_id) DO UPDATE SET business_id=EXCLUDED.business_id,tier=EXCLUDED.tier,plus=true,
				tier_expires_at=GREATEST(COALESCE(account_entitlements.tier_expires_at,now()),now())+($3||' days')::interval,
				plus_since=COALESCE(account_entitlements.plus_since,now()),updated_at=now()
				RETURNING tier_expires_at`, p.UserID, businessID, days, p.ServiceKey).Scan(&expiry)
			}
			if err != nil {
				return nil, fmt.Errorf("activate entitlement: %w", err)
			}
			expires = expiry
		case store.ProductMagicCoins:
			if p.MagicCoinPurchaseAmount == nil || *p.MagicCoinPurchaseAmount <= 0 {
				return nil, errors.New("invalid coin purchase")
			}
			if _, err = tx.Exec(`INSERT INTO magic_coin_wallets(user_id,business_id) VALUES($1,$2) ON CONFLICT(user_id) DO NOTHING`, p.UserID, businessID); err != nil {
				return nil, err
			}
			var balance float64
			if err = tx.QueryRow(`SELECT balance FROM magic_coin_wallets WHERE user_id=$1 AND business_id=$2 FOR UPDATE`, p.UserID, businessID).Scan(&balance); err != nil {
				return nil, err
			}
			balance += *p.MagicCoinPurchaseAmount
			if _, err = tx.Exec(`INSERT INTO magic_coin_transactions
				(business_id,user_id,delta,balance_after,kind,service_purchase_id,note,actor_user_id)
				VALUES($1,$2,$3,$4,'purchase_credit',$5,'Store bank payment approved',$6)`,
				businessID, p.UserID, *p.MagicCoinPurchaseAmount, balance, p.ID, nullIfBlank(actorUserID)); err != nil {
				return nil, fmt.Errorf("credit coin ledger: %w", err)
			}
			if _, err = tx.Exec(`UPDATE magic_coin_wallets SET balance=$1,updated_at=now() WHERE user_id=$2 AND business_id=$3`, balance, p.UserID, businessID); err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("unknown store product")
		}
	}
	row := tx.QueryRow(`UPDATE service_purchases SET status=$3,decided_by=$4,decided_at=$5,
		reject_reason=NULLIF($6,''),activated_expires_at=$7,updated_at=$5
		WHERE business_id=$1 AND id=$2 RETURNING `+storePurchaseColumns,
		businessID, requestID, status, nullIfBlank(actorUserID), now, rejectReason, expires)
	out, err := scanStorePurchase(row)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`INSERT INTO service_purchase_audit(business_id,purchase_id,actor_user_id,action,note) VALUES($1,$2,$3,$4,NULLIF($5,''))`, businessID, requestID, nullIfBlank(actorUserID), action, rejectReason); err != nil {
		return nil, err
	}
	return out, nil
}

func nullIfBlank(s string) any {
	if s == "" {
		return nil
	}
	return s
}
