package database

import (
	"database/sql"
	"encoding/json"
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

// strOrNil binds an empty string as SQL NULL.
func strOrNil(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

const portalConfigCols = `business_id, business_name, welcome_text, terms_text, primary_hex, glass_tint_hex, consent_required, consent_text, bank_name, bank_account, bank_holder, mmqr_url, guest_grace_minutes, guest_grace_data_mb, guest_plans, voucher_login_enabled, mobile_payment_enabled, updated_at`

func portalConfigUpsertSQL() string {
	return `INSERT INTO portal_configs (business_id, business_name, welcome_text, terms_text, primary_hex, glass_tint_hex, consent_required, consent_text, bank_name, bank_account, bank_holder, mmqr_url, guest_grace_minutes, guest_grace_data_mb, guest_plans, voucher_login_enabled, mobile_payment_enabled, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (business_id) DO UPDATE SET
		   business_name = EXCLUDED.business_name,
		   welcome_text = EXCLUDED.welcome_text,
		   terms_text = EXCLUDED.terms_text,
		   primary_hex = EXCLUDED.primary_hex,
		   glass_tint_hex = EXCLUDED.glass_tint_hex,
		   consent_required = EXCLUDED.consent_required,
		   consent_text = EXCLUDED.consent_text,
		   bank_name = EXCLUDED.bank_name,
		   bank_account = EXCLUDED.bank_account,
		   bank_holder = EXCLUDED.bank_holder,
		   mmqr_url = EXCLUDED.mmqr_url,
		   guest_grace_minutes = EXCLUDED.guest_grace_minutes,
		   guest_grace_data_mb = EXCLUDED.guest_grace_data_mb,
		   guest_plans = EXCLUDED.guest_plans,
		   voucher_login_enabled = EXCLUDED.voucher_login_enabled,
		   mobile_payment_enabled = EXCLUDED.mobile_payment_enabled,
		   updated_at = EXCLUDED.updated_at`
}

func scanPortalConfig(row interface{ Scan(...interface{}) error }) (*model.PortalConfig, error) {
	p := &model.PortalConfig{}
	var welcome, terms, primary, glass, consentText, bankName, bankAccount, bankHolder, mmqrURL sql.NullString
	var guestPlans []byte
	err := row.Scan(&p.BusinessID, &p.BusinessName, &welcome, &terms, &primary, &glass,
		&p.ConsentRequired, &consentText, &bankName, &bankAccount, &bankHolder, &mmqrURL,
		&p.GuestGraceMinutes, &p.GuestGraceDataMB, &guestPlans, &p.VoucherLoginEnabled, &p.MobilePaymentEnabled, &p.UpdatedAt)
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
	p.BankName = nullStr(bankName)
	p.BankAccount = nullStr(bankAccount)
	p.BankHolder = nullStr(bankHolder)
	p.MMQRURL = nullStr(mmqrURL)
	if len(guestPlans) > 0 {
		// jsonb arrives as []byte; decode into the typed slice (tolerant on no-op).
		_ = json.Unmarshal(guestPlans, &p.GuestPlans)
	}
	return p, nil
}

func (r *PortalConfigRepository) GetByBusiness(businessID string) (*model.PortalConfig, error) {
	return scanPortalConfig(r.db.QueryRow(`SELECT `+portalConfigCols+` FROM portal_configs WHERE business_id = $1`, businessID))
}

func (r *PortalConfigRepository) Upsert(cfg *model.PortalConfig) error {
	if cfg.GuestGraceDataMB == 0 {
		cfg.GuestGraceDataMB = 50
	}
	guestPlans, err := json.Marshal(cfg.GuestPlans)
	if err != nil {
		return err
	}
	voucherEnabled, paymentEnabled := portalAccessMethods(cfg)
	_, err = r.db.Exec(portalConfigUpsertSQL(),
		cfg.BusinessID, cfg.BusinessName, cfg.WelcomeText, cfg.TermsText,
		cfg.PrimaryHex, cfg.GlassTintHex, cfg.ConsentRequired, cfg.ConsentText,
		strOrNil(cfg.BankName), strOrNil(cfg.BankAccount), strOrNil(cfg.BankHolder), strOrNil(cfg.MMQRURL),
		cfg.GuestGraceMinutes, cfg.GuestGraceDataMB, guestPlans, voucherEnabled, paymentEnabled, time.Now().UTC())
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
	return scanPortalConfig(tx.QueryRow(`SELECT `+portalConfigCols+` FROM portal_configs WHERE business_id = $1`, businessID))
}

// UpsertTXN is the transaction-scoped variant of Upsert.
func (r *PortalConfigRepository) UpsertTXN(tx *sql.Tx, cfg *model.PortalConfig) error {
	if cfg.GuestGraceDataMB == 0 {
		cfg.GuestGraceDataMB = 50
	}
	guestPlans, err := json.Marshal(cfg.GuestPlans)
	if err != nil {
		return err
	}
	voucherEnabled, paymentEnabled := portalAccessMethods(cfg)
	_, err = tx.Exec(portalConfigUpsertSQL(),
		cfg.BusinessID, cfg.BusinessName, cfg.WelcomeText, cfg.TermsText,
		cfg.PrimaryHex, cfg.GlassTintHex, cfg.ConsentRequired, cfg.ConsentText,
		strOrNil(cfg.BankName), strOrNil(cfg.BankAccount), strOrNil(cfg.BankHolder), strOrNil(cfg.MMQRURL),
		cfg.GuestGraceMinutes, cfg.GuestGraceDataMB, guestPlans, voucherEnabled, paymentEnabled, time.Now().UTC())
	return err
}

// portalAccessMethods keeps old callers and historical fixtures compatible:
// before these fields existed a zero-value PortalConfig meant "both methods".
// The HTTP handler rejects an owner explicitly disabling both, so this fallback
// cannot bypass the user-facing invariant.
func portalAccessMethods(cfg *model.PortalConfig) (bool, bool) {
	if !cfg.VoucherLoginEnabled && !cfg.MobilePaymentEnabled {
		return true, true
	}
	return cfg.VoucherLoginEnabled, cfg.MobilePaymentEnabled
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
