package database

import (
	"database/sql"
	"errors"
	"fmt"

	"mega-magic/internal/model"
)

// ErrDeviceRequestNotFound is returned when a request id is not in the caller's
// business.
var ErrDeviceRequestNotFound = errors.New("device request not found")

// ErrDeviceRequestAlreadyDecided is returned when a request has already reached
// a terminal state — a decision is final and must not be silently overwritten.
var ErrDeviceRequestAlreadyDecided = errors.New("device request already decided")

// ErrDeviceCapacityExceeded prevents a write from silently exceeding its
// granted tenant capacity.
var ErrDeviceCapacityExceeded = errors.New("device capacity exceeded")

// DeviceAllowanceRepository reads a business's granted capacity plus live usage,
// and manages the device-request workflow.
type DeviceAllowanceRepository struct {
	db *DB
}

// NewDeviceAllowanceRepository creates a device allowance repository.
func NewDeviceAllowanceRepository(db *DB) *DeviceAllowanceRepository {
	return &DeviceAllowanceRepository{db: db}
}

// CheckCapacityTXN serializes checks on the tenant allowance row. The caller
// creates the device/site in this same transaction, so concurrent requests
// cannot both consume the final available slot.
func (r *DeviceAllowanceRepository) CheckCapacityTXN(tx *sql.Tx, businessID, kind string) error {
	if !model.IsDeviceAllowanceKind(kind) {
		return fmt.Errorf("unknown capacity kind %q", kind)
	}
	if _, err := tx.Exec(`INSERT INTO device_allowances (business_id, routers)
		VALUES ($1, 1) ON CONFLICT (business_id) DO NOTHING`, businessID); err != nil {
		return fmt.Errorf("ensure device allowance: %w", err)
	}
	column := map[string]string{"routers": "routers", "controllers": "controllers", "sites": "sites"}[kind]
	var granted int
	if err := tx.QueryRow(`SELECT `+column+` FROM device_allowances WHERE business_id=$1 FOR UPDATE`, businessID).Scan(&granted); err != nil {
		return fmt.Errorf("lock device allowance: %w", err)
	}
	var used int
	var query string
	switch kind {
	case "routers":
		query = `SELECT count(*) FROM devices WHERE business_id=$1 AND deployment_layer IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH')`
	case "controllers":
		query = `SELECT count(*) FROM devices WHERE business_id=$1 AND deployment_layer='CONTROLLER'`
	case "sites":
		query = `SELECT count(*) FROM sites WHERE business_id=$1`
	}
	if err := tx.QueryRow(query, businessID).Scan(&used); err != nil {
		return fmt.Errorf("count capacity usage: %w", err)
	}
	if used >= granted {
		return ErrDeviceCapacityExceeded
	}
	return nil
}

// GetAllowanceTXN returns the business's allowance with live usage counted.
// A business with no allowance row still gets its included first-router slot
// plus real usage, so the UI never has to special-case a missing row.
func (r *DeviceAllowanceRepository) GetAllowanceTXN(tx *sql.Tx, businessID string) (*model.DeviceAllowance, error) {
	// A tenant always owns the included first-router slot, even before a row is
	// materialized by its first capacity-changing write.
	a := &model.DeviceAllowance{BusinessID: businessID, Routers: 1}

	var included sql.NullString
	err := tx.QueryRow(`
		SELECT routers, controllers, sites, included_router_id, created_at, updated_at
		  FROM device_allowances WHERE business_id = $1`, businessID).
		Scan(&a.Routers, &a.Controllers, &a.Sites, &included, &a.CreatedAt, &a.UpdatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get device allowance: %w", err)
	}
	if included.Valid {
		a.IncludedRouterID = &included.String
		var name sql.NullString
		if err := tx.QueryRow(`SELECT name FROM router_connections WHERE id = $1`,
			included.String).Scan(&name); err == nil && name.Valid {
			a.IncludedRouterName = &name.String
		}
	}

	// Live usage. Routers/controllers are derived from the deployment layer;
	// access points are reported separately rather than folded into either.
	if err := tx.QueryRow(`
		SELECT
		  (SELECT count(*) FROM sites s WHERE s.business_id = $1),
		  (SELECT count(*) FROM devices d WHERE d.business_id = $1 AND d.deployment_layer = 'CONTROLLER'),
		  (SELECT count(*) FROM devices d WHERE d.business_id = $1 AND d.deployment_layer IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH')),
		  (SELECT count(*) FROM devices d WHERE d.business_id = $1 AND d.deployment_layer = 'ACCESS_POINT')`,
		businessID).Scan(&a.UsedSites, &a.UsedControllers, &a.UsedRouters, &a.UsedAccessPoints); err != nil {
		return nil, fmt.Errorf("device allowance usage: %w", err)
	}

	byLayer := map[string]int{}
	rows, err := tx.Query(`SELECT COALESCE(d.deployment_layer::text, 'UNKNOWN'), count(*)
		FROM devices d WHERE d.business_id = $1 GROUP BY 1`, businessID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var layer string
			var n int
			if err := rows.Scan(&layer, &n); err == nil {
				byLayer[layer] = n
			}
		}
	}
	a.UsedDevicesByLayer = byLayer

	return a, nil
}

