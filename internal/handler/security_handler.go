package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/provisioner"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/security"
)

type routerCapabilityProbe interface {
	InspectCapabilities(context.Context, string, string) (routeros.RouterCapabilities, error)
}

type routerQuickConfigurator interface {
	ReadQuickConfig(context.Context, string, string) (routeros.QuickConfig, error)
	ApplyQuickConfig(context.Context, string, string, routeros.QuickConfig) (routeros.QuickConfigResult, error)
}

type webFigConfigurator interface {
	ConfigureSecureWebFig(context.Context, string, string, []string) (routeros.WebFigResult, error)
}

// ConfigureWebFig: POST /api/v1/routers/:id/webfig/secure
// ADMIN+ is enforced by the route. The device lookup and RouterOS target are
// both tenant-scoped; the trusted networks come from server configuration, not
// from browser-controlled input.
func (h *SecurityHandler) ConfigureWebFig(c *gin.Context) {
	businessID, deviceID := auth.GetBusinessID(c), c.Param("id")
	configurator, ok := h.quickConfig.(webFigConfigurator)
	if !ok || len(h.trustedCIDRs) == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "secure WebFig configuration is unavailable"})
		return
	}
	if !h.deviceExists(c, businessID, deviceID) {
		return
	}
	result, err := configurator.ConfigureSecureWebFig(c.Request.Context(), businessID, deviceID, h.trustedCIDRs)
	action, reason := "router.webfig.secured", "HTTPS WebFig enabled on trusted management networks and plaintext WebFig disabled"
	if err != nil {
		action, reason = "router.webfig.secure.failed", err.Error()
	}
	_ = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		return h.deviceRepo.RecordChangeTXN(tx, businessID, deviceID, auth.GetUserRole(c), action, reason)
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "result": result})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

// SecurityHandler serves the tenant-scoped security-posture audit (C1): every
// device's RouterOS version and safe|affected|unknown classification. All reads
// run inside a tenant-bound tx (WithTenant), mirroring the other protected
// handlers — a device from another business is invisible (no existence leak).
//
// 🔑 THE VERDICT IS RECOMPUTED AT READ TIME from the stored VERSION, not read
// back from the stored `security_posture` column. The version is the durable
// fact; the posture is a DERIVED opinion that depends on the advisory table.
// Serving the stored column would mean an advisory-table update could never
// reach any device already captured — the screen would keep showing the verdict
// the code held on the day it was written. The stored value is still returned as
// `posture_stored` so drift between the two is visible rather than hidden.
type SecurityHandler struct {
	db              *database.DB
	deviceRepo      *database.DeviceRepository
	apply           *provisioner.ApplyService
	prober          provisioner.Prober
	capabilities    routerCapabilityProbe
	quickConfig     routerQuickConfigurator
	managementMoves managementMigrationController
	trustedCIDRs    []string
}

func NewSecurityHandler(db *database.DB, deviceRepo *database.DeviceRepository) *SecurityHandler {
	return &SecurityHandler{db: db, deviceRepo: deviceRepo}
}

// EnableLive attaches the real per-device RouterOS control path. Until this is
// called, action endpoints fail closed with 503 and the UI reports that control
// is unavailable rather than presenting a decorative success button.
func (h *SecurityHandler) EnableLive(apply *provisioner.ApplyService, configurator routerQuickConfigurator, prober provisioner.Prober, trustedCIDRs []string) {
	h.apply = apply
	h.prober = prober
	h.quickConfig = configurator
	if capabilityProbe, ok := prober.(routerCapabilityProbe); ok {
		h.capabilities = capabilityProbe
	}
	h.trustedCIDRs = append([]string(nil), trustedCIDRs...)
}

