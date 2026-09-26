package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/topology"
)

// TopologyHandler exposes the read-only per-tenant network map. It is the only
// I/O boundary of the topology package: it pulls the tenant's devices (RLS-bound
// via the pinned connection) and hands the slice to the pure BuildMap function.
type TopologyHandler struct {
	db         *database.DB
	deviceRepo *database.DeviceRepository
}

func NewTopologyHandler(db *database.DB, deviceRepo *database.DeviceRepository) *TopologyHandler {
	return &TopologyHandler{db: db, deviceRepo: deviceRepo}
}

// Map returns the tenant's network topology graph: parent/child tree, orphans,
// any cycles, and a summary. Owner-scoped to the JWT business_id (RLS keeps
// cross-tenant devices invisible on the pinned connection).
func (h *TopologyHandler) Map(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var devices []*model.Device
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		devs, err := h.deviceRepo.ListDevicesByBusinessTXN(tx, businessID)
		if err != nil {
			return err
		}
		devices = devs
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build topology: " + err.Error()})
		return
	}

	m := topology.BuildMap(businessID, devices)
	c.JSON(http.StatusOK, m)
}
