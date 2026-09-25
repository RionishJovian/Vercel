package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/monitoring"

	"strings"
)

type MonitorHandler struct {
	db         *database.DB
	deviceRepo *database.DeviceRepository
	monitoring *monitoring.MonitoringService
}

func NewMonitorHandler(db *database.DB, deviceRepo *database.DeviceRepository, mon *monitoring.MonitoringService) *MonitorHandler {
	return &MonitorHandler{db: db, deviceRepo: deviceRepo, monitoring: mon}
}

// withTenantTx runs fn in an RLS-bound tenant transaction (pinned conn when
// database.rls_enforce is on, owner tx otherwise).
func (h *MonitorHandler) withTenantTx(c *gin.Context, businessID string, fn func(tx *sql.Tx) error) error {
	return runInTenant(c, h.db, businessID, fn)
}

// looksLikeUUID returns true when s is a canonical 8-4-4-4-12 hyphenated hex
// identifier (the shape of devices.device_id, a uuid column). Rejecting
// malformed ids up-front avoids a Postgres 'invalid input syntax for type uuid'
// error leaking as a 500 when a caller passes a non-uuid string.
func looksLikeUUID(s string) bool {
	parts := strings.Split(s, "-")
	if len(parts) != 5 {
		return false
	}
	want := []int{8, 4, 4, 4, 12}
	for i := 0; i < 5; i++ {
		if len(parts[i]) != want[i] {
			return false
		}
	}
	return true
}

// PeerState returns the current cached (or freshly evaluated) reachability
// state for a peer, owner-scoped to the tenant.
func (h *MonitorHandler) PeerState(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	peerID := c.Param("peerId")
	if peerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "peer_id required"})
		return
	}
	// device_id is a uuid column; a malformed id makes Postgres throw instead of
	// returning no rows, so reject non-uuid-shaped input as "not found" up-front.
	if !looksLikeUUID(peerID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "peer not found"})
		return
	}

	// Ownership check: the peer must belong to this tenant's devices.
	var dev *model.Device
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		d, err := h.deviceRepo.GetDeviceByIDTXN(tx, peerID)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		dev = d
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}
	if dev == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "peer not found"})
		return
	}
	if dev.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	state, err := h.monitoring.StateForPeer(c.Request.Context(), businessID, peerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to evaluate peer state: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"peer_id": peerID, "state": state})
}
