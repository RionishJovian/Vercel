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
// Fail-closed by default: backup capture and live restore are separate
// capabilities. A capture writes named export/.backup artifacts on the ROUTER
// and stores rollback metadata in the database; it does not transfer those
// files off-router. Restore is enabled only by the full applier + OTP gate.
// Reads (list/timeline) remain tenant-scoped.
type BackupSnapshotter interface {
	Snapshot(ctx context.Context, businessID, peerID string, changes []provisioner.Change) (provisioner.RollbackSnapshot, error)
}

type BackupHandler struct {
	db      *database.DB
	devices *database.DeviceRepository

	capture BackupSnapshotter        // nil => capture fails closed (503)
	applier provisioner.Applier       // nil => restore fails closed (503)
	otp     *provisioner.TOTPVerifier // nil/empty-secret => restore high-risk DENIED
}

func NewBackupHandler(db *database.DB, devices *database.DeviceRepository) *BackupHandler {
	return &BackupHandler{db: db, devices: devices}
}

// EnableCapture attaches ONLY the Snapshot interface. It does not provide an
// Apply or Restore handle, even when the underlying transport supports them.
// Operator configuration must explicitly permit on-router snapshot artifacts.
func (h *BackupHandler) EnableCapture(capture BackupSnapshotter) { h.capture = capture }

// EnableLive is the existing full-applier/OTP path. It continues to authorize
// both capture and restore only when the server wires that path explicitly.
func (h *BackupHandler) EnableLive(applier provisioner.Applier, otp *provisioner.TOTPVerifier) {
	h.applier = applier
	h.otp = otp
	h.capture = applier
}

func (h *BackupHandler) captureAvailable() bool { return h.capture != nil }
func (h *BackupHandler) restoreAvailable() bool { return h.applier != nil && h.otp != nil && h.otp.Configured() }

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
	// The console enables its buttons from these, so they say what these routes would really do.
	c.JSON(http.StatusOK, gin.H{"backups": backups, "count": len(backups), "capture_available": h.captureAvailable(), "restore_available": h.restoreAvailable(), "capture_scope": "router_artifacts_and_database_metadata"})
}

// CreateBackup: POST /api/v1/devices/:id/backups  (capture a fresh snapshot)
func (h *BackupHandler) CreateBackup(c *gin.Context) {
	businessID := auth.GetBusinessID(c)
	deviceID := c.Param("id")
	if h.capture == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "router backup capture is disabled; enable provisioner.backup_capture.enabled with pinned per-device SSH access (restore stays separate)"})
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

	// Capture uses only the limited Snapshot interface. The implementation
	// writes artifacts on-router, so it is not a read-only RouterOS operation.
	snap, err := h.capture.Snapshot(c.Request.Context(), businessID, deviceID, nil)
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
	if !h.restoreAvailable() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "live restore is disabled or OTP not configured (fail closed)"})
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

	if err := h.applier.Restore(context.Background(), businessID, snap); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "restore failed: " + err.Error()})
		return
	}

	_ = runInTenant(c, h.db, businessID, func(tx *sql.Tx) error {
		return h.devices.RecordChangeTXN(tx, businessID, deviceID, auth.GetUserRole(c), "restore", "one-click restore from backup "+backupID)
	})

	c.JSON(http.StatusOK, gin.H{"restored": true, "backup_id": backupID, "at": time.Now().UTC()})
}
