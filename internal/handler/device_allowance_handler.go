package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

// DeviceAllowanceHandler serves granted capacity + live usage, and the
// device-request approval workflow (both ported from Mikromagic).
type DeviceAllowanceHandler struct {
	db        *database.DB
	allowRepo *database.DeviceAllowanceRepository
}

// NewDeviceAllowanceHandler wires the device allowance repository.
func NewDeviceAllowanceHandler(db *database.DB, allowRepo *database.DeviceAllowanceRepository) *DeviceAllowanceHandler {
	return &DeviceAllowanceHandler{db: db, allowRepo: allowRepo}
}

// GetAllowance returns the business's granted capacity with live usage.
func (h *DeviceAllowanceHandler) GetAllowance(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var allowance *model.DeviceAllowance
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		a, err := h.allowRepo.GetAllowanceTXN(tx, businessID)
		if err != nil {
			return err
		}
		allowance = a
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"allowance": allowance, "kinds": model.DeviceAllowanceKinds})
}

// ListRequests returns every capacity request in the business.
func (h *DeviceAllowanceHandler) ListRequests(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var requests []*model.DeviceRequest
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		list, err := h.allowRepo.ListRequestsTXN(tx, businessID)
		if err != nil {
			return err
		}
		requests = list
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	pending := 0
	for _, r := range requests {
		if r.Pending() {
			pending++
		}
	}
	c.JSON(http.StatusOK, gin.H{"requests": requests, "count": len(requests), "pending": pending})
}

// CreateRequest opens a capacity request. Any authenticated role may ASK — it is
// the decision that is privileged, not the request.
func (h *DeviceAllowanceHandler) CreateRequest(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	userID := auth.GetUserID(c)
	if businessID == "" || userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req model.DeviceRequestCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !model.IsDeviceAllowanceKind(req.Kind) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "unknown kind",
			"reason": "kind must be one of " + joinKinds(model.DeviceAllowanceKinds),
		})
		return
	}

	var created *model.DeviceRequest
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		q, err := h.allowRepo.CreateRequestTXN(tx, businessID, userID, &req)
		if err != nil {
			return err
		}
		created = q
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"request": created})
}

// DecideRequest approves or rejects a pending request (ADMIN+ — enforced at route).
func (h *DeviceAllowanceHandler) DecideRequest(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	decidedBy := auth.GetUserID(c)
	if businessID == "" || decidedBy == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("id")
	if !isUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request id"})
		return
	}

	var req model.DeviceRequestDecision
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var decided *model.DeviceRequest
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		q, err := h.allowRepo.DecideRequestTXN(tx, businessID, id, decidedBy, req.Status)
		if err != nil {
			return err
		}
		decided = q
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, database.ErrDeviceRequestNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		case errors.Is(err, database.ErrDeviceRequestAlreadyDecided):
			// Conflict, not 400: the request is valid, its state has moved on.
			c.JSON(http.StatusConflict, gin.H{"error": "request has already been decided"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"request": decided})
}

func joinKinds(kinds []string) string {
	out := ""
	for i, k := range kinds {
		if i > 0 {
			out += ", "
		}
		out += k
	}
	return out
}
