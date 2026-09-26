package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/provisioner"
)

// BackupHandler serves C2: per-device config backups, the change-history
// timeline, and one-click restore.
//
// Fail-closed by default: the live applier + OTP verifier are attached only when
// the live path is enabled (EnableLive). Until then capture/restore return 503
// and no device is ever touched. Reads (list/timeline) are always tenant-scoped.
type BackupHandler struct {
	db      *database.DB
	devices *database.DeviceRepository

	applier provisioner.Applier       // nil => capture/restore fail closed (503)
	otp     *provisioner.TOTPVerifier // nil/empty-secret => high-risk DENIED
}

func NewBackupHandler(db *database.DB, devices *database.DeviceRepository) *BackupHandler {
	return &BackupHandler{db: db, devices: devices}
}

// EnableLive attaches the live applier + OTP verifier (called from the wiring
// site when provisioner.applier.enabled). Without it, capture/restore stay 503.
func (h *BackupHandler) EnableLive(applier provisioner.Applier, otp *provisioner.TOTPVerifier) {
	h.applier = applier
	h.otp = otp
}

// ListBackups: GET /api/v1/devices/:id/backups
func (h *BackupHandler) ListBackups(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	deviceID := c.Param("id")

	var rows []*database.DeviceConfigBackup
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		rs, e := h.devices.ListBackupsByDeviceTXN(tx, businessID, deviceID)
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

	// Metadata only (never ship the config blob in a list).
	backups := make([]gin.H, 0, len(rows))
	for _, b := range rows {
		backups = append(backups, gin.H{
			"id":         b.ID,
			"label":      b.Label,
			"kind":       b.Kind,
			"size_bytes": b.SizeBytes,
			"created_at": b.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"backups": backups, "count": len(backups)})
}

// CreateBackup: POST /api/v1/devices/:id/backups  (capture a fresh snapshot)
func (h *BackupHandler) CreateBackup(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	deviceID := c.Param("id")
	if h.applier == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "live applier disabled — backup capture unavailable (fail closed)"})
		return
	}

	label := strings.TrimSpace(c.PostForm("label"))
	if label == "" {
		var body struct {
			Label string `json:"label"`
		}
		_ = c.ShouldBindJSON(&body)
		label = strings.TrimSpace(body.Label)
	}

	// Tenant check BEFORE touching the device.
	var deviceExists bool
	if err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		d, e := h.devices.GetDeviceByIDTXN(tx, deviceID)
		if e != nil {
			return e
		}
		deviceExists = d != nil
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if !deviceExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	// Capture over the SAME applier seam the live path uses.
	snap, err := h.applier.Snapshot(context.Background(), deviceID, nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "backup capture failed: " + err.Error()})
		return
	}
	content, err := json.Marshal(snap)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encode snapshot"})
		return
	}

	var created *database.DeviceConfigBackup
	err = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		b, e := h.devices.CreateBackupTXN(tx, businessID, deviceID, label, "snapshot", string(content), auth.GetUserID(c))
		if e != nil {
			return e
		}
		created = b
		return h.devices.RecordChangeTXN(tx, businessID, deviceID, auth.GetUserRole(c), "backup.create", "captured config backup "+b.ID)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store backup"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         created.ID,
		"label":      created.Label,
		"kind":       created.Kind,
		"size_bytes": created.SizeBytes,
		"created_at": created.CreatedAt,
	})
}

// DeleteBackup: DELETE /api/v1/devices/:id/backups/:backupId
func (h *BackupHandler) DeleteBackup(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	deviceID := c.Param("id")
	backupID := c.Param("backupId")

	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		if e := h.devices.DeleteBackupTXN(tx, businessID, backupID); e != nil {
			return e
		}
		return h.devices.RecordChangeTXN(tx, businessID, deviceID, auth.GetUserRole(c), "backup.delete", "deleted backup "+backupID)
	})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": backupID})
}

// Changes: GET /api/v1/devices/:id/changes  (change-history timeline)
func (h *BackupHandler) Changes(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	deviceID := c.Param("id")

	var events []*database.ChangeEvent
	err := runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		es, e := h.devices.ListDeviceChangesTXN(tx, businessID, deviceID)
		if e != nil {
			return e
		}
		events = es
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	changes := make([]gin.H, 0, len(events))
	for _, e := range events {
		changes = append(changes, gin.H{
			"id":        e.ID,
			"user_role": e.UserRole,
			"action":    e.Action,
			"reason":    e.Reason,
			"timestamp": e.Timestamp,
		})
	}
	c.JSON(http.StatusOK, gin.H{"changes": changes, "count": len(changes)})
}

// Restore: POST /api/v1/devices/:id/restore/:backupId  (high-risk, OTP-gated)
func (h *BackupHandler) Restore(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	deviceID := c.Param("id")
	backupID := c.Param("backupId")
	if h.applier == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "live applier disabled — restore unavailable (fail closed)"})
		return
	}

	var body struct {
		OTP string `json:"otp"`
	}
	_ = c.ShouldBindJSON(&body)

	// High-risk gate FIRST: no valid OTP => deny (fail closed; empty secret denies).
	ok, err := h.otp.Verify(context.Background(), deviceID, body.OTP)
	if err != nil || !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "OTP required for restore (high-risk action)"})
		return
	}

	var backup *database.DeviceConfigBackup
	err = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		b, e := h.devices.GetBackupTXN(tx, businessID, backupID)
		if e != nil {
			return e
		}
		backup = b
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if backup == nil || backup.DeviceID != deviceID {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}

	var snap provisioner.RollbackSnapshot
	if err := json.Unmarshal([]byte(backup.Content), &snap); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "backup content unreadable"})
		return
	}

	if err := h.applier.Restore(context.Background(), snap); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "restore failed: " + err.Error()})
		return
	}

	_ = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		return h.devices.RecordChangeTXN(tx, businessID, deviceID, auth.GetUserRole(c), "restore", "one-click restore from backup "+backupID)
	})

	c.JSON(http.StatusOK, gin.H{"restored": true, "backup_id": backupID, "at": time.Now().UTC()})
}
