package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// PlatformHandler serves the DEVELOPER-only cross-tenant platform window.
// Every route in this handler is a SYSTEM-LEVEL read (no WithTenant) and MUST
// be gated by RequireRole(model.RoleDeveloper) at the router, never reachable
// by a customer role. See server.go. This is the platform operator's lens, not
// a tenant-scoped view.
type PlatformHandler struct {
	businessRepo *database.PlatformBusinessRepository
	incidentRepo *database.PlatformIncidentRepository
}

func NewPlatformHandler(
	businessRepo *database.PlatformBusinessRepository,
	incidentRepo *database.PlatformIncidentRepository,
) *PlatformHandler {
	return &PlatformHandler{businessRepo: businessRepo, incidentRepo: incidentRepo}
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

// requireDeveloper is a self-documenting gate used here and at route wiring.
func requireDeveloper() gin.HandlerFunc {
	return auth.RequireRole(string(model.RoleDeveloper))
}
