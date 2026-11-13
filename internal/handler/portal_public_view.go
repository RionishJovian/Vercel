package handler

import "mega-magic/internal/model"

// publicPortalConfig is the only portal configuration an anonymous guest receives.
//
// It is an explicit allow-list copy of model.PortalConfig, not the model itself, so a field added to the model later
// is NOT published to the internet by default. The owner-facing endpoints keep returning the full model.
//
// Left out on purpose:
//   - business_id: the tenant identifier. A guest never needs it; every guest request is anchored by the device id.
//   - updated_at: internal bookkeeping.
//
// media_host and media_port stay: the guest page builds the owner's media link from them (web/assets/js/portal.js),
// and the owner chose to publish that LAN address to the portal.
type publicPortalConfig struct {
	BusinessName         string            `json:"business_name"`
	WelcomeText          string            `json:"welcome_text"`
	TermsText            string            `json:"terms_text"`
	PrimaryHex           string            `json:"primary_hex"`
	GlassTintHex         string            `json:"glass_tint_hex"`
	ConsentRequired      bool              `json:"consent_required"`
	ConsentText          string            `json:"consent_text"`
	BankName             string            `json:"bank_name,omitempty"`
	BankAccount          string            `json:"bank_account,omitempty"`
	BankHolder           string            `json:"bank_holder,omitempty"`
	MMQRURL              string            `json:"mmqr_url,omitempty"`
	PaymentInstructions  string            `json:"payment_instructions,omitempty"`
	MediaHost            string            `json:"media_host,omitempty"`
	MediaPort            string            `json:"media_port,omitempty"`
	GuestGraceMinutes    int               `json:"guest_grace_minutes"`
	GuestGraceDataMB     int               `json:"guest_grace_data_mb"`
	GuestPlans           []model.GuestPlan `json:"guest_plans"`
	VoucherLoginEnabled  bool              `json:"voucher_login_enabled"`
	MobilePaymentEnabled bool              `json:"mobile_payment_enabled"`
}

// publicPortalView copies the guest-visible fields. It returns nil for a nil config.
func publicPortalView(cfg *model.PortalConfig) *publicPortalConfig {
	if cfg == nil {
		return nil
	}
	return &publicPortalConfig{
		BusinessName:         cfg.BusinessName,
		WelcomeText:          cfg.WelcomeText,
		TermsText:            cfg.TermsText,
		PrimaryHex:           cfg.PrimaryHex,
		GlassTintHex:         cfg.GlassTintHex,
		ConsentRequired:      cfg.ConsentRequired,
		ConsentText:          cfg.ConsentText,
		BankName:             cfg.BankName,
		BankAccount:          cfg.BankAccount,
		BankHolder:           cfg.BankHolder,
		MMQRURL:              cfg.MMQRURL,
		PaymentInstructions:  cfg.PaymentInstructions,
		MediaHost:            cfg.MediaHost,
		MediaPort:            cfg.MediaPort,
		GuestGraceMinutes:    cfg.GuestGraceMinutes,
		GuestGraceDataMB:     cfg.GuestGraceDataMB,
		GuestPlans:           cfg.GuestPlans,
		VoucherLoginEnabled:  cfg.VoucherLoginEnabled,
		MobilePaymentEnabled: cfg.MobilePaymentEnabled,
	}
}
