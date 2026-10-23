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

// ListAllPeerKeys returns every managed peer across all tenants — the
// authoritative fleet the state-sweep worker iterates. Lightweight: only
// business_id + device_id (use ListDevicesByBusiness for full device rows).
func (r *DeviceRepository) ListAllPeerKeys() ([]*model.PeerKey, error) {
	rows, err := r.db.Query("SELECT business_id, device_id FROM devices")
	if err != nil {
		return nil, fmt.Errorf("list all peer keys: %w", err)
	}
	defer rows.Close()

	var keys []*model.PeerKey
	for rows.Next() {
		k := &model.PeerKey{}
		if err := rows.Scan(&k.BusinessID, &k.DeviceID); err != nil {
			return nil, fmt.Errorf("scan peer key row: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate peer key rows: %w", err)
	}

	return keys, nil
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

// ---------------------------------------------------------------------------
// Transaction-scoped variants (RLS enforcement rollout: device request path).
// Same SQL on the caller's *sql.Tx so they can run under WithTenant. Bare
// versions stay for the later full rollout.
//
// NOTE: ListAllPeerKeys is deliberately NOT converted — it is the SWEEP WORKER's
// cross-tenant system path (iterate every business to evaluate every peer); it
// must stay owner-level, never RLS-scoped.
// ---------------------------------------------------------------------------

// CreateDeviceTXN is the transaction-scoped variant of CreateDevice.
func (r *DeviceRepository) CreateDeviceTXN(tx *sql.Tx, device *model.Device) (*model.Device, error) {
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

	err := tx.QueryRow(query,
		device.BusinessID, nullString(device.ParentGatewayID), device.HardwareName,
		nullString(device.DeviceModel), device.DeploymentLayer, device.Vendor, device.Category,
		device.Transport, macAddrStr(device.MACAddress), mgmtIPStr(device.ManagementIP),
		device.IsOnline, nullTime(device.LastSeenAt),
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

// GetDeviceByIDTXN is the transaction-scoped variant of GetDeviceByID.
func (r *DeviceRepository) GetDeviceByIDTXN(tx *sql.Tx, deviceID string) (*model.Device, error) {
	query := `
		SELECT device_id, business_id, parent_gateway_id, hardware_name, device_model,
		       deployment_layer, vendor, category, transport, mac_address,
		       management_ip, is_online, last_seen_at, created_at, updated_at
		FROM devices
		WHERE device_id = $1`

	d := &model.Device{}
	var parentGatewayID, deviceModel, macAddr, mgmtIP sql.NullString
	var lastSeen sql.NullTime

	err := tx.QueryRow(query, deviceID).Scan(
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

// ListDevicesByBusinessTXN is the transaction-scoped variant of
// ListDevicesByBusiness.
func (r *DeviceRepository) ListDevicesByBusinessTXN(tx *sql.Tx, businessID string) ([]*model.Device, error) {
	query := `
		SELECT device_id, business_id, parent_gateway_id, hardware_name, device_model,
		       deployment_layer, vendor, category, transport, mac_address,
		       management_ip, is_online, last_seen_at, created_at, updated_at
		FROM devices
		WHERE business_id = $1
		ORDER BY created_at DESC`

	rows, err := tx.Query(query, businessID)
	if err != nil {
		return nil, fmt.Errorf("list devices by business: %w", err)
	}
	defer rows.Close()

	var devices []*model.Device
	for rows.Next() {
		d := &model.Device{}
		var parentGatewayID, deviceModel, macAddr, mgmtIP sql.NullString
		var lastSeen sql.NullTime
		if err := rows.Scan(
			&d.DeviceID, &d.BusinessID, &parentGatewayID, &d.HardwareName, &deviceModel,
			&d.DeploymentLayer, &d.Vendor, &d.Category, &d.Transport, &macAddr,
			&mgmtIP, &d.IsOnline, &lastSeen, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
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

// UpdateDeviceTXN is the transaction-scoped variant of UpdateDevice.
func (r *DeviceRepository) UpdateDeviceTXN(tx *sql.Tx, device *model.Device) (*model.Device, error) {
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

	err := tx.QueryRow(query,
		device.DeviceID, nullString(device.ParentGatewayID), device.HardwareName,
		nullString(device.DeviceModel), device.DeploymentLayer, device.Vendor, device.Category,
		device.Transport, macAddrStr(device.MACAddress), mgmtIPStr(device.ManagementIP),
		device.IsOnline, nullTime(device.LastSeenAt), time.Now().UTC(),
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

// DeleteDeviceTXN is the transaction-scoped variant of DeleteDevice.
func (r *DeviceRepository) DeleteDeviceTXN(tx *sql.Tx, deviceID string) error {
	result, err := tx.Exec("DELETE FROM devices WHERE device_id = $1", deviceID)
	if err != nil {
		return fmt.Errorf("delete device: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("device not found")
	}
	return nil
}

// DiscoveryLabel represents a saved friendly name and type for a discovered device.
type DiscoveryLabel struct {
	MACAddress string    `json:"mac_address"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// GetDiscoveryLabels retrieves all saved discovery labels for a device.
func (r *DeviceRepository) GetDiscoveryLabels(deviceID string) ([]*DiscoveryLabel, error) {
	query := `
		SELECT mac_address, name, kind, created_at, updated_at
		FROM device_discovery_labels
		WHERE device_id = $1
		ORDER BY created_at DESC`

	rows, err := r.db.Query(query, deviceID)
	if err != nil {
		return nil, fmt.Errorf("get discovery labels: %w", err)
	}
	defer rows.Close()

	var labels []*DiscoveryLabel
	for rows.Next() {
		l := &DiscoveryLabel{}
		if err := rows.Scan(&l.MACAddress, &l.Name, &l.Kind, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan discovery label: %w", err)
		}
		labels = append(labels, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate discovery labels: %w", err)
	}
	return labels, nil
}

// UpsertDiscoveryLabel creates or updates a discovery label for a device.
func (r *DeviceRepository) UpsertDiscoveryLabel(businessID, deviceID, macAddress, name, kind string) (*DiscoveryLabel, error) {
	query := `
		INSERT INTO device_discovery_labels (business_id, device_id, mac_address, name, kind, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (business_id, device_id, mac_address) DO UPDATE SET
			name = EXCLUDED.name,
			kind = EXCLUDED.kind,
			updated_at = NOW()
		RETURNING mac_address, name, kind, created_at, updated_at`

	l := &DiscoveryLabel{}
	err := r.db.QueryRow(query, businessID, deviceID, macAddress, name, kind).Scan(
		&l.MACAddress, &l.Name, &l.Kind, &l.CreatedAt, &l.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert discovery label: %w", err)
	}
	return l, nil
}
