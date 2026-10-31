package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/security"
)

type WebhookHandler struct {
	db          *database.DB
	webhookRepo *database.WebhookRepository
}

func NewWebhookHandler(db *database.DB, repo *database.WebhookRepository) *WebhookHandler {
	return &WebhookHandler{db: db, webhookRepo: repo}
}

// withTenantTx runs fn in an RLS-bound tenant transaction (pinned conn when
// database.rls_enforce is on, owner tx otherwise).
func (h *WebhookHandler) withTenantTx(c *gin.Context, businessID string, fn func(tx *sql.Tx) error) error {
	return runInTenant(c, h.db, businessID, fn)
}

type CreateWebhookRequest struct {
	URL        string   `json:"url" binding:"required"`
	Secret     *string  `json:"secret"`
	EventTypes []string `json:"event_types"`
	Enabled    *bool    `json:"enabled"`
}

var validWebhookEvents = map[string]bool{
	"incident.opened":   true,
	"incident.resolved": true,
	"device.online":     true,
	"device.offline":    true,
}

// List returns all webhook endpoints for the tenant.
func (h *WebhookHandler) List(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var endpoints []*model.WebhookEndpoint
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		eps, err := h.webhookRepo.ListEndpointsTXN(tx, businessID)
		if err != nil {
			return err
		}
		endpoints = eps
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"webhooks": endpoints, "count": len(endpoints)})
}

// Create registers a new webhook endpoint for the tenant.
func (h *WebhookHandler) Create(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	eventTypes := req.EventTypes
	if len(eventTypes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "event_types is required"})
		return
	}
	for _, ev := range eventTypes {
		if !validWebhookEvents[ev] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event_type: " + ev})
			return
		}
	}

	if err := security.ValidateWebhookURL(req.URL); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook url: " + err.Error()})
		return
	}

	ep := &model.WebhookEndpoint{
		BusinessID: businessID,
		URL:        req.URL,
		Secret:     req.Secret,
		EventTypes: eventTypes,
		Enabled:    enabled,
	}

	var created *model.WebhookEndpoint
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		e, err := h.webhookRepo.CreateEndpointTXN(tx, ep)
		if err != nil {
			return err
		}
		created = e
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"webhook": created})
}

type UpdateWebhookRequest struct {
	URL        *string  `json:"url"`
	Secret     *string  `json:"secret"`
	EventTypes []string `json:"event_types"`
	Enabled    *bool    `json:"enabled"`
}

// Update modifies an existing webhook endpoint.
func (h *WebhookHandler) Update(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	var updated *model.WebhookEndpoint
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		existing, err := h.webhookRepo.GetEndpointByIDTXN(tx, id)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if existing == nil {
			return &httpError{status: http.StatusNotFound, msg: "webhook not found"}
		}
		if existing.BusinessID != businessID {
			return &httpError{status: http.StatusForbidden, msg: "access denied"}
		}

		var req UpdateWebhookRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			return &httpError{status: http.StatusBadRequest, msg: err.Error()}
		}

		if req.URL != nil {
			if err := security.ValidateWebhookURL(*req.URL); err != nil {
				return &httpError{status: http.StatusBadRequest, msg: "invalid webhook url: " + err.Error()}
			}
			existing.URL = *req.URL
		}
		if req.Secret != nil {
			existing.Secret = req.Secret
		}
		if req.EventTypes != nil {
			for _, ev := range req.EventTypes {
				if !validWebhookEvents[ev] {
					return &httpError{status: http.StatusBadRequest, msg: "invalid event_type: " + ev}
				}
			}
			existing.EventTypes = req.EventTypes
		}
		if req.Enabled != nil {
			existing.Enabled = *req.Enabled
		}

		if err := h.webhookRepo.UpdateEndpointTXN(tx, existing); err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		updated = existing
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"webhook": updated})
}

// Delete removes a webhook endpoint.
func (h *WebhookHandler) Delete(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		existing, err := h.webhookRepo.GetEndpointByIDTXN(tx, id)
		if err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		if existing == nil {
			return &httpError{status: http.StatusNotFound, msg: "webhook not found"}
		}
		if existing.BusinessID != businessID {
			return &httpError{status: http.StatusForbidden, msg: "access denied"}
		}
		if err := h.webhookRepo.DeleteEndpointTXN(tx, id); err != nil {
			return &httpError{status: http.StatusInternalServerError, msg: "database error"}
		}
		return nil
	})
	if err != nil {
		respondErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "webhook_id": id})
}

// ListDeliveries returns recent delivery attempts for the tenant.
func (h *WebhookHandler) ListDeliveries(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	var deliveries []*model.WebhookDelivery
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		ds, err := h.webhookRepo.ListDeliveriesTXN(tx, businessID, limit)
		if err != nil {
			return err
		}
		deliveries = ds
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deliveries": deliveries, "count": len(deliveries)})
}
