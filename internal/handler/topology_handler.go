package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/messaging"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner"
	"mega-magic/internal/topology"
)

// TopologyHandler exposes the read-only per-tenant network map plus the
// explicit device-action surface (reboot). It is the only I/O boundary of the
// topology package: it pulls the tenant's devices (RLS-bound via the pinned
// connection) and hands the slice to the pure BuildMap function; device actions
// are published as intents for the topology action consumer to drive.
type TopologyHandler struct {
	SubscriptionOnly bool
	db               *database.DB
	deviceRepo       *database.DeviceRepository
	quotaRepo        *database.BusinessMeteredQuotaRepository
	msg              *messaging.Client
}

func NewTopologyHandler(db *database.DB, deviceRepo *database.DeviceRepository, quotaRepo *database.BusinessMeteredQuotaRepository, msg *messaging.Client) *TopologyHandler {
	return &TopologyHandler{db: db, deviceRepo: deviceRepo, quotaRepo: quotaRepo, msg: msg}
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

// planActive reports whether the tenant's Sovereign pass is currently active —
// the same condition the quota-enforcement sweep and VPS handler use.
func (h *TopologyHandler) planActive(businessID string) bool {
	if h.SubscriptionOnly {
		return true
	} // Shared subscription middleware enforces base access.
	q, err := h.quotaRepo.GetByBusiness(businessID)
	if err != nil || q == nil || q.WebfigPassExpiresAt == nil {
		return false
	}
	return q.WebfigPassExpiresAt.After(time.Now())
}

// Reboot accepts an explicit force-reboot request for one of the tenant's
// devices. Sovereign-plan gated, tenant-bound (404 for devices outside the
// tenant), and emitted as a topology.action intent for the (flag-gated)
// consumer to drive through approval + apply. This endpoint ONLY records the
// intent — the router is never touched from here.
func (h *TopologyHandler) Reboot(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if h.quotaRepo == nil || !h.planActive(businessID) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "device reboot requires an active Sovereign pass",
			"code":  "plan_required",
		})
		return
	}
	deviceID := c.Param("deviceId")

	var device *model.Device
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		d, err := h.deviceRepo.GetDeviceByIDTXN(tx, deviceID)
		if err != nil {
			return err
		}
		device = d
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if device == nil || device.BusinessID != businessID {
		// 404 for both absent and cross-tenant devices keeps identity from
		// leaking — ownership must be confirmed here regardless of RLS state.
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found in your tenant"})
		return
	}

	if h.msg == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provisioning bus unavailable"})
		return
	}
	data, jerr := json.Marshal(map[string]string{
		"business_id": businessID,
		"device_id":   deviceID,
		"action":      provisioner.ActionReboot,
	})
	if jerr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode reboot intent"})
		return
	}
	pubErr := h.msg.Publish(businessID, provisioner.SubjectTopologyAction, data)
	if pubErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue reboot intent"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"device_id": deviceID,
		"status":    "reboot intent queued for approval",
		"action":    provisioner.ActionReboot,
	})
}
