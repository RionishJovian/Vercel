package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/fleet"
)

// FleetHandler exposes the read-only fleet aggregate (peer-health overview).
type FleetHandler struct {
	fleetSvc *fleet.FleetService
}

func NewFleetHandler(fleetSvc *fleet.FleetService) *FleetHandler {
	return &FleetHandler{fleetSvc: fleetSvc}
}

// FleetHealth returns the tenant's fleet picture: total peers, per-state counts,
// overall severity, and per-peer rows. Owner-scoped to the JWT business_id.
func (h *FleetHandler) FleetHealth(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	health, err := h.fleetSvc.Health(c.Request.Context(), businessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build fleet health: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, health)
}

// OfflineHealth returns only peers whose state is offline, owner-scoped.
func (h *FleetHandler) OfflineHealth(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	health, err := h.fleetSvc.Health(c.Request.Context(), businessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build fleet health: " + err.Error()})
		return
	}

	// Filter to only offline peers
	var offline []fleet.PeerSummary
	for _, p := range health.Peers {
		if p.State == fleet.StateOffline {
			offline = append(offline, p)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"business_id": health.BusinessID,
		"total_peers": health.Total,
		"offline_peers": len(offline),
		"offline":     offline,
		"overall_state": health.Overall,
	})
}