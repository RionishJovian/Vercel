package database

import (
	"database/sql"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

type BusinessMeteredQuotaRepository struct {
	db *DB
}

func NewBusinessMeteredQuotaRepository(db *DB) *BusinessMeteredQuotaRepository {
	return &BusinessMeteredQuotaRepository{db: db}
}

func (r *BusinessMeteredQuotaRepository) GetByBusiness(businessID string) (*model.BusinessMeteredQuota, error) {
	q := &model.BusinessMeteredQuota{}
	var passExpiry sql.NullTime
	err := r.db.QueryRow(
		`SELECT business_id, purchased_extra_slots, webfig_pass_expires_at, ai_token_balance, updated_at FROM business_metered_quotas WHERE business_id = $1`,
		businessID,
	).Scan(&q.BusinessID, &q.PurchasedExtraSlots, &passExpiry, &q.AITokenBalance, &q.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get quota: %w", err)
	}
	if passExpiry.Valid {
		q.WebfigPassExpiresAt = &passExpiry.Time
	}
	return q, nil
}

func (r *BusinessMeteredQuotaRepository) Upsert(quota *model.BusinessMeteredQuota) error {
	passExpiry := quotableTime(quota.WebfigPassExpiresAt)
	_, err := r.db.Exec(
		`INSERT INTO business_metered_quotas (business_id, purchased_extra_slots, webfig_pass_expires_at, ai_token_balance, updated_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (business_id) DO UPDATE SET
		   purchased_extra_slots = EXCLUDED.purchased_extra_slots,
		   webfig_pass_expires_at = EXCLUDED.webfig_pass_expires_at,
		   ai_token_balance = EXCLUDED.ai_token_balance,
		   updated_at = EXCLUDED.updated_at`,
		quota.BusinessID, quota.PurchasedExtraSlots, passExpiry,
		quota.AITokenBalance, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("upsert quota: %w", err)
	}
	return nil
}

func (r *BusinessMeteredQuotaRepository) UpdateAITokenBalance(businessID string, balance int) error {
	_, err := r.db.Exec(
		`UPDATE business_metered_quotas SET ai_token_balance = $1, updated_at = $2 WHERE business_id = $3`,
		balance, time.Now().UTC(), businessID)
	return err
}

func (r *BusinessMeteredQuotaRepository) ExtendWebfigPass(businessID string, expiry time.Time) error {
	_, err := r.db.Exec(
		`UPDATE business_metered_quotas SET webfig_pass_expires_at = $1, updated_at = $2 WHERE business_id = $3`,
		expiry, time.Now().UTC(), businessID)
	return err
}

func quotableTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}