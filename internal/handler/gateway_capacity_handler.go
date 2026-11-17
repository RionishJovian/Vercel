package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
)

// GatewayCapacityHandler reports how many gateways a client account may register:
// the included gateway, legacy router grants, fulfilled router-slot purchases and
// platform grants, with live usage and the state of each router-slot purchase.
type GatewayCapacityHandler struct{ db *database.DB }

func NewGatewayCapacityHandler(db *database.DB) *GatewayCapacityHandler {
	return &GatewayCapacityHandler{db: db}
}

func (h *GatewayCapacityHandler) Get(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz := auth.GetBusinessID(c)
	if biz == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var out *database.GatewayCapacity
	err := runInTenant(c, h.db, biz, func(tx *sql.Tx) error {
		v, e := database.GatewayCapacityTXN(tx, biz)
		out = v
		return e
	})
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Gateway capacity is unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"capacity": out})
}

// gatewayCapacityConflict answers a refused gateway registration with the
// current numbers, so the app can offer a purchase instead of a dead end.
func gatewayCapacityConflict(c *gin.Context, db *database.DB, biz string) {
	body := gin.H{"code": "gateway_capacity_reached", "error": "All gateway slots on this account are in use. Buy another gateway slot in the Store, or remove a gateway you no longer use.", "resource": "gateway", "scope": "business"}
	if db != nil {
		_ = runInTenant(c, db, biz, func(tx *sql.Tx) error {
			if v, e := database.GatewayCapacityTXN(tx, biz); e == nil {
				body["capacity"], body["used"], body["limit"] = v.Capacity, v.Used, v.Capacity
			}
			return nil
		})
	}
	c.JSON(http.StatusConflict, body)
}
