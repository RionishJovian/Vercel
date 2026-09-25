package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/notifier"
)

type IncidentHandler struct {
	db           *database.DB
	incidentRepo *database.IncidentRepository
	policyRepo   *database.AlertPolicyRepository
	deviceRepo   *database.DeviceRepository
	notifier     *notifier.Notifier
}

func NewIncidentHandler(
	db *database.DB,
	incidentRepo *database.IncidentRepository,
	policyRepo *database.AlertPolicyRepository,
	deviceRepo *database.DeviceRepository,
	notifier *notifier.Notifier,
) *IncidentHandler {
	return &IncidentHandler{db: db, incidentRepo: incidentRepo, policyRepo: policyRepo, deviceRepo: deviceRepo, notifier: notifier}
}

// withTenantTx runs fn in a transaction with the tenant RLS context set via
// database.WithTenant, committing on success and rolling back on error. Mirrors
// the money-path helper so every operational-alerting query is tenant-scoped.
func (h *IncidentHandler) withTenantTx(businessID string, fn func(tx *sql.Tx) error) error {
	tx, err := h.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := database.WithTenant(tx, businessID, func() error { return fn(tx) }); err != nil {
		return err
	}
	return tx.Commit()
}

// respondErr renders an error produced inside a tenant tx, honouring a carried
// httpError (exact status + message) and falling back to 500 otherwise.
func respondErr(c *gin.Context, err error) {
	var he *httpError
	if errors.As(err, &he) {
		c.JSON(he.status, gin.H{"error": he.msg})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	var incidents []model.Incident
	err := h.withTenantTx(businessID, func(tx *sql.Tx) error {
		xs, err := h.incidentRepo.ListOpenByBusinessTXN(tx, businessID)
		if err != nil {
			return err
		}
		incidents = xs
		return nil
	})
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
	var incidents []model.Incident
	err := h.withTenantTx(businessID, func(tx *sql.Tx) error {
		xs, err := h.incidentRepo.ListByDeviceTXN(tx, businessID, deviceID)
		if err != nil {
			return err
		}
		incidents = xs
		return nil
	})
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

	var created *model.Incident
	err := h.withTenantTx(businessID, func(tx *sql.Tx) error {
		cr, err := h.incidentRepo.CreateTXN(tx, inc)
		if err != nil {
			return err
		}
		created = cr
		return nil
	})
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
	err := h.withTenantTx(businessID, func(tx *sql.Tx) error {
		inc, err := h.incidentRepo.GetByIDTXN(tx, id)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if inc == nil {
			return &httpError{status: http.StatusNotFound, msg: "incident not found"}
		}
		if inc.BusinessID != businessID {
			return &httpError{status: http.StatusForbidden, msg: "access denied"}
		}
		if err := h.incidentRepo.AcknowledgeTXN(tx, id, userID); err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: err.Error()}
		}
		return nil
	})
	if err != nil {
		respondErr(c, err)
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
	var resolved *model.Incident
	err := h.withTenantTx(businessID, func(tx *sql.Tx) error {
		inc, err := h.incidentRepo.GetByIDTXN(tx, id)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if inc == nil {
			return &httpError{status: http.StatusNotFound, msg: "incident not found"}
		}
		if inc.BusinessID != businessID {
			return &httpError{status: http.StatusForbidden, msg: "access denied"}
		}
		if err := h.incidentRepo.ResolveTXN(tx, id, userID); err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: err.Error()}
		}
		resolved = inc
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}

	if resolved != nil {
		now := time.Now().UTC()
		resolved.ResolvedAt = &now
		if h.notifier != nil {
			h.notifier.IncidentResolved(businessID, resolved)
		}
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

	var policies []*model.AlertPolicy
	err := h.withTenantTx(businessID, func(tx *sql.Tx) error {
		ps, err := h.policyRepo.ListByBusinessTXN(tx, businessID)
		if err != nil {
			return err
		}
		policies = ps
		return nil
	})
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

	var created *model.AlertPolicy
	err := h.withTenantTx(businessID, func(tx *sql.Tx) error {
		cr, err := h.policyRepo.CreateTXN(tx, policy)
		if err != nil {
			return err
		}
		created = cr
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create policy: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"policy": created})
}

type UpdatePolicyRequest struct {
	Kind                 *string  `json:"kind"`
	Enabled              *bool    `json:"enabled"`
	Severity             *string  `json:"severity"`
	Threshold            *float64 `json:"threshold"`
	CooldownMinutes      *int     `json:"cooldown_minutes"`
	NotificationChannels []string `json:"notification_channels"`
}

// UpdatePolicy updates an existing alert policy.
func (h *IncidentHandler) UpdatePolicy(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	var updated *model.AlertPolicy
	err := h.withTenantTx(businessID, func(tx *sql.Tx) error {
		existing, err := h.policyRepo.GetByIDTXN(tx, id)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if existing == nil {
			return &httpError{status: http.StatusNotFound, msg: "policy not found"}
		}
		if existing.BusinessID != businessID {
			return &httpError{status: http.StatusForbidden, msg: "access denied"}
		}

		var req UpdatePolicyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			return &httpError{status: http.StatusBadRequest, msg: err.Error()}
		}

		if req.Kind != nil {
			existing.Kind = *req.Kind
		}
		if req.Enabled != nil {
			existing.Enabled = *req.Enabled
		}
		if req.Severity != nil {
			if *req.Severity != "info" && *req.Severity != "warning" && *req.Severity != "critical" {
				return &httpError{status: http.StatusBadRequest, msg: "invalid severity: expected info, warning or critical"}
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

		if err := h.policyRepo.UpdateTXN(tx, existing); err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "failed to update policy: " + err.Error()}
		}
		updated = existing
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"policy": updated})
}

// DeletePolicy removes an alert policy.
func (h *IncidentHandler) DeletePolicy(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	err := h.withTenantTx(businessID, func(tx *sql.Tx) error {
		existing, err := h.policyRepo.GetByIDTXN(tx, id)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if existing == nil {
			return &httpError{status: http.StatusNotFound, msg: "policy not found"}
		}
		if existing.BusinessID != businessID {
			return &httpError{status: http.StatusForbidden, msg: "access denied"}
		}
		if err := h.policyRepo.DeleteTXN(tx, id); err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: err.Error()}
		}
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "policy_id": id})
}
