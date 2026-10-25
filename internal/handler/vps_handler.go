package handler

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/monitoring"
	"mega-magic/internal/vps"
)

// VPSHandler exposes the read-only WireGuard peer-provisioning surface: hub
// config, per-device peer block, and reachability. All endpoints are Sovereign-plan
// gated (an ACTIVE Webfig pass — the same condition the quota sweep enforces)
// and tenant-bound. No live-router writes; relay/apply stays behind the applier gate.
type VPSHandler struct {
	SubscriptionOnly   bool
	db                 *database.DB
	deviceRepo         *database.DeviceRepository
	quotaRepo          *database.BusinessMeteredQuotaRepository
	monitoring         *monitoring.MonitoringService
	wg                 config.WireGuardConfig
	endpointHost       string
	liveHub            hubInspector
	enrollmentHub      hubEnroller
	liveDevice         func(*gin.Context, string, string) (*model.Device, error)
	livePlan           func(string) bool
	managementAuth     func(context.Context, string, string, string, string, string, int, string) error
	automaticPreflight func(context.Context, string, string) error
	durableQuickKeys   func(context.Context, string, string, bool) (string, string, error)
	automaticPrepare   func(context.Context, string, string, string, string) (string, error)
	automaticVerify    func(context.Context, string, string, string, string) error
	quickKeysMu        sync.Mutex
	quickKeys          map[string]quickEnrollmentKey
}

func NewVPSHandler(db *database.DB, deviceRepo *database.DeviceRepository, quotaRepo *database.BusinessMeteredQuotaRepository, mon *monitoring.MonitoringService, wg config.WireGuardConfig, endpointHost string) *VPSHandler {
	return &VPSHandler{db: db, deviceRepo: deviceRepo, quotaRepo: quotaRepo, monitoring: mon, wg: wg, endpointHost: endpointHost}
}

// planActive reports whether the tenant's Sovereign pass is currently active —
// the same condition the quota-enforcement sweep uses to gate the product.
func (h *VPSHandler) planActive(businessID string) bool {
	if h.SubscriptionOnly {
		return true
	} // Shared subscription middleware enforces base access.
	q, err := h.quotaRepo.GetByBusiness(businessID)
	if err != nil || q == nil || q.WebfigPassExpiresAt == nil {
		return false
	}
	return q.WebfigPassExpiresAt.After(time.Now())
}

func (h *VPSHandler) planDenied(c *gin.Context) {
	c.JSON(http.StatusForbidden, gin.H{"error": "vps provisioning requires an active Sovereign pass", "code": "plan_required"})
}

// HubConfig returns the mesh config a router must match (interface, listen
// port, allowed IPs, management subnet) so the operator can provision correctly.
func (h *VPSHandler) HubConfig(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !h.planActive(businessID) {
		h.planDenied(c)
		return
	}
	c.JSON(http.StatusOK, vps.BuildHubConfig(
		h.wg.Interface, h.wg.ListenPort, h.endpointHost, h.wg.AllowedIPs,
		h.wg.ManagementVLAN, h.wg.ManagementSubnet,
		int(h.wg.PeerKeepalive.Seconds()),
	))
}

// Peer returns the deterministic WireGuard client block for one device, resolved
// under the tenant's RLS scope. 404 when the device doesn't belong to the tenant.
func (h *VPSHandler) Peer(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !h.planActive(businessID) {
		h.planDenied(c)
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
		// Not found in THIS tenant (business mismatch or absent). Returning 404
		// for both keeps cross-tenant device identity from leaking — important
		// because GetDeviceByIDTXN is scoped by device_id only, so ownership
		// must be confirmed here regardless of the RLS flag state.
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found in your tenant"})
		return
	}

	p, err := vps.BuildPeer(
		device.DeviceID, device.HardwareName, device.DeploymentLayer, h.wg.Interface,
		h.wg.ManagementSubnet, h.endpointHost, h.wg.ListenPort,
		int(h.wg.PeerKeepalive.Seconds()),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build peer config: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, p)
}

// Health classifies the device tunnel's reachability (up/degraded/unreachable)
// using the monitoring state. Owner- + plan-gated.
func (h *VPSHandler) Health(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !h.planActive(businessID) {
		h.planDenied(c)
		return
	}
	deviceID := c.Param("deviceId")

	state, err := h.monitoring.StateForPeer(c.Request.Context(), businessID, deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to classify peer health: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"device_id": deviceID,
		"state":     vps.Classify(state),
	})
}
