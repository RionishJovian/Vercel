package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/security"
)

// SecurityHandler serves the tenant-scoped security-posture audit (C1): every
// device's RouterOS version and safe|affected|unknown classification. All reads
// run inside a tenant-bound tx (WithTenant), mirroring the other protected
// handlers — a device from another business is invisible (no existence leak).
//
// 🔑 THE VERDICT IS RECOMPUTED AT READ TIME from the stored VERSION, not read
// back from the stored `security_posture` column. The version is the durable
// fact; the posture is a DERIVED opinion that depends on the advisory table.
// Serving the stored column would mean an advisory-table update could never
// reach any device already captured — the screen would keep showing the verdict
// the code held on the day it was written. The stored value is still returned as
// `posture_stored` so drift between the two is visible rather than hidden.
type SecurityHandler struct {
	db         *database.DB
	deviceRepo *database.DeviceRepository
}

func NewSecurityHandler(db *database.DB, deviceRepo *database.DeviceRepository) *SecurityHandler {
	return &SecurityHandler{db: db, deviceRepo: deviceRepo}
}

// postureView renders one device's live posture summary.
func postureView(p *database.DevicePosture) gin.H {
	version := ""
	if p.Version != nil {
		version = *p.Version
	}
	// NOTE: p.Channel is deliberately NOT passed to Summarize. The classifier
	// has no channel-specific rules yet, and inventing one here would be a guess.
	// It is still returned to the client as routeros_channel.
	sum := security.Summarize(version, security.AdvisoryTable)

	advs := make([]gin.H, 0, len(sum.Advisories))
	for _, a := range sum.Advisories {
		advs = append(advs, gin.H{
			"id":              a.ID,
			"title":           a.Title,
			"severity":        a.Severity,
			"exploited":       a.Exploited,
			"fix_unavailable": a.FixUnavailable,
			"note":            a.Note,
			"source":          a.Source,
		})
	}

	return gin.H{
		"device_id":          p.DeviceID,
		"hardware_name":      p.HardwareName,
		"routeros_version":   p.Version,
		"routeros_channel":   p.Channel,
		"version_checked_at": p.CheckedAt,
		"posture":            sum.Posture,     // live, derived from the version
		"posture_stored":     p.Posture,       // what capture-time code recorded
		"advisories":         advs,
		"advisory_count":     len(advs),
		"unpatchable":        sum.Unpatchable, // no vendor fix exists for at least one match
		"exploited":          sum.Exploited,   // at least one match is exploited in the wild
		"fix_first":          sum.FixFirst,    // lowest release clearing the FIXABLE matches
	}
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

	var safe, affected, unknown, unpatchable, exploited int
	devices := make([]gin.H, 0, len(rows))
	for _, p := range rows {
		v := postureView(p)
		switch v["posture"] {
		case "safe":
			safe++
		case "affected":
			affected++
		default:
			unknown++
		}
		// Counted separately from `affected` because the remedy differs: these
		// cannot be upgraded out of, so a plain "affected" count would overstate
		// how much of the fleet a patch window can actually fix.
		if v["unpatchable"] == true {
			unpatchable++
		}
		if v["exploited"] == true {
			exploited++
		}
		devices = append(devices, v)
	}

	c.JSON(http.StatusOK, gin.H{
		"safe":        safe,
		"affected":    affected,
		"unknown":     unknown,
		"unpatchable": unpatchable,
		"exploited":   exploited,
		"count":       len(devices),
		"devices":     devices,
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

	c.JSON(http.StatusOK, postureView(p))
}
