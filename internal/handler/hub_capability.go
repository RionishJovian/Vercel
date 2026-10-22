package handler

import (
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"net/http"
)

// EnrollmentCapability reports whether this deployment can complete the
// one-paste Hub workflow. It exposes feature state only; no Hub credentials,
// router identity or tenant details are returned.
func (h *VPSHandler) EnrollmentCapability(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz := auth.GetBusinessID(c)
	if biz == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	lookupEnabled := h.liveHub != nil
	enrollmentEnabled := h.enrollmentHub != nil && h.liveDevice != nil && h.livePlan != nil
	automaticManagementEnabled := h.automaticPrepare != nil && h.automaticVerify != nil
	planActive := false
	if h.livePlan != nil {
		planActive = h.livePlan(biz)
	}
	code := "ready"
	switch {
	case !lookupEnabled:
		code = "hub_lookup_disabled"
	case !enrollmentEnabled:
		code = "hub_enrollment_disabled"
	case !automaticManagementEnabled:
		code = "automatic_management_disabled"
	case !planActive:
		code = "plan_required"
	}
	c.JSON(http.StatusOK, gin.H{
		"available":                    code == "ready",
		"code":                         code,
		"lookup_enabled":               lookupEnabled,
		"enrollment_enabled":           enrollmentEnabled,
		"automatic_management_enabled": automaticManagementEnabled,
		"plan_active":                  planActive,
	})
}
