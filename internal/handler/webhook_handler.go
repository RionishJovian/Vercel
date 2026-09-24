package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

type WebhookHandler struct {
	webhookRepo *database.WebhookRepository
}

func NewWebhookHandler(repo *database.WebhookRepository) *WebhookHandler {
	return &WebhookHandler{webhookRepo: repo}
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
	endpoints, err := h.webhookRepo.ListEndpoints(businessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

	ep := &model.WebhookEndpoint{
		BusinessID: businessID,
		URL:        req.URL,
		Secret:     req.Secret,
		EventTypes: eventTypes,
		Enabled:    enabled,
	}
	created, err := h.webhookRepo.CreateEndpoint(ep)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	existing, err := h.webhookRepo.GetEndpointByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
		return
	}
	if existing.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req UpdateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.URL != nil {
		existing.URL = *req.URL
	}
	if req.Secret != nil {
		existing.Secret = req.Secret
	}
	if req.EventTypes != nil {
		for _, ev := range req.EventTypes {
			if !validWebhookEvents[ev] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event_type: " + ev})
				return
			}
		}
		existing.EventTypes = req.EventTypes
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	if err := h.webhookRepo.UpdateEndpoint(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"webhook": existing})
}

// Delete removes a webhook endpoint.
func (h *WebhookHandler) Delete(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id := c.Param("id")
	existing, err := h.webhookRepo.GetEndpointByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
		return
	}
	if existing.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := h.webhookRepo.DeleteEndpoint(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	deliveries, err := h.webhookRepo.ListDeliveries(businessID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deliveries": deliveries, "count": len(deliveries)})
}