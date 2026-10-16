package handler

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// PlatformHandler serves the DEVELOPER-only cross-tenant platform window (read
// AND, as of the admin console, tenant administration).
//
// Every route in this handler is a SYSTEM-LEVEL operation (no WithTenant) and
// MUST be gated by RequireRole(RoleDeveloper) at the router — the strongest
// gate; no customer role (OWNER, ADMIN, …) can reach it. See server.go.
type PlatformHandler struct {
	businessRepo *database.PlatformBusinessRepository
	incidentRepo *database.PlatformIncidentRepository
	pricingRepo  *database.PricingRepository
	db           *database.DB
	authService  *auth.Service
}

func NewPlatformHandler(
	businessRepo *database.PlatformBusinessRepository,
	incidentRepo *database.PlatformIncidentRepository,
	pricingRepo *database.PricingRepository,
	db *database.DB,
	authService *auth.Service,
) *PlatformHandler {
	return &PlatformHandler{businessRepo: businessRepo, incidentRepo: incidentRepo, pricingRepo: pricingRepo, db: db, authService: authService}
}

var pricingDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// GetPricing returns the platform-wide price list to the Developer console.
func (h *PlatformHandler) GetPricing(c *gin.Context) {
	p, err := h.pricingRepo.GetPromo(c.Request.Context())
	if err == database.ErrPricingPromoNotFound {
		c.JSON(http.StatusOK, gin.H{"published": false, "pricing": nil})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load platform pricing"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"published": true, "pricing": p})
}

// PutPricing publishes a complete, validated platform subscription price list.
// This is intentionally separate from tenant portal pricing: only a
// DEVELOPER can change the public platform subscription offer.
func (h *PlatformHandler) PutPricing(c *gin.Context) {
	var p model.PricingPromo
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pricing JSON"})
		return
	}
	if err := validatePricingPromo(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.pricingRepo.UpsertPromo(c.Request.Context(), &p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish platform pricing"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"published": true, "pricing": p})
}

func validatePricingPromo(p *model.PricingPromo) error {
	if p.MonthlyStandardMMK == nil || *p.MonthlyStandardMMK <= 0 || *p.MonthlyStandardMMK > 1000000000 {
		return fmt.Errorf("monthly_standard_mmk must be between 1 and 1000000000")
	}
	if p.AnnualStandardMMK == nil || *p.AnnualStandardMMK <= 0 || *p.AnnualStandardMMK > 1000000000 {
		return fmt.Errorf("annual_standard_mmk must be between 1 and 1000000000")
	}
	if p.MonthlyPromoMMK != nil && (*p.MonthlyPromoMMK <= 0 || *p.MonthlyPromoMMK >= *p.MonthlyStandardMMK) {
		return fmt.Errorf("monthly_promo_mmk must be positive and below monthly_standard_mmk")
	}
	if p.AnnualPromoMMK != nil && (*p.AnnualPromoMMK <= 0 || *p.AnnualPromoMMK >= *p.AnnualStandardMMK) {
		return fmt.Errorf("annual_promo_mmk must be positive and below annual_standard_mmk")
	}
	if (p.StartsOn == nil) != (p.EndsOn == nil) {
		return fmt.Errorf("starts_on and ends_on must be supplied together")
	}
	if p.StartsOn != nil {
		if !pricingDatePattern.MatchString(*p.StartsOn) || !pricingDatePattern.MatchString(*p.EndsOn) {
			return fmt.Errorf("starts_on and ends_on must use YYYY-MM-DD")
		}
		if *p.StartsOn > *p.EndsOn {
			return fmt.Errorf("starts_on must not be after ends_on")
		}
	}
	if p.Active && (p.StartsOn == nil || p.MonthlyPromoMMK == nil || p.AnnualPromoMMK == nil) {
		return fmt.Errorf("an active promotion requires dates and both promo prices")
	}
	if p.Label != nil && len(strings.TrimSpace(*p.Label)) > 120 {
		return fmt.Errorf("label must be 120 characters or fewer")
	}
	return nil
}

// ListBusinesses returns every business across all tenants (DEVELOPER only).
func (h *PlatformHandler) ListBusinesses(c *gin.Context) {
	bizs, err := h.businessRepo.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list businesses"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"businesses": bizs, "count": len(bizs)})
}

// ListOpenIncidents returns open incidents across every business (DEVELOPER only).
func (h *PlatformHandler) ListOpenIncidents(c *gin.Context) {
	inxs, err := h.incidentRepo.ListOpenAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list incidents"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"incidents": inxs, "count": len(inxs)})
}

// CountIncidents returns cross-tenant incident counts for the platform overview.
func (h *PlatformHandler) CountIncidents(c *gin.Context) {
	counts, err := h.incidentRepo.CountAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count incidents"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"counts": counts})
}

// validTiers is DERIVED from the model's ordered tier list rather than repeated
// here, so the set a tenant can be created with and the set the pricing page
// advertises cannot drift apart: adding a tier in one place makes it both
// creatable and publishable.
var validTiers = func() map[string]bool {
	m := make(map[string]bool, len(model.AccessTiers))
	for _, tier := range model.AccessTiers {
		m[tier] = true
	}
	return m
}()

// CreateBusiness: POST /platform/businesses — create a tenant atomically.
func (h *PlatformHandler) CreateBusiness(c *gin.Context) {
	var body struct {
		CompanyName      string `json:"company_name"`
		OwnerEmail       string `json:"owner_email"`
		OwnerPassword    string `json:"owner_password"`
		AccessTier       string `json:"access_tier"`
		DeviceQuotaLimit int    `json:"device_quota_limit"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	body.CompanyName = strings.TrimSpace(body.CompanyName)
	body.OwnerEmail = strings.ToLower(strings.TrimSpace(body.OwnerEmail))
	tier := strings.ToUpper(strings.TrimSpace(body.AccessTier))
	if tier == "" {
		tier = "SMALL_WISP"
	}
	if body.CompanyName == "" || body.OwnerEmail == "" || len(body.OwnerPassword) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_name, owner_email and an 8+ char owner_password are required"})
		return
	}
	if !validTiers[tier] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid access_tier"})
		return
	}

	hash, err := h.authService.HashPassword(body.OwnerPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to secure password"})
		return
	}

	businessID, ownerUserID, err := h.businessRepo.CreateTenant(body.CompanyName, body.OwnerEmail, hash, tier, body.DeviceQuotaLimit)
	if err != nil {
		if err == database.ErrEmailInUse {
			c.JSON(http.StatusConflict, gin.H{"error": "owner email already registered"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create tenant"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"business_id": businessID, "owner_user_id": ownerUserID})
}

// SuspendBusiness: POST /platform/businesses/:businessId/suspend
func (h *PlatformHandler) SuspendBusiness(c *gin.Context) {
	h.setActive(c, false)
}

// UnsuspendBusiness: POST /platform/businesses/:businessId/unsuspend
func (h *PlatformHandler) UnsuspendBusiness(c *gin.Context) {
	h.setActive(c, true)
}

func (h *PlatformHandler) setActive(c *gin.Context, active bool) {
	businessID := c.Param("businessId")
	// Never allow an admin to suspend their own tenant — that self-locks the
	// account (is_active=false on the business cascades to all its users,
	// including the DEVELOPER) and there is then no route back in to unsuspend.
	if !active && businessID != "" && businessID == c.GetString(auth.ContextKeyBusinessID) {
		c.JSON(http.StatusConflict, gin.H{"error": "cannot suspend your own tenant"})
		return
	}
	if err := h.businessRepo.SetBusinessActive(businessID, active); err != nil {
		if err == database.ErrTenantNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tenant"})
		return
	}
	status := "suspended"
	if active {
		status = "active"
	}
	c.JSON(http.StatusOK, gin.H{"business_id": businessID, "status": status})
}

// ResetOwnerPassword: POST /platform/businesses/:businessId/reset-owner-password
func (h *PlatformHandler) ResetOwnerPassword(c *gin.Context) {
	businessID := c.Param("businessId")
	var body struct {
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.NewPassword) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new_password (8+ chars) is required"})
		return
	}
	hash, err := h.authService.HashPassword(body.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to secure password"})
		return
	}
	if err := h.businessRepo.ResetOwnerPassword(businessID, hash); err != nil {
		if err == database.ErrTenantNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "tenant or owner not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset owner password"})
		return
	}
	// 204 No Content; the plaintext is never stored or logged.
	c.Status(http.StatusNoContent)
}

// TenantState: GET /platform/businesses/:businessId/state
func (h *PlatformHandler) TenantState(c *gin.Context) {
	businessID := c.Param("businessId")
	st, err := h.businessRepo.State(businessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read tenant state"})
		return
	}
	if st == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"business_id":           st.BusinessID,
		"company_name":          st.CompanyName,
		"access_tier":           st.AccessTier,
		"is_active":             st.IsActive,
		"owner_email":           st.OwnerEmail,
		"device_quota_limit":    st.DeviceQuotaLimit,
		"device_count":          st.DeviceCount,
		"open_incidents":        st.OpenIncidents,
		"purchased_extra_slots": st.PurchasedExtra,
		"ai_token_balance":      st.AITokenBalance,
	})
}

var _ = strconv.Itoa
var _ = model.RoleDeveloper
