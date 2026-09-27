package database

import (
	"database/sql"
	"errors"
	"fmt"

	"mega-magic/internal/model"
)

// ErrRouterConnectionNotFound is returned when a connection id does not exist
// inside the caller's business.
var ErrRouterConnectionNotFound = errors.New("router connection not found")

// RouterConnectionRepository reads and writes router management connections.
// Every method takes a *sql.Tx because all access must run inside a
// tenant-bound transaction (RLS context set).
type RouterConnectionRepository struct {
	db *DB
}

// NewRouterConnectionRepository creates a router connection repository.
func NewRouterConnectionRepository(db *DB) *RouterConnectionRepository {
	return &RouterConnectionRepository{db: db}
}

// connColumns is shared by every read. legacy_password_ciphertext is reduced to
// a boolean here on purpose — the ciphertext itself must never leave the DB.
const connColumns = `
	rc.id, rc.business_id, rc.name, rc.host, rc.port, rc.use_tls, rc.is_default,
	rc.site_id, s.name,
	rc.connection_mode, rc.cloud_status, rc.cloud_last_seen_at, rc.environment,
	(rc.legacy_password_ciphertext IS NOT NULL) AS creds_need_reentry,
	COALESCE((SELECT count(*) FROM devices d WHERE d.router_connection_id = rc.id), 0) AS device_count,
	rc.created_at, rc.updated_at`

func scanConn(row interface{ Scan(...any) error }) (*model.RouterConnection, error) {
	r := &model.RouterConnection{}
	var host, siteName, mode, status, env sql.NullString
	var port sql.NullInt64
	var siteID sql.NullString
	var lastSeen sql.NullTime
	if err := row.Scan(
		&r.ID, &r.BusinessID, &r.Name, &host, &port, &r.UseTLS, &r.IsDefault,
		&siteID, &siteName,
		&mode, &status, &lastSeen, &env,
		&r.CredentialsNeedReentry, &r.DeviceCount, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if host.Valid {
		r.Host = &host.String
	}
	if port.Valid {
		p := int(port.Int64)
		r.Port = &p
	}
	if siteID.Valid {
		r.SiteID = &siteID.String
	}
	if siteName.Valid {
		r.SiteName = &siteName.String
	}
	if mode.Valid {
		r.ConnectionMode = &mode.String
	}
	if status.Valid {
		r.CloudStatus = &status.String
	}
	if lastSeen.Valid {
		r.CloudLastSeenAt = &lastSeen.Time
	}
	if env.Valid {
		r.Environment = &env.String
	}
	return r, nil
}

// ListRouterConnectionsTXN returns every connection in the business with its
// site name and attached device count.
func (r *RouterConnectionRepository) ListRouterConnectionsTXN(tx *sql.Tx, businessID string) ([]*model.RouterConnection, error) {
	rows, err := tx.Query(`SELECT `+connColumns+`
		FROM router_connections rc
		LEFT JOIN sites s ON s.site_id = rc.site_id
		WHERE rc.business_id = $1
		ORDER BY rc.name`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list router connections: %w", err)
	}
	defer rows.Close()

	out := []*model.RouterConnection{}
	for rows.Next() {
		c, err := scanConn(rows)
		if err != nil {
			return nil, fmt.Errorf("scan router connection: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate router connections: %w", err)
	}
	return out, nil
}

// GetRouterConnectionTXN returns one connection, or ErrRouterConnectionNotFound.
func (r *RouterConnectionRepository) GetRouterConnectionTXN(tx *sql.Tx, businessID, id string) (*model.RouterConnection, error) {
	c, err := scanConn(tx.QueryRow(`SELECT `+connColumns+`
		FROM router_connections rc
		LEFT JOIN sites s ON s.site_id = rc.site_id
		WHERE rc.id = $1 AND rc.business_id = $2`, id, businessID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRouterConnectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get router connection: %w", err)
	}
	return c, nil
}

// CreateRouterConnectionTXN inserts a connection and returns it.
func (r *RouterConnectionRepository) CreateRouterConnectionTXN(tx *sql.Tx, businessID string, req *model.RouterConnectionRequest) (*model.RouterConnection, error) {
	useTLS := true
	if req.UseTLS != nil {
		useTLS = *req.UseTLS
	}
	isDefault := false
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	}

	var id string
	if err := tx.QueryRow(
		`INSERT INTO router_connections
		   (business_id, name, host, port, use_tls, is_default, site_id, connection_mode, environment)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id`,
		businessID, req.Name, req.Host, req.Port, useTLS, isDefault, req.SiteID,
		req.ConnectionMode, req.Environment,
	).Scan(&id); err != nil {
		return nil, fmt.Errorf("create router connection: %w", err)
	}
	return r.GetRouterConnectionTXN(tx, businessID, id)
}

// UpdateRouterConnectionTXN replaces a connection's editable fields. cloud_status
// is intentionally not writable — it is owned by the cloud handshake.
func (r *RouterConnectionRepository) UpdateRouterConnectionTXN(tx *sql.Tx, businessID, id string, req *model.RouterConnectionRequest) (*model.RouterConnection, error) {
	var existing struct {
		useTLS    bool
		isDefault bool
	}
	if err := tx.QueryRow(`SELECT use_tls, is_default FROM router_connections WHERE id = $1 AND business_id = $2`,
		id, businessID).Scan(&existing.useTLS, &existing.isDefault); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRouterConnectionNotFound
	} else if err != nil {
		return nil, fmt.Errorf("load router connection: %w", err)
	}

	// Omitted booleans keep their current value rather than silently flipping.
	useTLS, isDefault := existing.useTLS, existing.isDefault
	if req.UseTLS != nil {
		useTLS = *req.UseTLS
	}
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	}

	res, err := tx.Exec(
		`UPDATE router_connections
		    SET name = $1, host = $2, port = $3, use_tls = $4, is_default = $5,
		        site_id = $6, connection_mode = $7, environment = $8, updated_at = now()
		  WHERE id = $9 AND business_id = $10`,
		req.Name, req.Host, req.Port, useTLS, isDefault, req.SiteID,
		req.ConnectionMode, req.Environment, id, businessID)
	if err != nil {
		return nil, fmt.Errorf("update router connection: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrRouterConnectionNotFound
	}
	return r.GetRouterConnectionTXN(tx, businessID, id)
}

// DeleteRouterConnectionTXN removes a connection. Devices, daily closes and
// device allowances all reference it with ON DELETE SET NULL, so dependent rows
// survive and simply lose the link.
func (r *RouterConnectionRepository) DeleteRouterConnectionTXN(tx *sql.Tx, businessID, id string) error {
	res, err := tx.Exec(`DELETE FROM router_connections WHERE id = $1 AND business_id = $2`, id, businessID)
	if err != nil {
		return fmt.Errorf("delete router connection: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrRouterConnectionNotFound
	}
	return nil
}
