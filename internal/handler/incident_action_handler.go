package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/messaging"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner"
)

// IncidentActionHandler applies an incident's suggested_action (Stage 1A).
//
// Fail-closed: the live applier is attached only when the live path is enabled
// (EnableLive). Actions that need a live write are refused while it is absent.
// Every incident read is tenant-scoped (business_id), never a bare-id read.
type IncidentActionHandler struct {
	db        *database.DB
	incidents *database.IncidentRepository
	devices   *database.DeviceRepository
	msg       *messaging.Client
	applier   provisioner.Applier
}

func NewIncidentActionHandler(db *database.DB, incidents *database.IncidentRepository, devices *database.DeviceRepository, msg *messaging.Client) *IncidentActionHandler {
	return &IncidentActionHandler{db: db, incidents: incidents, devices: devices, msg: msg}
}

// EnableLive attaches the live applier (called from the flag-gated wiring site).
func (h *IncidentActionHandler) EnableLive(a provisioner.Applier) { h.applier = a }

func newIntentID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "intent"
	}
	return hex.EncodeToString(b)
}

// Apply: POST /api/v1/incidents/:id/action
func (h *IncidentActionHandler) Apply(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	incidentID := c.Param("id")

	var inc *model.Incident
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		got, e := h.incidents.GetByIDTenantTXN(tx, businessID, incidentID)
		if e != nil {
			return e
		}
		inc = got
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if inc == nil {
		// 404 for absent AND cross-tenant keeps identity from leaking.
		c.JSON(http.StatusNotFound, gin.H{"error": "incident not found in your tenant"})
		return
	}

	sa := inc.SuggestedAction
	if sa == nil || sa.Kind == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "this incident has no suggested action"})
		return
	}
	switch sa.Kind {
	case "none", "review":
		c.JSON(http.StatusBadRequest, gin.H{"error": "this incident requires manual review — no automatic fix", "kind": sa.Kind})
		return
	}

	deviceID := ""
	if inc.DeviceID != nil {
		deviceID = *inc.DeviceID
	}

	switch sa.Kind {
	case "reboot":
		if h.msg == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provisioning bus unavailable (fail closed)"})
			return
		}
		intentID := newIntentID()
		data, jerr := json.Marshal(map[string]string{
			"business_id": businessID,
			"device_id":   deviceID,
			"action":      provisioner.ActionReboot,
			"intent_id":   intentID,
		})
		if jerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode reboot intent"})
			return
		}
		// Reuse the SAME intent subject the topology reboot path publishes.
		if perr := h.msg.Publish(businessID, provisioner.SubjectTopologyAction, data); perr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue reboot intent"})
			return
		}
		h.record(c, businessID, deviceID, "incident.action", "apply suggested reboot for incident "+incidentID)
		c.JSON(http.StatusAccepted, gin.H{
			"status": "reboot intent queued for approval", "action": provisioner.ActionReboot,
			"intent_id": intentID, "incident_id": incidentID,
		})

	case "restore_backup", "reprovision_peer":
		if h.applier == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "live applier disabled — action unavailable (fail closed)"})
			return
		}
		// The applier is enabled but there is no repo wiring for these two yet:
		// refuse rather than pretend. (No fabricated success.)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "action not wired for live execution yet", "kind": sa.Kind})
		h.record(c, businessID, deviceID, "incident.action", "attempted "+sa.Kind+" for incident "+incidentID+" (not wired)")

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown suggested action", "kind": sa.Kind})
	}
}

// record writes a device_change_event (best-effort, tenant-scoped).
func (h *IncidentActionHandler) record(c *gin.Context, businessID, deviceID, action, reason string) {
	if deviceID == "" {
		return
	}
	_ = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		return h.devices.RecordChangeTXN(tx, businessID, deviceID, auth.GetUserRole(c), action, reason)
	})
}
