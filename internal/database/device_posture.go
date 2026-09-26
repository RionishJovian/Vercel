package database

import (
	"database/sql"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

// DevicePosture is a lean read for the security-posture audit: the device
// identity plus its captured RouterOS version and classification. It does not
// pull the whole device row.
type DevicePosture struct {
	DeviceID     string
	BusinessID   string
	HardwareName string
	Version      *string
	Channel      *string
	CheckedAt    *time.Time
	Posture      string
}

// SavePostureTXN records a device's observed RouterOS version + classification,
// tenant-scoped (runs inside a WithTenant tx). Fail-soft upstream: a capture
// failure simply never calls this.
func (r *DeviceRepository) SavePostureTXN(tx *sql.Tx, businessID, deviceID, version, channel, posture string, now time.Time) error {
	var ch *string
	if channel != "" {
		ch = &channel
	}
	res, err := tx.Exec(
		`UPDATE devices
		    SET routeros_version=$3, routeros_channel=$4, version_checked_at=$5, security_posture=$6, updated_at=$5
		  WHERE device_id=$1 AND business_id=$2`,
		deviceID, businessID, version, ch, now, posture)
	if err != nil {
		return fmt.Errorf("save posture: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("save posture: device not found in tenant")
	}
	return nil
}

const devicePostureCols = `device_id, business_id, hardware_name, routeros_version, routeros_channel, version_checked_at, security_posture`

func scanDevicePosture(row interface{ Scan(...interface{}) error }) (*DevicePosture, error) {
	p := &DevicePosture{}
	var version, channel, posture sql.NullString
	var checked sql.NullTime
	err := row.Scan(&p.DeviceID, &p.BusinessID, &p.HardwareName, &version, &channel, &checked, &posture)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan device posture: %w", err)
	}
	if version.Valid {
		p.Version = &version.String
	}
	if channel.Valid {
		p.Channel = &channel.String
	}
	if checked.Valid {
		p.CheckedAt = &checked.Time
	}
	if posture.Valid && posture.String != "" {
		p.Posture = posture.String
	} else {
		p.Posture = "unknown"
	}
	return p, nil
}

// ListPostureTXN returns every device's posture for one tenant.
func (r *DeviceRepository) ListPostureTXN(tx *sql.Tx, businessID string) ([]*DevicePosture, error) {
	rows, err := tx.Query(`SELECT `+devicePostureCols+` FROM devices WHERE business_id=$1 ORDER BY hardware_name`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list posture: %w", err)
	}
	defer rows.Close()

	var out []*DevicePosture
	for rows.Next() {
		p, err := scanDevicePosture(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPostureTXN returns one device's posture, scoped to the tenant (nil when the
// device is not this tenant's — no existence leak).
func (r *DeviceRepository) GetPostureTXN(tx *sql.Tx, businessID, deviceID string) (*DevicePosture, error) {
	return scanDevicePosture(tx.QueryRow(
		`SELECT `+devicePostureCols+` FROM devices WHERE device_id=$1 AND business_id=$2`, deviceID, businessID))
}

var _ = model.Device{} // keep model import (posture rows are device-scoped)
