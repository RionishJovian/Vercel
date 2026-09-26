package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
)

// SecurityHandler serves the tenant-scoped security-posture audit (C1): every
// device's RouterOS version and safe|affected|unknown classification. All reads
// run inside a tenant-bound tx (WithTenant), mirroring the other protected
// handlers — a device from another business is invisible (no existence leak).
type SecurityHandler struct {
	db         *database.DB
	deviceRepo *database.DeviceRepository
}

func NewSecurityHandler(db *database.DB, deviceRepo *database.DeviceRepository) *SecurityHandler {
	return &SecurityHandler{db: db, deviceRepo: deviceRepo}
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

	safe, affected, unknown := 0, 0, 0
	devices := make([]gin.H, 0, len(rows))
	for _, p := range rows {
		switch p.Posture {
		case "safe":
			safe++
		case "affected":
			affected++
		default:
			unknown++
		}
		devices = append(devices, gin.H{
			"device_id":          p.DeviceID,
			"hardware_name":      p.HardwareName,
			"routeros_version":   p.Version,
			"routeros_channel":   p.Channel,
			"version_checked_at": p.CheckedAt,
			"posture":            p.Posture,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"safe":     safe,
		"affected": affected,
		"unknown":  unknown,
		"count":    len(devices),
		"devices":  devices,
	})
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

	c.JSON(http.StatusOK, gin.H{
		"device_id":          p.DeviceID,
		"hardware_name":      p.HardwareName,
		"routeros_version":   p.Version,
		"routeros_channel":   p.Channel,
		"version_checked_at": p.CheckedAt,
		"posture":            p.Posture,
	})
}
