package database

import (
	"database/sql"
	"fmt"
	"time"
)

// DeviceConfigBackup is one tenant-scoped config snapshot for a device. content
// holds the serialized RollbackSnapshot (export artifact name + WireGuard values)
// so a one-click restore can round-trip through the SAME applier seam the live
// path uses.
type DeviceConfigBackup struct {
	ID         string
	BusinessID string
	DeviceID   string
	Label      string
	Kind       string
	Content    string
	SizeBytes  int
	CreatedAt  time.Time
}

// ChangeEvent is one entry of a device's change-history timeline
// (device_change_events — a tenant-scoped table, distinct from the system-level
// audit_events trail whose RLS policy denies tenant inserts).
type ChangeEvent struct {
	ID        string
	UserRole  string
	Action    string
	Reason    string
	Timestamp time.Time
}

const deviceBackupCols = `id, business_id, device_id, label, kind, content, size_bytes, created_at`

func scanDeviceBackup(row interface{ Scan(...interface{}) error }) (*DeviceConfigBackup, error) {
	b := &DeviceConfigBackup{}
	var label, kind, content sql.NullString
	var size sql.NullInt64
	err := row.Scan(&b.ID, &b.BusinessID, &b.DeviceID, &label, &kind, &content, &size, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan device backup: %w", err)
	}
	b.Label = label.String
	b.Kind = kind.String
	b.Content = content.String
	b.SizeBytes = int(size.Int64)
	return b, nil
}

// CreateBackupTXN inserts a device config backup (tenant-scoped).
func (r *DeviceRepository) CreateBackupTXN(tx *sql.Tx, businessID, deviceID, label, kind, content, createdBy string) (*DeviceConfigBackup, error) {
	var by *string
	if createdBy != "" {
		by = &createdBy
	}
	return scanDeviceBackup(tx.QueryRow(
		`INSERT INTO device_config_backups (business_id, device_id, label, kind, content, size_bytes, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+deviceBackupCols,
		businessID, deviceID, label, kind, content, len(content), by))
}

// ListBackupsByDeviceTXN lists a device's backups, newest first (metadata only
// where possible — content is included; the UI slices by id for restore).
func (r *DeviceRepository) ListBackupsByDeviceTXN(tx *sql.Tx, businessID, deviceID string) ([]*DeviceConfigBackup, error) {
	rows, err := tx.Query(
		`SELECT `+deviceBackupCols+` FROM device_config_backups
		  WHERE business_id=$1 AND device_id=$2 ORDER BY created_at DESC`, businessID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list device backups: %w", err)
	}
	defer rows.Close()
	var out []*DeviceConfigBackup
	for rows.Next() {
		b, err := scanDeviceBackup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetBackupTXN returns one backup, tenant-scoped (nil = not this tenant's).
func (r *DeviceRepository) GetBackupTXN(tx *sql.Tx, businessID, backupID string) (*DeviceConfigBackup, error) {
	return scanDeviceBackup(tx.QueryRow(
		`SELECT `+deviceBackupCols+` FROM device_config_backups WHERE id=$1 AND business_id=$2`, backupID, businessID))
}

// DeleteBackupTXN removes one backup, tenant-scoped.
func (r *DeviceRepository) DeleteBackupTXN(tx *sql.Tx, businessID, backupID string) error {
	res, err := tx.Exec(`DELETE FROM device_config_backups WHERE id=$1 AND business_id=$2`, backupID, businessID)
	if err != nil {
		return fmt.Errorf("delete device backup: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("device backup not found in tenant")
	}
	return nil
}

// ListDeviceChangesTXN returns a device's change-history timeline,
// tenant-scoped.
func (r *DeviceRepository) ListDeviceChangesTXN(tx *sql.Tx, businessID, deviceID string) ([]*ChangeEvent, error) {
	rows, err := tx.Query(
		`SELECT id, user_role, action, reason, created_at
		   FROM device_change_events
		  WHERE business_id=$1 AND device_id=$2
		  ORDER BY created_at DESC LIMIT 200`, businessID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list device changes: %w", err)
	}
	defer rows.Close()
	var out []*ChangeEvent
	for rows.Next() {
		e := &ChangeEvent{}
		if err := rows.Scan(&e.ID, &e.UserRole, &e.Action, &e.Reason, &e.Timestamp); err != nil {
			return nil, fmt.Errorf("scan change event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RecordChangeTXN appends one change-history entry for a device (tenant-scoped).
func (r *DeviceRepository) RecordChangeTXN(tx *sql.Tx, businessID, deviceID, userRole, action, reason string) error {
	_, err := tx.Exec(
		`INSERT INTO device_change_events (business_id, device_id, user_role, action, reason)
		 VALUES ($1, $2, $3, $4, $5)`,
		businessID, deviceID, userRole, action, reason)
	if err != nil {
		return fmt.Errorf("record change: %w", err)
	}
	return nil
}
