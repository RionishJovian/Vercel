package database

import (
	"database/sql"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

// DeviceRepository handles device database operations
type DeviceRepository struct {
	db *DB
}

// NewDeviceRepository creates a new device repository
func NewDeviceRepository(db *DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

// CreateDevice inserts a new device record
func (r *DeviceRepository) CreateDevice(device *model.Device) (*model.Device, error) {
	query := `
		INSERT INTO devices (business_id, parent_gateway_id, hardware_name, device_model,
		                     deployment_layer, vendor, category, transport, mac_address,
		                     management_ip, is_online, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING device_id, business_id, parent_gateway_id, hardware_name, device_model,
		          deployment_layer, vendor, category, transport, mac_address,
		          management_ip, is_online, last_seen_at, created_at, updated_at`

	d := &model.Device{}
	var parentGatewayID, deviceModel, macAddr, mgmtIP sql.NullString
	var lastSeen sql.NullTime

	err := r.db.QueryRow(query,
		device.BusinessID,
		nullString(device.ParentGatewayID),
		device.HardwareName,
		nullString(device.DeviceModel),
		device.DeploymentLayer,
		device.Vendor,
		device.Category,
		device.Transport,
		macAddrStr(device.MACAddress),
		mgmtIPStr(device.ManagementIP),
		device.IsOnline,
		nullTime(device.LastSeenAt),
	).Scan(
		&d.DeviceID, &d.BusinessID, &parentGatewayID, &d.HardwareName, &deviceModel,
		&d.DeploymentLayer, &d.Vendor, &d.Category, &d.Transport, &macAddr,
		&mgmtIP, &d.IsOnline, &lastSeen, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create device: %w", err)
	}

	d.ParentGatewayID = scanNullableString(parentGatewayID)
	d.DeviceModel = scanNullableString(deviceModel)
	d.MACAddress = scanMACAddr(macAddr)
	d.ManagementIP = scanIP(mgmtIP)
	d.LastSeenAt = scanNullableTime(lastSeen)

	return d, nil
}

// GetDeviceByID retrieves a device by its ID
func (r *DeviceRepository) GetDeviceByID(deviceID string) (*model.Device, error) {
	query := `
		SELECT device_id, business_id, parent_gateway_id, hardware_name, device_model,
		       deployment_layer, vendor, category, transport, mac_address,
		       management_ip, is_online, last_seen_at, created_at, updated_at
		FROM devices
		WHERE device_id = $1`

	d := &model.Device{}
	var parentGatewayID, deviceModel, macAddr, mgmtIP sql.NullString
	var lastSeen sql.NullTime

	err := r.db.QueryRow(query, deviceID).Scan(
		&d.DeviceID, &d.BusinessID, &parentGatewayID, &d.HardwareName, &deviceModel,
		&d.DeploymentLayer, &d.Vendor, &d.Category, &d.Transport, &macAddr,
		&mgmtIP, &d.IsOnline, &lastSeen, &d.CreatedAt, &d.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get device by id: %w", err)
	}

	d.ParentGatewayID = scanNullableString(parentGatewayID)
	d.DeviceModel = scanNullableString(deviceModel)
	d.MACAddress = scanMACAddr(macAddr)
	d.ManagementIP = scanIP(mgmtIP)
	d.LastSeenAt = scanNullableTime(lastSeen)

	return d, nil
}

// ListDevicesByBusiness retrieves all devices for a given business
func (r *DeviceRepository) ListDevicesByBusiness(businessID string) ([]*model.Device, error) {
	query := `
		SELECT device_id, business_id, parent_gateway_id, hardware_name, device_model,
		       deployment_layer, vendor, category, transport, mac_address,
		       management_ip, is_online, last_seen_at, created_at, updated_at
		FROM devices
		WHERE business_id = $1
		ORDER BY created_at DESC`

	rows, err := r.db.Query(query, businessID)
	if err != nil {
		return nil, fmt.Errorf("list devices by business: %w", err)
	}
	defer rows.Close()

	var devices []*model.Device
	for rows.Next() {
		d := &model.Device{}
		var parentGatewayID, deviceModel, macAddr, mgmtIP sql.NullString
		var lastSeen sql.NullTime

		err := rows.Scan(
			&d.DeviceID, &d.BusinessID, &parentGatewayID, &d.HardwareName, &deviceModel,
			&d.DeploymentLayer, &d.Vendor, &d.Category, &d.Transport, &macAddr,
			&mgmtIP, &d.IsOnline, &lastSeen, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan device row: %w", err)
		}

		d.ParentGatewayID = scanNullableString(parentGatewayID)
		d.DeviceModel = scanNullableString(deviceModel)
		d.MACAddress = scanMACAddr(macAddr)
		d.ManagementIP = scanIP(mgmtIP)
		d.LastSeenAt = scanNullableTime(lastSeen)

		devices = append(devices, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device rows: %w", err)
	}

	return devices, nil
}

// UpdateDevice updates an existing device record
func (r *DeviceRepository) UpdateDevice(device *model.Device) (*model.Device, error) {
	query := `
		UPDATE devices
		SET parent_gateway_id = $2, hardware_name = $3, device_model = $4,
		    deployment_layer = $5, vendor = $6, category = $7, transport = $8,
		    mac_address = $9, management_ip = $10, is_online = $11,
		    last_seen_at = $12, updated_at = $13
		WHERE device_id = $1
		RETURNING device_id, business_id, parent_gateway_id, hardware_name, device_model,
		          deployment_layer, vendor, category, transport, mac_address,
		          management_ip, is_online, last_seen_at, created_at, updated_at`

	d := &model.Device{}
	var parentGatewayID, deviceModel, macAddr, mgmtIP sql.NullString
	var lastSeen sql.NullTime

	err := r.db.QueryRow(query,
		device.DeviceID,
		nullString(device.ParentGatewayID),
		device.HardwareName,
		nullString(device.DeviceModel),
		device.DeploymentLayer,
		device.Vendor,
		device.Category,
		device.Transport,
		macAddrStr(device.MACAddress),
		mgmtIPStr(device.ManagementIP),
		device.IsOnline,
		nullTime(device.LastSeenAt),
		time.Now().UTC(),
	).Scan(
		&d.DeviceID, &d.BusinessID, &parentGatewayID, &d.HardwareName, &deviceModel,
		&d.DeploymentLayer, &d.Vendor, &d.Category, &d.Transport, &macAddr,
		&mgmtIP, &d.IsOnline, &lastSeen, &d.CreatedAt, &d.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update device: %w", err)
	}

	d.ParentGatewayID = scanNullableString(parentGatewayID)
	d.DeviceModel = scanNullableString(deviceModel)
	d.MACAddress = scanMACAddr(macAddr)
	d.ManagementIP = scanIP(mgmtIP)
	d.LastSeenAt = scanNullableTime(lastSeen)

	return d, nil
}

// DeleteDevice removes a device by its ID
func (r *DeviceRepository) DeleteDevice(deviceID string) error {
	result, err := r.db.Exec("DELETE FROM devices WHERE device_id = $1", deviceID)
	if err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("device not found")
	}
	return nil
}
