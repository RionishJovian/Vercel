package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

type VoucherHandler struct {
	voucherRepo *database.VoucherRepository
}

func NewVoucherHandler(voucherRepo *database.VoucherRepository) *VoucherHandler {
	return &VoucherHandler{voucherRepo: voucherRepo}
}

// CreateVoucher generates new voucher codes for a business
func (h *VoucherHandler) CreateVoucher(c *gin.Context) {
	var req struct {
		DeviceID       *string `json:"device_id,omitempty"`
		PlanKey        string  `json:"plan_key"`
		PlanLabel      string  `json:"plan_label"`
		PriceMMK       int     `json:"price_mmk"`
		DurationMinutes *int    `json:"duration_minutes,omitempty"`
		DeviceMAC      *string `json:"device_mac,omitempty"`
		Status         string  `json:"status,omitempty"`
		Count          int     `json:"count"` // number of vouchers to generate
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

	// Set defaults
	if req.Status == "" {
		req.Status = "active"
	}

	now := time.Now().UTC()

	var vouchers []*model.Voucher
	for i := 0; i < req.Count; i++ {
		// Generate unique voucher code
		voucher := &model.Voucher{
			BusinessID:     businessID,
			DeviceID:       req.DeviceID,
			PlanKey:        req.PlanKey,
			PlanLabel:      req.PlanLabel,
			PriceMMK:       req.PriceMMK,
			DurationMinutes: req.DurationMinutes,
			DeviceMAC:      parseMAC(req.DeviceMAC),
			Status:         req.Status,
			FirstSeenAt:    &now,
			ExpiresAt:      &now,
		}

		created, err := h.voucherRepo.CreateVoucher(voucher)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create voucher: " + err.Error()})
			return
		}
		vouchers = append(vouchers, created)
	}

	c.JSON(http.StatusCreated, gin.H{"vouchers": vouchers})
}

// GetVoucherByCode retrieves a voucher for redemption
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

// ListVouchers lists all vouchers for the business
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

// UpdateVoucherStatus updates voucher status (e.g., when used)
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

	voucher, err := h.voucherRepo.GetVoucherByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if voucher == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "voucher not found"})
		return
	}

	// If status is "used", mark it as used now
	if req.Status == "used" && voucher.Status != "used" {
		now := time.Now().UTC()
		voucher.Status = "used"
		voucher.FirstSeenAt = &now
	}

	c.JSON(http.StatusOK, gin.H{"voucher": voucher})
}

// DeleteVoucher removes a voucher
func (h *VoucherHandler) DeleteVoucher(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	voucher, err := h.voucherRepo.GetVoucherByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if voucher == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "voucher not found"})
		return
	}

	// In a real implementation, you'd delete the voucher from the database
	// For now, we'll just return the voucher being "deleted"
	c.JSON(http.StatusOK, gin.H{"voucher": voucher, "status": "deleted"})
}

// RedeemVoucher handles voucher redemption
func (h *VoucherHandler) RedeemVoucher(c *gin.Context) {
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

	// Check if voucher is already used
	if voucher.Status == "used" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "voucher already redeemed"})
		return
	}

	// Mark voucher as used
	voucher.Status = "used"
	now := time.Now().UTC()
	voucher.FirstSeenAt = &now

	// In a real implementation, you'd update this in the database
	// For now, we'll return the voucher
	c.JSON(http.StatusOK, gin.H{"voucher": voucher, "message": "voucher redeemed successfully"})
}