package handler

import (
	"database/sql"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
)

var gatewaySiteUUID = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// GatewaySite uses the existing site relationship; no new inventory or tenant
// store is introduced by the workflow revamp.
func (h *VPSHandler) GatewaySite(c *gin.Context) {
	biz, id := auth.GetBusinessID(c), c.Param("deviceId")
	if !gatewaySiteUUID.MatchString(id) {
		c.JSON(400, gin.H{"error": "Invalid gateway"})
		return
	}
	if h.db == nil {
		c.JSON(503, gin.H{"error": "Site assignment is unavailable"})
		return
	}
	var requested struct {
		SiteID string `json:"site_id"`
	}
	if c.Request.Method == http.MethodPut {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
		if c.ShouldBindJSON(&requested) != nil {
			c.JSON(400, gin.H{"error": "Select a site"})
			return
		}
		if !gatewaySiteUUID.MatchString(requested.SiteID) {
			c.JSON(400, gin.H{"error": "Select a valid site"})
			return
		}
	}
	var site sql.NullString
	err := runInTenant(c, h.db, biz, func(tx *sql.Tx) error {
		if err := tx.QueryRow(`SELECT site_id FROM devices WHERE business_id=$1 AND device_id=$2 AND deployment_layer='MAIN_GATEWAY' FOR UPDATE`, biz, id).Scan(&site); err != nil {
			return err
		}
		if c.Request.Method == http.MethodPut {
			var found string
			if err := tx.QueryRow(`SELECT site_id FROM sites WHERE business_id=$1 AND site_id=$2 FOR KEY SHARE`, biz, requested.SiteID).Scan(&found); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE devices SET site_id=$1,updated_at=now() WHERE business_id=$2 AND device_id=$3`, found, biz, id); err != nil {
				return err
			}
			site = sql.NullString{String: found, Valid: true}
		}
		return nil
	})
	if err == sql.ErrNoRows {
		c.JSON(404, gin.H{"error": "Gateway or site not found in your business"})
		return
	}
	if err != nil {
		c.JSON(503, gin.H{"error": "Site assignment is unavailable"})
		return
	}
	value := any(nil)
	if site.Valid {
		value = site.String
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, gin.H{"site_id": value, "assigned": site.Valid})
}

// RequireGatewaySite is registered before any endpoint can allocate a Hub peer
// or issue a script. Missing, foreign and deleted sites never pass this gate.
func (h *VPSHandler) RequireGatewaySite(c *gin.Context) {
	biz, id := auth.GetBusinessID(c), c.Param("deviceId")
	if !gatewaySiteUUID.MatchString(id) {
		c.AbortWithStatusJSON(400, gin.H{"error": "Invalid gateway"})
		return
	}
	if h.db == nil {
		c.AbortWithStatusJSON(503, gin.H{"error": "Site readiness is unavailable"})
		return
	}
	var exists, ready bool
	err := runInTenant(c, h.db, biz, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM devices WHERE business_id=$1 AND device_id=$2 AND deployment_layer='MAIN_GATEWAY'), EXISTS(SELECT 1 FROM devices d JOIN sites s ON s.site_id=d.site_id AND s.business_id=d.business_id WHERE d.business_id=$1 AND d.device_id=$2 AND d.deployment_layer='MAIN_GATEWAY')`, biz, id).Scan(&exists, &ready)
	})
	if err != nil {
		c.AbortWithStatusJSON(503, gin.H{"error": "Site readiness could not be checked"})
		return
	}
	if !exists {
		c.AbortWithStatusJSON(404, gin.H{"error": "Gateway not found in your business"})
		return
	}
	if !ready {
		c.AbortWithStatusJSON(409, gin.H{"error": "Add a site and assign this gateway before connecting Magic Hub", "code": "site_required"})
		return
	}
	c.Next()
}
// RequireGatewayCapacity guards only the endpoints that start a NEW Hub
// allocation (bootstrap, enroll, quick-enroll). The gateway must fit the
// account's gateway capacity, counted in registration order. Finishing or
// re-checking an existing connection is not gated here, so a later change in
// capacity never disconnects or strands a gateway that is already set up.
func (h *VPSHandler) RequireGatewayCapacity(c *gin.Context) {
	biz, id := auth.GetBusinessID(c), c.Param("deviceId")
	if h.db == nil {
		c.AbortWithStatusJSON(503, gin.H{"error": "Gateway capacity could not be checked"})
		return
	}
	var allowed bool
	err := runInTenant(c, h.db, biz, func(tx *sql.Tx) error {
		var e error
		allowed, e = database.GatewayEnrollmentAllowedTXN(tx, biz, id)
		return e
	})
	if err != nil {
		c.AbortWithStatusJSON(503, gin.H{"error": "Gateway capacity could not be checked"})
		return
	}
	if !allowed {
		c.AbortWithStatusJSON(409, gin.H{"error": "This gateway is beyond the account's gateway slots. Buy another gateway slot in the Store before connecting it.", "code": "gateway_capacity_reached", "resource": "gateway"})
		return
	}
	c.Next()
}
