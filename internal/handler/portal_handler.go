package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// PortalHandler serves the captive-portal surface.
//
// PUBLIC endpoints are anonymous (no auth): the tenant is resolved from the
// DEVICE the client is attached to (device_id -> devices.business_id), never
// from the voucher code (codes are only business-scoped-unique). STAFF endpoints
// are owner-authenticated and mounted under `protected`.
type PortalHandler struct {
	db          *database.DB
	deviceRepo  *database.DeviceRepository
	configRepo  *database.PortalConfigRepository
	sessionRepo *database.PortalSessionRepository
	voucherRepo *database.VoucherRepository
	quotaRepo   *database.BusinessMeteredQuotaRepository
}

func NewPortalHandler(
	db *database.DB,
	deviceRepo *database.DeviceRepository,
	configRepo *database.PortalConfigRepository,
	sessionRepo *database.PortalSessionRepository,
	voucherRepo *database.VoucherRepository,
	quotaRepo *database.BusinessMeteredQuotaRepository,
) *PortalHandler {
	return &PortalHandler{
		db:          db,
		deviceRepo:  deviceRepo,
		configRepo:  configRepo,
		sessionRepo: sessionRepo,
		voucherRepo: voucherRepo,
		quotaRepo:   quotaRepo,
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

	c.JSON(http.StatusOK, gin.H{"config": cfg})
}

// PublicRedeem: POST /api/v1/portal/:deviceId/redeem
func (h *PortalHandler) PublicRedeem(c *gin.Context) {
	var req struct {
		ClientMAC string  `json:"client_mac" binding:"required"`
		Code      string  `json:"code" binding:"required"`
		ClientIP  *string `json:"client_ip"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

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
			ConsentGiven:    false,
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
		BusinessName    string  `json:"business_name" binding:"required"`
		WelcomeText     *string `json:"welcome_text"`
		TermsText       *string `json:"terms_text"`
		PrimaryHex      *string `json:"primary_hex"`
		GlassTintHex    *string `json:"glass_tint_hex"`
		ConsentRequired *bool   `json:"consent_required"`
		ConsentText     *string `json:"consent_text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg := &model.PortalConfig{
		BusinessID:   businessID,
		BusinessName: req.BusinessName,
	}
	if req.WelcomeText != nil {
		cfg.WelcomeText = *req.WelcomeText
	}
	if req.TermsText != nil {
		cfg.TermsText = *req.TermsText
	}
	if req.PrimaryHex != nil {
		cfg.PrimaryHex = *req.PrimaryHex
	}
	if req.GlassTintHex != nil {
		cfg.GlassTintHex = *req.GlassTintHex
	}
	if req.ConsentRequired != nil {
		cfg.ConsentRequired = *req.ConsentRequired
	}
	if req.ConsentText != nil {
		cfg.ConsentText = *req.ConsentText
	}

	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		return h.configRepo.UpsertTXN(tx, cfg)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update portal config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "config": cfg})
}