// postureView renders one device's live posture summary.
func postureView(p *database.DevicePosture) gin.H {
	version := ""
	if p.Version != nil {
		version = *p.Version
	}
	channel := ""
	if p.Channel != nil {
		channel = *p.Channel
	}
	sum := security.SummarizeForChannel(version, channel, security.AdvisoryTable)

	advs := make([]gin.H, 0, len(sum.Advisories))
	for _, a := range sum.Advisories {
		advs = append(advs, gin.H{
			"id":              a.ID,
			"title":           a.Title,
			"severity":        a.Severity,
			"exploited":       a.Exploited,
			"fix_unavailable": a.FixUnavailable,
			"note":            a.Note,
			"source":          a.Source,
		})
	}

	return gin.H{
		"device_id":          p.DeviceID,
		"hardware_name":      p.HardwareName,
		"routeros_version":   p.Version,
		"routeros_channel":   p.Channel,
		"version_checked_at": p.CheckedAt,
		"posture":            sum.Posture, // live, derived from the version
		"posture_stored":     p.Posture,   // what capture-time code recorded
		"advisories":         advs,
		"advisory_count":     len(advs),
		"unpatchable":        sum.Unpatchable, // no vendor fix exists for at least one match
		"exploited":          sum.Exploited,   // at least one match is exploited in the wild
		"fix_first":          sum.FixFirst,    // lowest release clearing the FIXABLE matches
	}
}

// FleetPosture: GET /api/v1/security/posture
func (h *SecurityHandler) FleetPosture(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var rows []*database.DevicePosture
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		rs, e := h.deviceRepo.ListPostureTXN(tx, businessID)
		if e != nil {
			return e
		}
		rows = rs
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	var safe, affected, unknown, unpatchable, exploited int
	devices := make([]gin.H, 0, len(rows))
	for _, p := range rows {
		v := postureView(p)
		switch v["posture"] {
		case "safe":
			safe++
		case "affected":
			affected++
		default:
			unknown++
		}
		// Counted separately from `affected` because the remedy differs: these
		// cannot be upgraded out of, so a plain "affected" count would overstate
		// how much of the fleet a patch window can actually fix.
		if v["unpatchable"] == true {
			unpatchable++
		}
		if v["exploited"] == true {
			exploited++
		}
		devices = append(devices, v)
	}

	c.JSON(http.StatusOK, gin.H{
		"safe":             safe,
		"affected":         affected,
		"unknown":          unknown,
		"unpatchable":      unpatchable,
		"exploited":        exploited,
		"count":            len(devices),
		"devices":          devices,
		"controls_enabled": h.apply != nil && h.prober != nil && len(h.trustedCIDRs) > 0,
	})
}

// VerifyDevice: POST /api/v1/devices/:id/security/verify
// Performs a real, read-only observation over the device's pinned SSH target.
func (h *SecurityHandler) VerifyDevice(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	deviceID := c.Param("id")
	if h.prober == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "live RouterOS control is disabled"})
		return
	}
	if !h.deviceExists(c, businessID, deviceID) {
		return
	}
	obs, conn, err := h.prober.Observe(c.Request.Context(), businessID, deviceID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "router verification failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"device_id":            deviceID,
		"state":                obs.State,
		"connectivity_checked": conn.Checked,
		"reachable":            conn.Reachable,
		"detail":               conn.Detail,
	})
}

// RouterCapabilities: GET /api/v1/routers/:id/capabilities
// Reads RouterOS menus from the exact tenant/device target. Feature eligibility
// never depends on a hard-coded model list: model and board names are returned
// only as descriptive facts.
func (h *SecurityHandler) RouterCapabilities(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	deviceID := c.Param("id")
	if h.capabilities == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "live RouterOS capability discovery is disabled"})
		return
	}
	if !h.deviceExists(c, businessID, deviceID) {
		return
	}
	caps, err := h.capabilities.InspectCapabilities(c.Request.Context(), businessID, deviceID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "RouterOS capability discovery failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"capabilities": caps})
}

// ReadQuickConfig: GET /api/v1/routers/:id/quick-config
func (h *SecurityHandler) ReadQuickConfig(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	deviceID := c.Param("id")
	if h.quickConfig == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "live RouterOS configuration is disabled"})
		return
	}
	if !h.deviceExists(c, businessID, deviceID) {
		return
	}
	config, err := h.quickConfig.ReadQuickConfig(c.Request.Context(), businessID, deviceID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read RouterOS quick config: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": config})
}

