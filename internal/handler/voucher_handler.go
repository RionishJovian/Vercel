package handler

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/voucher"
)

const (
	voucherCodeAttempts = 5 // unique-code retries before giving up
	voucherMaxBatch     = 1000
)

type VoucherHandler struct {
	db          *database.DB
	voucherRepo *database.VoucherRepository
	quotaRepo   *database.BusinessMeteredQuotaRepository
}

// NewVoucherHandler wires the voucher repo, the metered-quota repo used to apply
// a voucher's entitlement, and the DB handle needed to run redemption as one
// transaction (claim + entitlement atomically).
func NewVoucherHandler(db *database.DB, voucherRepo *database.VoucherRepository, quotaRepo *database.BusinessMeteredQuotaRepository) *VoucherHandler {
	return &VoucherHandler{db: db, voucherRepo: voucherRepo, quotaRepo: quotaRepo}
}

// CreateVoucher generates new voucher codes for a business.
func (h *VoucherHandler) CreateVoucher(c *gin.Context) {
	var req struct {
		DeviceID        *string `json:"device_id,omitempty"`
		PlanKey         string  `json:"plan_key"`
		PlanLabel       string  `json:"plan_label"`
		PriceMMK        int     `json:"price_mmk"`
		DurationMinutes *int    `json:"duration_minutes,omitempty"`
		DeviceMAC       *string `json:"device_mac,omitempty"`
		Status          string  `json:"status,omitempty"`
		Count           int     `json:"count"` // number of vouchers to generate
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if req.Status == "" {
		req.Status = voucher.StatusActive
	}
	count := req.Count
	if count <= 0 {
		count = 1
	}
	if count > voucherMaxBatch {
		count = voucherMaxBatch
	}

	now := time.Now().UTC()
	// Expiry is now + duration (or NULL when no duration) — NOT creation time.
	expiresAt := voucher.ComputeExpiry(now, req.DurationMinutes)

	vouchers := make([]*model.Voucher, 0, count)
	for i := 0; i < count; i++ {
		created, err := h.createOneVoucher(businessID, req.PlanKey, req.PlanLabel, req.PriceMMK,
			req.DurationMinutes, req.DeviceMAC, req.DeviceID, req.Status, expiresAt, now)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create voucher: " + err.Error()})
			return
		}
		vouchers = append(vouchers, created)
	}

	c.JSON(http.StatusCreated, gin.H{"vouchers": vouchers})
}

