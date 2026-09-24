package handler

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/notifier"
)

type IncidentHandler struct {
	incidentRepo *database.IncidentRepository
	policyRepo   *database.AlertPolicyRepository
	deviceRepo   *database.DeviceRepository
	notifier     *notifier.Notifier
}

func NewIncidentHandler(
	incidentRepo *database.IncidentRepository,
	policyRepo *database.AlertPolicyRepository,
	deviceRepo *database.DeviceRepository,
	notifier *notifier.Notifier,
) *IncidentHandler {
	return &IncidentHandler{incidentRepo: incidentRepo, policyRepo: policyRepo, deviceRepo: deviceRepo, notifier: notifier}
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ListOpen returns all open (unresolved) incidents for the tenant.
func (h *IncidentHandler) ListOpen(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	incidents, err := h.incidentRepo.ListOpenByBusiness(businessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list incidents: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"incidents": incidents, "count": len(incidents)})
}

// ListByDevice returns incidents for one device, scoped to the tenant.
func (h *IncidentHandler) ListByDevice(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	deviceID := c.Param("deviceId")
	incidents, err := h.incidentRepo.ListByDevice(businessID, deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list device incidents: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"incidents": incidents, "count": len(incidents)})
}

type CreateIncidentRequest struct {
	DeviceID     *string `json:"device_id"`
	Kind         string  `json:"kind" binding:"required"`
	Severity     string  `json:"severity" binding:"required"`
	SubjectID    string  `json:"subject_id" binding:"required"`
	SubjectLabel string  `json:"subject_label" binding:"required"`
	Detail       *string `json:"detail"`
}

// Create opens a new incident for the tenant.
func (h *IncidentHandler) Create(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req CreateIncidentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Severity != "info" && req.Severity != "warning" && req.Severity != "critical" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid severity: expected info, warning or critical"})
		return
	}

	now := time.Now().UTC()
	inc := &model.Incident{
		ID:           newID(),
		BusinessID:   businessID,
		DeviceID:     req.DeviceID,
		Kind:         req.Kind,
		Severity:     req.Severity,
		SubjectID:    req.SubjectID,
		SubjectLabel: req.SubjectLabel,
		Detail:       req.Detail,
		OpenedAt:     now,
	}

	created, err := h.incidentRepo.Create(inc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create incident: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"incident": created})
}

// Acknowledge marks an incident acknowledged (user from JWT).
func (h *IncidentHandler) Acknowledge(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID := auth.GetUserID(c)

	id := c.Param("id")
	inc, err := h.incidentRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if inc == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "incident not found"})
		return
	}
	if inc.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := h.incidentRepo.Acknowledge(id, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "acknowledged", "incident_id": id})
}

// Resolve marks an incident resolved.
func (h *IncidentHandler) Resolve(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID := auth.GetUserID(c)

	id := c.Param("id")
	inc, err := h.incidentRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if inc == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "incident not found"})
		return
	}
	if inc.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := h.incidentRepo.Resolve(id, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	now := time.Now().UTC()
	inc.ResolvedAt = &now
	if h.notifier != nil {
		h.notifier.IncidentResolved(businessID, inc)
	}
	c.JSON(http.StatusOK, gin.H{"status": "resolved", "incident_id": id})
}

// --- Alert policies ---

type CreatePolicyRequest struct {
	Kind                 string   `json:"kind" binding:"required"`
	Enabled              bool     `json:"enabled"`
	Severity             string   `json:"severity" binding:"required"`
	Threshold            float64  `json:"threshold"`
	CooldownMinutes      int      `json:"cooldown_minutes"`
	NotificationChannels []string `json:"notification_channels"`
}

// ListPolicies returns all alert policies for the tenant.
func (h *IncidentHandler) ListPolicies(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	policies, err := h.policyRepo.ListByBusiness(businessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list policies: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"policies": policies, "count": len(policies)})
}

// CreatePolicy creates a new alert policy.
func (h *IncidentHandler) CreatePolicy(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req CreatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Severity != "info" && req.Severity != "warning" && req.Severity != "critical" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid severity: expected info, warning or critical"})
		return
	}

	policy := &model.AlertPolicy{
		BusinessID:           businessID,
		Kind:                 req.Kind,
		Enabled:              req.Enabled,
		Severity:             req.Severity,
		Threshold:            req.Threshold,
		CooldownMinutes:      req.CooldownMinutes,
		NotificationChannels: req.NotificationChannels,
	}

	created, err := h.policyRepo.Create(policy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create policy: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"policy": created})
}

type UpdatePolicyRequest struct {
	Kind                 *string   `json:"kind"`
	Enabled              *bool     `json:"enabled"`
	Severity             *string   `json:"severity"`
	Threshold            *float64  `json:"threshold"`
	CooldownMinutes      *int      `json:"cooldown_minutes"`
	NotificationChannels []string  `json:"notification_channels"`
}

// UpdatePolicy updates an existing alert policy.
func (h *IncidentHandler) UpdatePolicy(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	existing, err := h.policyRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}
	if existing.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req UpdatePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Kind != nil {
		existing.Kind = *req.Kind
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.Severity != nil {
		if *req.Severity != "info" && *req.Severity != "warning" && *req.Severity != "critical" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid severity: expected info, warning or critical"})
			return
		}
		existing.Severity = *req.Severity
	}
	if req.Threshold != nil {
		existing.Threshold = *req.Threshold
	}
	if req.CooldownMinutes != nil {
		existing.CooldownMinutes = *req.CooldownMinutes
	}
	if req.NotificationChannels != nil {
		existing.NotificationChannels = req.NotificationChannels
	}

	if err := h.policyRepo.Update(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update policy: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"policy": existing})
}

// DeletePolicy removes an alert policy.
func (h *IncidentHandler) DeletePolicy(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	existing, err := h.policyRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}
	if existing.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := h.policyRepo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "policy_id": id})
}