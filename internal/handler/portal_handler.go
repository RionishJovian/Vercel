package handler

import (
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
)

// deref returns the value behind an optional string, or "" when unset.
func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// PortalHandler serves the captive-portal surface.
//
// PUBLIC endpoints are anonymous (no auth): the tenant is resolved from the
// DEVICE the client is attached to (device_id -> devices.business_id), never
// from the voucher code (codes are only business-scoped-unique). STAFF endpoints
// are owner-authenticated and mounted under `protected`.
type PortalHandler struct {
	GuestVoucherAccess  bool
	db                  *database.DB
	deviceRepo          *database.DeviceRepository
	configRepo          *database.PortalConfigRepository
	sessionRepo         *database.PortalSessionRepository
	voucherRepo         *database.VoucherRepository
	quotaRepo           *database.BusinessMeteredQuotaRepository
	notifyGuestRedeemed func(businessID, planLabel string)
}

func NewPortalHandler(
	db *database.DB,
	deviceRepo *database.DeviceRepository,
	configRepo *database.PortalConfigRepository,
	sessionRepo *database.PortalSessionRepository,
	voucherRepo *database.VoucherRepository,
	quotaRepo *database.BusinessMeteredQuotaRepository,
	notifyGuestRedeemed func(businessID, planLabel string),
) *PortalHandler {
	return &PortalHandler{
		db:                  db,
		deviceRepo:          deviceRepo,
		configRepo:          configRepo,
		sessionRepo:         sessionRepo,
		voucherRepo:         voucherRepo,
		quotaRepo:           quotaRepo,
		notifyGuestRedeemed: notifyGuestRedeemed,
	}
}

// withTenantTx is used by the STAFF (owner-authenticated) endpoints.
func (h *PortalHandler) withTenantTx(c *gin.Context, businessID string, fn func(tx *sql.Tx) error) error {
	return runInTenant(c, h.db, businessID, fn)
}

// businessForDevice resolves the tenant anchor for an anonymous client.
//
// This is an intentional SYSTEM-LEVEL read (owner): the device row is precisely
// the thing that tells us which tenant an anonymous client belongs to. It must be
// read BEFORE any tenant context exists. Returns (nil, nil) when unknown.
func (h *PortalHandler) businessForDevice(deviceID string) (*model.Device, error) {
	return h.deviceRepo.GetDeviceByID(deviceID)
}

// --- public (anonymous) -----------------------------------------------------

