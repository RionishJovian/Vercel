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

// RouterConnectionHandler serves saved router management connections (ported
// from Mikromagic) with their site binding and cloud status.
type RouterConnectionHandler struct {
	db       *database.DB
	connRepo *database.RouterConnectionRepository
}

// NewRouterConnectionHandler wires the router connection repository.
func NewRouterConnectionHandler(db *database.DB, connRepo *database.RouterConnectionRepository) *RouterConnectionHandler {
	return &RouterConnectionHandler{db: db, connRepo: connRepo}
}

// ListRouterConnections returns every connection in the caller's business.
func (h *RouterConnectionHandler) ListRouterConnections(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var conns []*model.RouterConnection
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		list, err := h.connRepo.ListRouterConnectionsTXN(tx, businessID)
		if err != nil {
			return err
		}
		conns = list
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"connections": conns, "count": len(conns)})
}

// CreateRouterConnection adds a connection (ADMIN+ — enforced at the route).
func (h *RouterConnectionHandler) CreateRouterConnection(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req model.RouterConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var conn *model.RouterConnection
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		created, err := h.connRepo.CreateRouterConnectionTXN(tx, businessID, &req)
		if err != nil {
			return err
		}
		conn = created
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"connection": conn})
}

// UpdateRouterConnection edits a connection's config (ADMIN+).
func (h *RouterConnectionHandler) UpdateRouterConnection(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("id")
	if !isUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
		return
	}

	var req model.RouterConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var conn *model.RouterConnection
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		updated, err := h.connRepo.UpdateRouterConnectionTXN(tx, businessID, id, &req)
		if err != nil {
			return err
		}
		conn = updated
		return nil
	})
	if err != nil {
		if errors.Is(err, database.ErrRouterConnectionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"connection": conn})
}

// DeleteRouterConnection removes a connection (ADMIN+). Devices, daily closes and
// device allowances referencing it are detached, not deleted.
func (h *RouterConnectionHandler) DeleteRouterConnection(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	id := c.Param("id")
	if !isUUID(id) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection id"})
		return
	}

	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		return h.connRepo.DeleteRouterConnectionTXN(tx, businessID, id)
	})
	if err != nil {
		if errors.Is(err, database.ErrRouterConnectionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true})
}
