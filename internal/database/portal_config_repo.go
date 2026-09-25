package database

import (
	"database/sql"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

type PortalConfigRepository struct {
	db *DB
}

func NewPortalConfigRepository(db *DB) *PortalConfigRepository {
	return &PortalConfigRepository{db: db}
}

func (r *PortalConfigRepository) GetByBusiness(businessID string) (*model.PortalConfig, error) {
	p := &model.PortalConfig{}
	var welcome, terms, primary, glass, consentText sql.NullString
	err := r.db.QueryRow(
		`SELECT business_id, business_name, welcome_text, terms_text, primary_hex, glass_tint_hex, consent_required, consent_text, updated_at
		 FROM portal_configs WHERE business_id = $1`, businessID,
	).Scan(&p.BusinessID, &p.BusinessName, &welcome, &terms, &primary, &glass, &p.ConsentRequired, &consentText, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get portal config: %w", err)
	}
	p.WelcomeText = nullStr(welcome)
	p.TermsText = nullStr(terms)
	p.PrimaryHex = nullStr(primary)
	p.GlassTintHex = nullStr(glass)
	p.ConsentText = nullStr(consentText)
	return p, nil
}

func (r *PortalConfigRepository) Upsert(cfg *model.PortalConfig) error {
	_, err := r.db.Exec(
		`INSERT INTO portal_configs (business_id, business_name, welcome_text, terms_text, primary_hex, glass_tint_hex, consent_required, consent_text, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (business_id) DO UPDATE SET
		   business_name = EXCLUDED.business_name,
		   welcome_text = EXCLUDED.welcome_text,
		   terms_text = EXCLUDED.terms_text,
		   primary_hex = EXCLUDED.primary_hex,
		   glass_tint_hex = EXCLUDED.glass_tint_hex,
		   consent_required = EXCLUDED.consent_required,
		   consent_text = EXCLUDED.consent_text,
		   updated_at = EXCLUDED.updated_at`,
		cfg.BusinessID, cfg.BusinessName, cfg.WelcomeText, cfg.TermsText,
		cfg.PrimaryHex, cfg.GlassTintHex, cfg.ConsentRequired, cfg.ConsentText, time.Now().UTC())
	return err
}

func (r *PortalConfigRepository) Delete(businessID string) error {
	result, err := r.db.Exec(`DELETE FROM portal_configs WHERE business_id = $1`, businessID)
	if err != nil {
		return fmt.Errorf("delete portal config: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("portal config not found: %s", businessID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Transaction-scoped variants (RLS enforcement rollout: portal config). Same
// SQL on the caller's *sql.Tx so they can run under WithTenant. Bare versions
// stay for the later full rollout.
// ---------------------------------------------------------------------------

// GetByBusinessTXN is the transaction-scoped variant of GetByBusiness.
func (r *PortalConfigRepository) GetByBusinessTXN(tx *sql.Tx, businessID string) (*model.PortalConfig, error) {
	p := &model.PortalConfig{}
	var welcome, terms, primary, glass, consentText sql.NullString
	err := tx.QueryRow(
		`SELECT business_id, business_name, welcome_text, terms_text, primary_hex, glass_tint_hex, consent_required, consent_text, updated_at
		 FROM portal_configs WHERE business_id = $1`, businessID,
	).Scan(&p.BusinessID, &p.BusinessName, &welcome, &terms, &primary, &glass, &p.ConsentRequired, &consentText, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get portal config: %w", err)
	}
	p.WelcomeText = nullStr(welcome)
	p.TermsText = nullStr(terms)
	p.PrimaryHex = nullStr(primary)
	p.GlassTintHex = nullStr(glass)
	p.ConsentText = nullStr(consentText)
	return p, nil
}

// UpsertTXN is the transaction-scoped variant of Upsert.
func (r *PortalConfigRepository) UpsertTXN(tx *sql.Tx, cfg *model.PortalConfig) error {
	_, err := tx.Exec(
		`INSERT INTO portal_configs (business_id, business_name, welcome_text, terms_text, primary_hex, glass_tint_hex, consent_required, consent_text, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (business_id) DO UPDATE SET
		   business_name = EXCLUDED.business_name,
		   welcome_text = EXCLUDED.welcome_text,
		   terms_text = EXCLUDED.terms_text,
		   primary_hex = EXCLUDED.primary_hex,
		   glass_tint_hex = EXCLUDED.glass_tint_hex,
		   consent_required = EXCLUDED.consent_required,
		   consent_text = EXCLUDED.consent_text,
		   updated_at = EXCLUDED.updated_at`,
		cfg.BusinessID, cfg.BusinessName, cfg.WelcomeText, cfg.TermsText,
		cfg.PrimaryHex, cfg.GlassTintHex, cfg.ConsentRequired, cfg.ConsentText, time.Now().UTC())
	return err
}

// DeleteTXN is the transaction-scoped variant of Delete.
func (r *PortalConfigRepository) DeleteTXN(tx *sql.Tx, businessID string) error {
	result, err := tx.Exec(`DELETE FROM portal_configs WHERE business_id = $1`, businessID)
	if err != nil {
		return fmt.Errorf("delete portal config: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("portal config not found: %s", businessID)
	}
	return nil
}
