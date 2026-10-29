package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/cache"
	"mega-magic/internal/database"
	"mega-magic/internal/messaging"
	"mega-magic/internal/model"
)

type TelemetryHandler struct {
	db            *database.DB
	telemetryRepo *database.TelemetrySampleRepository
	cache         *cache.Client
	msg           *messaging.Client
}

func NewTelemetryHandler(db *database.DB, tr *database.TelemetrySampleRepository, c *cache.Client, msg *messaging.Client) *TelemetryHandler {
	return &TelemetryHandler{db: db, telemetryRepo: tr, cache: c, msg: msg}
}

// withTenantTx runs fn in a transaction with the tenant RLS context set via
// database.WithTenant, committing on success and rolling back on error. Mirrors
// the money-path/alerting helpers so telemetry queries are tenant-scoped.
func (h *TelemetryHandler) withTenantTx(c *gin.Context, businessID string, fn func(tx *sql.Tx) error) error {
	return runInTenant(c, h.db, businessID, fn)
}

// Ingest retires the browser-writable legacy telemetry endpoint. Router readings
// are collected server-side through the authenticated, tenant-scoped RouterOS
// reader; client-supplied measurements must never feed incident evaluation.
func (h *TelemetryHandler) Ingest(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusGone, gin.H{
		"error": "Client-supplied telemetry is no longer accepted; use the read-only router telemetry endpoint",
		"code":  "telemetry_ingest_retired",
	})
}

// List returns telemetry samples for a device or the whole business.
func (h *TelemetryHandler) List(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	deviceID := c.Query("device_id")
	days := 1
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 30 {
			days = n
		}
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

	var samples []*model.TelemetrySample
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		var e error
		if deviceID != "" {
			samples, e = h.telemetryRepo.ListByDeviceTXN(tx, deviceID, &since, nil)
		} else {
			samples, e = h.telemetryRepo.ListByBusinessTXN(tx, businessID, &since, nil)
		}
		return e
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list telemetry: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"samples": samples, "count": len(samples), "days": days})
}

// Latest returns the cached latest sample for a device, falling back to the DB.
func (h *TelemetryHandler) Latest(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	deviceID := c.Param("deviceId")
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device_id required"})
		return
	}

	var s model.TelemetrySample
	cacheKey := h.cache.Key(businessID, "telemetry:latest:"+deviceID)
	if err := h.cache.GetJSON(c.Request.Context(), cacheKey, &s); err == nil {
		c.JSON(http.StatusOK, gin.H{"sample": &s, "source": "cache"})
		return
	}

	// DB fallback and re-warm the cache.
	var sample *model.TelemetrySample
	err := h.withTenantTx(c, businessID, func(tx *sql.Tx) error {
		s, e := h.telemetryRepo.GetLatestByDeviceTXN(tx, deviceID)
		if e != nil {
			return e
		}
		sample = s
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read telemetry: " + err.Error()})
		return
	}
	if sample == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no telemetry for device"})
		return
	}
	_ = h.cache.SetJSON(c.Request.Context(), cacheKey, sample, 5*time.Minute)

	c.JSON(http.StatusOK, gin.H{"sample": sample, "source": "db"})
}
