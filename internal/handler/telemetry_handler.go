package handler

import (
	"database/sql"
	"encoding/json"
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
func (h *TelemetryHandler) withTenantTx(businessID string, fn func(tx *sql.Tx) error) error {
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

// Ingest receives a telemetry sample (from the collector or edge agent) and stores it.
func (h *TelemetryHandler) Ingest(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	if businessID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		DeviceID       *string  `json:"device_id"`
		SubjectKind    string   `json:"subject_kind"`
		SubjectID      string   `json:"subject_id"`
		CPUPercent     *float64 `json:"cpu_pct"`
		MemoryPercent  *float64 `json:"memory_pct"`
		LatencyMs      *int     `json:"latency_ms"`
		UptimeSeconds  *int64   `json:"uptime_seconds"`
		WANState       *string  `json:"wan_state"`
		ConnectorState *string  `json:"connector_state"`
		TunnelState    *string  `json:"tunnel_state"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: " + err.Error()})
		return
	}
	if req.SubjectKind == "" || req.SubjectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "subject_kind and subject_id are required"})
		return
	}

	sample := &model.TelemetrySample{
		BusinessID:     businessID,
		DeviceID:       req.DeviceID,
		SubjectKind:    req.SubjectKind,
		SubjectID:      req.SubjectID,
		CPUPercent:     req.CPUPercent,
		MemoryPercent:  req.MemoryPercent,
		LatencyMs:      req.LatencyMs,
		UptimeSeconds:  req.UptimeSeconds,
		WANState:       req.WANState,
		ConnectorState: req.ConnectorState,
		TunnelState:    req.TunnelState,
		ObservedAt:     time.Now().UTC(),
	}
	if err := h.withTenantTx(businessID, func(tx *sql.Tx) error {
		_, err := h.telemetryRepo.InsertTXN(tx, sample)
		return err
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store telemetry: " + err.Error()})
		return
	}

	// Cache the latest sample per device for fast dashboard reads.
	if req.DeviceID != nil && *req.DeviceID != "" {
		_ = h.cache.SetJSON(c.Request.Context(), h.cache.Key(businessID, "telemetry:latest:"+*req.DeviceID), sample, 5*time.Minute)
	}

	// Fan out to NATS so the policy evaluator (and future consumers) see the
	// sample without coupling. Fire-and-forget: storage already succeeded.
	if h.msg != nil {
		env := map[string]any{
			"business_id":     businessID,
			"device_id":       req.DeviceID,
			"subject_kind":    req.SubjectKind,
			"subject_id":      req.SubjectID,
			"cpu_pct":         req.CPUPercent,
			"memory_pct":      req.MemoryPercent,
			"latency_ms":      req.LatencyMs,
			"wan_state":       req.WANState,
			"connector_state": req.ConnectorState,
			"tunnel_state":    req.TunnelState,
			"observed_at":     time.Now().UTC(),
		}
		if data, err := json.Marshal(env); err == nil {
			_ = h.msg.Publish(businessID, "telemetry.ingest", data)
		}
	}

	c.JSON(http.StatusCreated, gin.H{"status": "ok", "sample": sample})
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
	err := h.withTenantTx(businessID, func(tx *sql.Tx) error {
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
	err := h.withTenantTx(businessID, func(tx *sql.Tx) error {
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