// createOneVoucher generates a unique code and inserts the voucher, retrying on
// a unique-constraint collision (codes are business-scoped-unique).
func (h *VoucherHandler) createOneVoucher(businessID, planKey, planLabel string, priceMMK int,
	durationMinutes *int, deviceMAC, deviceID *string, status string, expiresAt *time.Time, now time.Time,
) (*model.Voucher, error) {
	var lastErr error
	for attempt := 0; attempt < voucherCodeAttempts; attempt++ {
		code, err := voucher.GenerateCode(rand.Reader, voucher.DefaultCodeLength)
		if err != nil {
			return nil, err
		}
		v := &model.Voucher{
			BusinessID:      businessID,
			DeviceID:        deviceID,
			PlanKey:         planKey,
			PlanLabel:       planLabel,
			Code:            code,
			PriceMMK:        priceMMK,
			DurationMinutes: durationMinutes,
			DeviceMAC:       parseMAC(deviceMAC),
			Status:          status,
			// first_seen_at is stamped at redemption, not creation.
			FirstSeenAt: nil,
			ExpiresAt:   expiresAt,
		}
		created, err := h.voucherRepo.CreateVoucher(v)
		if err == nil {
			return created, nil
		}
		lastErr = err
		if !isUniqueViolation(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

// isUniqueViolation reports whether err is a postgres unique-constraint error.
func isUniqueViolation(err error) bool {
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// GetVoucherByCode retrieves a voucher for redemption.
func (h *VoucherHandler) GetVoucherByCode(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	code := c.Param("code")
	voucher, err := h.voucherRepo.GetVoucherByCode(businessID, code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if voucher == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "voucher not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"voucher": voucher})
}

// ListVouchers lists all vouchers for the business.
func (h *VoucherHandler) ListVouchers(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	vouchers, err := h.voucherRepo.ListVouchersByBusiness(businessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"vouchers": vouchers})
}

// UpdateVoucherStatus updates voucher status (e.g., when used) and PERSISTS it.
func (h *VoucherHandler) UpdateVoucherStatus(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	var req struct {
		Status      string     `json:"status"`
		FirstSeenAt *time.Time `json:"first_seen_at,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Ownership check: never allow a cross-tenant edit by id.
	voucher, err := h.voucherRepo.GetVoucherByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if voucher == nil || voucher.BusinessID != businessID {
		c.JSON(http.StatusNotFound, gin.H{"error": "voucher not found"})
		return
	}

	status := req.Status
	if status == "" {
		status = voucher.Status
	}

	updated, err := h.voucherRepo.UpdateVoucherStatusByBusiness(businessID, id, status, req.FirstSeenAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if updated == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "voucher not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"voucher": updated})
}

// DeleteVoucher removes a voucher, but NEVER a redeemed (used) one.
func (h *VoucherHandler) DeleteVoucher(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	deleted, err := h.voucherRepo.DeleteVoucherIfUnused(businessID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !deleted {
		// Distinguish "not ours / not found" from "redeemed, cannot delete".
		existing, gerr := h.voucherRepo.GetVoucherByID(id)
		if gerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gerr.Error()})
			return
		}
		if existing == nil || existing.BusinessID != businessID {
			c.JSON(http.StatusNotFound, gin.H{"error": "voucher not found"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": "redeemed vouchers cannot be deleted"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": id})
}

// RedeemVoucher handles voucher redemption: atomically claim, apply the plan's
// entitlement, and return the redeemed voucher.
func (h *VoucherHandler) RedeemVoucher(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	code := c.Param("code")
	now := time.Now().UTC()

	// Pre-check for a precise error (the claim below is the authoritative gate).
	existing, err := h.voucherRepo.GetVoucherByCode(businessID, code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "voucher not found"})
		return
	}
	if reason := voucher.RedeemBlockReason(existing, now); reason != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": reason})
		return
	}

	// Fail closed on an unknown plan BEFORE claiming anything.
	effect, err := voucher.ResolvePlanEffect(existing.PlanKey)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if effect.ExtendWebfigPass && (existing.DurationMinutes == nil || *existing.DurationMinutes <= 0) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "voucher plan requires duration_minutes to grant a pass"})
		return
	}

	// One atomic unit: claim the voucher AND grant the entitlement in the SAME
	// transaction (mirrors auth_handler.go's register flow). If anything fails
	// (or the process dies) before commit, the deferred Rollback leaves the
	// voucher untouched — a paid voucher is never consumed without its
	// entitlement, and there is no crash window between two commits.
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer tx.Rollback()

	claimed, err := h.voucherRepo.ClaimVoucherTXN(tx, businessID, code, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if claimed == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "voucher is no longer redeemable"})
		return
	}

	applied, err := h.applyPlanEffectTXN(tx, businessID, effect, *claimed.DurationMinutes, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to apply voucher entitlement: " + err.Error()})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize redemption"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"voucher": claimed,
		"applied": applied,
		"message": "voucher redeemed successfully",
	})
}

// applyPlanEffectTXN grants the plan entitlement inside the redemption tx and
// returns a summary. Reading the current pass and writing the new expiry use the
// same *sql.Tx so the whole redemption is atomic.
func (h *VoucherHandler) applyPlanEffectTXN(tx *sql.Tx, businessID string, effect voucher.PlanEffect, durationMinutes int, now time.Time) (gin.H, error) {
	if h.quotaRepo == nil {
		return nil, errors.New("no quota repository configured")
	}
	if !effect.ExtendWebfigPass {
		return gin.H{"plan_key": effect.PlanKey, "effect": "none"}, nil
	}

	current, err := h.quotaRepo.GetByBusinessTXN(tx, businessID)
	if err != nil {
		return nil, err
	}
	var currentExpiry *time.Time
	if current != nil {
		currentExpiry = current.WebfigPassExpiresAt
	}
	newExpiry := voucher.ExtendExpiry(currentExpiry, now, durationMinutes)

	if current == nil {
		q := &model.BusinessMeteredQuota{BusinessID: businessID, WebfigPassExpiresAt: &newExpiry}
		if err := h.quotaRepo.UpsertTXN(tx, q); err != nil {
			return nil, err
		}
	} else if err := h.quotaRepo.ExtendWebfigPassTXN(tx, businessID, newExpiry); err != nil {
		return nil, err
	}

	return gin.H{
		"plan_key":                 effect.PlanKey,
		"effect":                   "extend_webfig_pass",
		"webfig_pass_expires_at":   newExpiry,
		"duration_minutes_applied": durationMinutes,
	}, nil
}