// PublicConfig: GET /api/v1/portal/:deviceId/config
func (h *PortalHandler) PublicConfig(c *gin.Context) {
	deviceID := c.Param("deviceId")
	dev, err := h.businessForDevice(deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if dev == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	var cfg *model.PortalConfig
	err = withDeviceTenant(h.db, dev.BusinessID, func(tx *sql.Tx) error {
		pc, e := h.configRepo.GetByBusinessTXN(tx, dev.BusinessID)
		if e != nil {
			return e
		}
		cfg = pc
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if cfg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "portal config not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"config": cfg, "guest_voucher_access": h.GuestVoucherAccess})
}

// PublicRedeem: POST /api/v1/portal/:deviceId/redeem
func (h *PortalHandler) PublicRedeem(c *gin.Context) {
	var req struct {
		ClientMAC string  `json:"client_mac" binding:"required"`
		Code      string  `json:"code" binding:"required"`
		ClientIP  *string `json:"client_ip"`
		Consent   bool    `json:"consent"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mac, macErr := net.ParseMAC(req.ClientMAC)
	if macErr != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid client_mac required"})
		return
	}
	req.ClientMAC = strings.ToUpper(mac.String())

	deviceID := c.Param("deviceId")
	dev, err := h.businessForDevice(deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if dev == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	now := time.Now().UTC()
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()

	var (
		session *model.PortalSession
		applied gin.H
	)
	err = database.WithTenant(tx, dev.BusinessID, func() error {
		cfg, e := h.configRepo.GetByBusinessTXN(tx, dev.BusinessID)
		if e != nil {
			return e
		}
		if cfg == nil || !cfg.VoucherLoginEnabled {
			return &httpError{status: http.StatusForbidden, msg: "voucher login is disabled for this portal"}
		}
		if cfg.ConsentRequired && !req.Consent {
			return &httpError{status: http.StatusBadRequest, msg: "accept the hotspot terms before connecting"}
		}
		claimed, ap, e := redeemVoucherTx(tx, h.voucherRepo, h.quotaRepo, dev.BusinessID, req.Code, now)
		if e != nil {
			return e
		}
		applied = ap

		var expiresAt *time.Time
		if claimed.DurationMinutes != nil && *claimed.DurationMinutes > 0 {
			t := now.Add(time.Duration(*claimed.DurationMinutes) * time.Minute)
			expiresAt = &t
		}
		deviceIDCopy := deviceID
		s, e := h.sessionRepo.CreateTXN(tx, &model.PortalSession{
			BusinessID:      dev.BusinessID,
			DeviceID:        &deviceIDCopy,
			VoucherID:       &claimed.ID,
			ClientMAC:       req.ClientMAC,
			ClientIP:        parseIP(req.ClientIP),
			AuthenticatedAt: now,
			ExpiresAt:       expiresAt,
			IsActive:        true,
			ConsentGiven:    req.Consent,
		})
		if e != nil {
			return e
		}
		session = s
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize redemption"})
		return
	}

	// Option C: notify the OWNER's own bot that a guest redeemed a printed
	// voucher (informational — the voucher is pre-paid, so no approval needed).
	// Fail closed: nil notifier (feature off / no owner bot) means no message,
	// and a failed send never fails the redemption.
	if h.notifyGuestRedeemed != nil {
		h.notifyGuestRedeemed(dev.BusinessID, "guest redeemed a voucher on the captive portal")
	}

	c.JSON(http.StatusCreated, gin.H{
		"session": session,
		"applied": applied,
		"message": "voucher redeemed successfully",
	})
}

// PublicStatus: GET /api/v1/portal/status?client_mac=...
func (h *PortalHandler) PublicStatus(c *gin.Context) {
	clientMAC := c.Query("client_mac")
	if clientMAC == "" {
		var req struct {
			ClientMAC string `json:"client_mac"`
		}
		if err := c.ShouldBindJSON(&req); err == nil {
			clientMAC = req.ClientMAC
		}
	}
	if clientMAC == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client_mac required"})
		return
	}

	// System-level (MAC-only) lookup: the anonymous client has no tenant context
	// until we find their session; this is the GetActiveByMAC seam.
	sess, err := h.sessionRepo.GetActiveByMAC(clientMAC)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if sess == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no active session"})
		return
	}

	var businessName string
	err = withDeviceTenant(h.db, sess.BusinessID, func(tx *sql.Tx) error {
		cfg, e := h.configRepo.GetByBusinessTXN(tx, sess.BusinessID)
		if e != nil {
			return e
		}
		if cfg != nil {
			businessName = cfg.BusinessName
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"is_active":        sess.IsActive,
		"authenticated_at": sess.AuthenticatedAt,
		"expires_at":       sess.ExpiresAt,
		"business_name":    businessName,
	})
}

// --- staff (owner-authenticated) --------------------------------------------

// StaffGetConfig: GET /api/v1/portal/config
func (h *PortalHandler) StaffGetConfig(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var cfg *model.PortalConfig
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		pc, e := h.configRepo.GetByBusinessTXN(tx, businessID)
		if e != nil {
			return e
		}
		cfg = pc
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if cfg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "portal config not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"config": cfg})
}

// StaffUpdateConfig: PUT /api/v1/portal/config
func (h *PortalHandler) StaffUpdateConfig(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		BusinessName         *string            `json:"business_name"`
		WelcomeText          *string            `json:"welcome_text"`
		TermsText            *string            `json:"terms_text"`
		PrimaryHex           *string            `json:"primary_hex"`
		GlassTintHex         *string            `json:"glass_tint_hex"`
		ConsentRequired      *bool              `json:"consent_required"`
		ConsentText          *string            `json:"consent_text"`
		GraceMinutes         *int               `json:"guest_grace_minutes"`
		GraceDataMB          *int               `json:"guest_grace_data_mb"`
		MMQRURL              *string            `json:"mmqr_url"`
		MediaHost            *string            `json:"media_host"`
		MediaPort            *string            `json:"media_port"`
		BankName             *string            `json:"bank_name"`
		BankAccount          *string            `json:"bank_account"`
		BankHolder           *string            `json:"bank_holder"`
		GuestPlans           *[]model.GuestPlan `json:"guest_plans"`
		VoucherLoginEnabled  *bool              `json:"voucher_login_enabled"`
		MobilePaymentEnabled *bool              `json:"mobile_payment_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.GraceMinutes != nil && (*req.GraceMinutes < 1 || *req.GraceMinutes > 1440) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "grace time must be between 1 and 1440 minutes"})
		return
	}
	if req.GraceDataMB != nil && (*req.GraceDataMB < 1 || *req.GraceDataMB > 10240) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "grace data must be between 1 and 10240 MB"})
		return
	}
	if req.MMQRURL != nil && !safePortalImageURL(*req.MMQRURL) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "QR image must be an HTTPS URL or a same-origin path"})
		return
	}
	if req.MediaHost != nil {
		if _, _, mediaErr := routeros.MediaTarget(*req.MediaHost, deref(req.MediaPort)); mediaErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": mediaErr.Error()})
			return
		}
	}
	// Brand colours are owner-supplied strings that end up in the GUEST page's
	// CSS. Reject anything that is not a literal hex colour before it can reach
	// a stylesheet — the client sanitises too, but a public page must not depend
	// on the client having done so.
	if req.PrimaryHex != nil && !model.IsHexColor(*req.PrimaryHex) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "primary_hex must be a hex colour like #ffb547"})
		return
	}
	if req.GlassTintHex != nil && !model.IsHexColor(*req.GlassTintHex) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "glass_tint_hex must be a hex colour like #7ad0ff"})
		return
	}
	if req.GuestPlans != nil {
		// Fail closed on malformed plans: each must be non-empty-ready
		// (Key/Label non-blank) and non-negative in Minutes/Amount/DataMB.
		// Owner-set pricing/data by location (Myanmar market) — validated,
		// not trusted blindly.
		for _, p := range *req.GuestPlans {
			if strings.TrimSpace(p.Key) == "" || strings.TrimSpace(p.Label) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "each guest plan needs a key and a label"})
				return
			}
			if p.Minutes < 0 || p.Amount < 0 || p.DataMB < 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "guest plan minutes, price and data must be >= 0"})
				return
			}
		}
	}

	// Read-modify-write, NOT replace. The upsert writes every column, so a body
	// that omits a field would otherwise erase it: owners lost their bank details
	// and consent flag simply by saving an unrelated change. Load the row, apply
	// only the fields actually supplied, then persist the merged result.
	var merged *model.PortalConfig
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		existing, e := h.configRepo.GetByBusinessTXN(tx, businessID)
		if e != nil {
			return e
		}
		if existing == nil {
			existing = &model.PortalConfig{
				BusinessID:           businessID,
				VoucherLoginEnabled:  true,
				MobilePaymentEnabled: true,
				GuestGraceMinutes:    10,
				GuestGraceDataMB:     50,
			}
		}
		if req.BusinessName != nil {
			existing.BusinessName = strings.TrimSpace(*req.BusinessName)
		}
		if req.WelcomeText != nil {
			existing.WelcomeText = *req.WelcomeText
		}
		if req.TermsText != nil {
			existing.TermsText = *req.TermsText
		}
		if req.PrimaryHex != nil {
			existing.PrimaryHex = *req.PrimaryHex
		}
		if req.GlassTintHex != nil {
			existing.GlassTintHex = *req.GlassTintHex
		}
		if req.MMQRURL != nil {
			existing.MMQRURL = *req.MMQRURL
		}
		if req.MediaHost != nil {
			existing.MediaHost = strings.TrimSpace(*req.MediaHost)
			existing.MediaPort = ""
			if host, port, _ := routeros.MediaTarget(existing.MediaHost, deref(req.MediaPort)); host != "" {
				existing.MediaPort = port
			}
		}
		if req.BankName != nil {
			existing.BankName = *req.BankName
		}
		if req.BankAccount != nil {
			existing.BankAccount = *req.BankAccount
		}
		if req.BankHolder != nil {
			existing.BankHolder = *req.BankHolder
		}
		if req.GraceMinutes != nil {
			existing.GuestGraceMinutes = *req.GraceMinutes
		}
		if req.GraceDataMB != nil {
			existing.GuestGraceDataMB = *req.GraceDataMB
		}
		if req.ConsentRequired != nil {
			existing.ConsentRequired = *req.ConsentRequired
		}
		if req.ConsentText != nil {
			existing.ConsentText = *req.ConsentText
		}
		if req.GuestPlans != nil {
			existing.GuestPlans = *req.GuestPlans
		}
		if req.VoucherLoginEnabled != nil {
			existing.VoucherLoginEnabled = *req.VoucherLoginEnabled
		}
		if req.MobilePaymentEnabled != nil {
			existing.MobilePaymentEnabled = *req.MobilePaymentEnabled
		}
		if !existing.VoucherLoginEnabled && !existing.MobilePaymentEnabled {
			return errPortalAccessMethodRequired
		}
		// A portal with no business name renders a nameless guest page, so this
		// stays required — it just may arrive from the stored row instead.
		if strings.TrimSpace(existing.BusinessName) == "" {
			return errBusinessNameRequired
		}
		if e := h.configRepo.UpsertTXN(tx, existing); e != nil {
			return e
		}
		merged = existing
		return nil
	})
	if err != nil {
		if errors.Is(err, errBusinessNameRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "business_name is required"})
			return
		}
		if errors.Is(err, errPortalAccessMethodRequired) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "enable voucher login or mobile payment"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update portal config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "config": merged})
}

func safePortalImageURL(raw string) bool {
	if raw == "" {
		return true
	}
	if strings.ContainsAny(raw, "\r\n\x00\\") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return false
	}
	if strings.HasPrefix(raw, "/") {
		return !strings.HasPrefix(raw, "//") && u.Host == "" && u.Scheme == ""
	}
	return u.Scheme == "https" && u.Hostname() != ""
}

// errBusinessNameRequired marks the empty-name case so it can answer 400 rather
// than 500 without aborting the transaction from inside the closure.
var errBusinessNameRequired = errors.New("business_name is required")

var errPortalAccessMethodRequired = errors.New("at least one portal access method is required")