// ListRequestsTXN returns every capacity request in the business, newest first.
func (r *DeviceAllowanceRepository) ListRequestsTXN(tx *sql.Tx, businessID string) ([]*model.DeviceRequest, error) {
	rows, err := tx.Query(`
		SELECT dr.id, dr.business_id, dr.user_id, u.email, dr.kind, dr.quantity, dr.reason, dr.status,
		       dr.decided_by, dr.decided_at, dr.created_at, dr.updated_at
		  FROM device_requests dr
		  LEFT JOIN users u ON u.user_id = dr.user_id
		 WHERE dr.business_id = $1
		 ORDER BY dr.created_at DESC`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list device requests: %w", err)
	}
	defer rows.Close()

	out := []*model.DeviceRequest{}
	for rows.Next() {
		q := &model.DeviceRequest{}
		var userID, email, reason, decidedBy sql.NullString
		var decidedAt sql.NullTime
		if err := rows.Scan(&q.ID, &q.BusinessID, &userID, &email, &q.Kind, &q.Quantity, &reason, &q.Status,
			&decidedBy, &decidedAt, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan device request: %w", err)
		}
		if userID.Valid {
			q.UserID = &userID.String
		}
		if email.Valid {
			q.UserEmail = &email.String
		}
		if reason.Valid {
			q.Reason = &reason.String
		}
		if decidedBy.Valid {
			q.DecidedBy = &decidedBy.String
		}
		if decidedAt.Valid {
			q.DecidedAt = &decidedAt.Time
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate device requests: %w", err)
	}
	return out, nil
}

// CreateRequestTXN opens a request on behalf of userID.
func (r *DeviceAllowanceRepository) CreateRequestTXN(tx *sql.Tx, businessID, userID string, req *model.DeviceRequestCreate) (*model.DeviceRequest, error) {
	var id string
	if err := tx.QueryRow(`
		INSERT INTO device_requests (business_id, user_id, kind, quantity, reason, status)
		VALUES ($1, $2, $3, $4, $5, 'pending')
		RETURNING id`, businessID, userID, req.Kind, req.Quantity, req.Reason).Scan(&id); err != nil {
		return nil, fmt.Errorf("create device request: %w", err)
	}
	return r.getRequestTXN(tx, businessID, id)
}

func (r *DeviceAllowanceRepository) getRequestTXN(tx *sql.Tx, businessID, id string) (*model.DeviceRequest, error) {
	rows, err := r.ListRequestsTXN(tx, businessID)
	if err != nil {
		return nil, err
	}
	for _, q := range rows {
		if q.ID == id {
			return q, nil
		}
	}
	return nil, ErrDeviceRequestNotFound
}

// DecideRequestTXN approves or rejects a request. Only a PENDING request can be
// decided — the UPDATE is guarded on `status = 'pending'` so two approvers racing
// cannot both claim the decision, and a re-decision returns
// ErrDeviceRequestAlreadyDecided rather than silently overwriting the first.
func (r *DeviceAllowanceRepository) DecideRequestTXN(tx *sql.Tx, businessID, id, decidedBy, status string) (*model.DeviceRequest, error) {
	var kind string
	var quantity int
	var existingStatus string
	if err := tx.QueryRow(`SELECT kind, quantity, status FROM device_requests
		WHERE id = $1 AND business_id = $2 FOR UPDATE`, id, businessID).
		Scan(&kind, &quantity, &existingStatus); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeviceRequestNotFound
	} else if err != nil {
		return nil, fmt.Errorf("lock device request: %w", err)
	}
	if existingStatus != "pending" {
		return nil, ErrDeviceRequestAlreadyDecided
	}
	if !model.IsDeviceAllowanceKind(kind) || quantity < 1 {
		return nil, fmt.Errorf("invalid stored device request")
	}

	res, err := tx.Exec(`
		UPDATE device_requests
		   SET status = $1, decided_by = $2, decided_at = now(), updated_at = now()
		 WHERE id = $3 AND business_id = $4 AND status = 'pending'`,
		status, decidedBy, id, businessID)
	if err != nil {
		return nil, fmt.Errorf("decide device request: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Distinguish "no such request" from "already decided".
		var existing string
		err := tx.QueryRow(`SELECT status FROM device_requests WHERE id = $1 AND business_id = $2`,
			id, businessID).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeviceRequestNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("check device request: %w", err)
		}
		return nil, ErrDeviceRequestAlreadyDecided
	}
	if status == "approved" {
		// kind is constrained above and mapped explicitly, never interpolated from
		// unchecked request data. The grant and decision share one transaction.
		column := map[string]string{"routers": "routers", "controllers": "controllers", "sites": "sites"}[kind]
		if _, err := tx.Exec(`INSERT INTO device_allowances (business_id, routers) VALUES ($1, 1)
			ON CONFLICT (business_id) DO NOTHING`, businessID); err != nil {
			return nil, fmt.Errorf("ensure device allowance grant row: %w", err)
		}
		query := `UPDATE device_allowances SET ` + column + ` = ` + column + ` + $2, updated_at = now() WHERE business_id = $1`
		if _, err := tx.Exec(query, businessID, quantity); err != nil {
			return nil, fmt.Errorf("apply device allowance grant: %w", err)
		}
	}
	return r.getRequestTXN(tx, businessID, id)
}