// ApplyQuickConfig: PUT /api/v1/routers/:id/quick-config
// ADMIN+ is enforced at the route. The configured device is still resolved by
// business ID, backed up, verified and rolled back inside DeviceApplier.
func (h *SecurityHandler) ApplyQuickConfig(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	deviceID := c.Param("id")
	if h.quickConfig == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "live RouterOS configuration is disabled"})
		return
	}
	var request routeros.QuickConfig
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid quick config: " + err.Error()})
		return
	}
	normalized, err := routeros.ValidateQuickConfig(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.deviceExists(c, businessID, deviceID) {
		return
	}
	result, err := h.quickConfig.ApplyQuickConfig(c.Request.Context(), businessID, deviceID, normalized)
	action := "router.quick_config"
	reason := "updated and verified router identity and DNS settings"
	if err != nil {
		action = "router.quick_config.failed"
		reason = err.Error()
		if result.RolledBack {
			reason += "; targeted rollback completed"
		}
	}
	_ = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		return h.deviceRepo.RecordChangeTXN(tx, businessID, deviceID, auth.GetUserRole(c), action, reason)
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "backup": result.Backup, "rolled_back": result.RolledBack})
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}

// HardenManagement: POST /api/v1/devices/:id/security/harden
// Snapshots first, applies only the explicit service policy, verifies the
// management tunnel, and automatically rolls back on a failed verification.
func (h *SecurityHandler) HardenManagement(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	deviceID := c.Param("id")
	if h.apply == nil || len(h.trustedCIDRs) == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "security controls are not configured (live applier and management CIDRs are required)"})
		return
	}
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.ToUpper(strings.TrimSpace(body.Confirm)) != "HARDEN" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type HARDEN to confirm this RouterOS change"})
		return
	}
	if !h.deviceExists(c, businessID, deviceID) {
		return
	}
	proposal, _ := json.Marshal(gin.H{"trusted_cidrs": h.trustedCIDRs})
	change := provisioner.Change{
		Action:   provisioner.ActionHardenManagement,
		Target:   deviceID,
		Current:  "legacy/public management services may be exposed",
		Proposed: string(proposal),
		Reason:   "disable telnet, ftp, HTTP and RouterOS APIs; restrict SSH and Winbox to management networks",
		Risk:     "high",
	}
	result, err := h.apply.ApplyFixes(c.Request.Context(), provisioner.ApplyRequest{
		BusinessID: businessID,
		PeerID:     deviceID,
		Changes:    []provisioner.Change{change},
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "security hardening failed before apply: " + err.Error()})
		return
	}
	action := "security.harden.failed"
	reason := result.VerificationDetail
	if result.Verified {
		action = "security.harden"
		reason = "management services hardened and WireGuard reachability verified"
	} else if result.RolledBack {
		reason += "; automatic rollback: " + result.RollbackDetail
	}
	_ = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		return h.deviceRepo.RecordChangeTXN(tx, businessID, deviceID, auth.GetUserRole(c), action, reason)
	})
	status := http.StatusOK
	if !result.Verified {
		status = http.StatusConflict
	}
	c.JSON(status, result)
}

func (h *SecurityHandler) deviceExists(c *gin.Context, businessID, deviceID string) bool {
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(deviceID) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	var exists bool
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		device, e := h.deviceRepo.GetPostureTXN(tx, businessID, deviceID)
		if e != nil {
			return e
		}
		exists = device != nil
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return false
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return false
	}
	return true
}

// DeviceSecurity: GET /api/v1/devices/:id/security
func (h *SecurityHandler) DeviceSecurity(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	deviceID := c.Param("id")

	var p *database.DevicePosture
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		got, e := h.deviceRepo.GetPostureTXN(tx, businessID, deviceID)
		if e != nil {
			return e
		}
		p = got
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	c.JSON(http.StatusOK, postureView(p))
}
