package database

import (
	"database/sql"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

// AuditEventRepository handles audit event database operations
type AuditEventRepository struct {
	db *DB
}

// NewAuditEventRepository creates a new audit event repository
func NewAuditEventRepository(db *DB) *AuditEventRepository {
	return &AuditEventRepository{db: db}
}

// Insert creates a new audit event
func (r *AuditEventRepository) Insert(event *model.AuditEvent) (*model.AuditEvent, error) {
	query := `
		INSERT INTO audit_events (business_id, user_id, user_role, action, entity_type,
		                          entity_id, before_state, after_state, reason, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, business_id, user_id, user_role, action, entity_type,
		          entity_id, before_state, after_state, reason, ip_address, user_agent, timestamp`

	e := &model.AuditEvent{}
	var userID, ipAddr, userAgent sql.NullString
	var beforeState, afterState sql.NullString

	err := r.db.QueryRow(query,
		event.BusinessID, event.UserID, event.UserRole, event.Action, event.EntityType,
		event.EntityID, event.BeforeState, event.AfterState, event.Reason, event.IPAddress, event.UserAgent,
	).Scan(
		&e.ID, &e.BusinessID, &userID, &e.UserRole, &e.Action, &e.EntityType,
		&e.EntityID, &beforeState, &afterState, &e.Reason, &ipAddr, &userAgent, &e.Timestamp,
	)
	if err != nil {
		return nil, fmt.Errorf("insert audit event: %w", err)
	}

	if userID.Valid {
		e.UserID = &userID.String
	}
	if ipAddr.Valid {
		e.IPAddress = &ipAddr.String
	}
	if userAgent.Valid {
		e.UserAgent = &userAgent.String
	}
	if beforeState.Valid {
		e.BeforeState = &beforeState.String
	}
	if afterState.Valid {
		e.AfterState = &afterState.String
	}

	return e, nil
}

// ListByBusiness returns paginated audit events for a business within an optional time range
func (r *AuditEventRepository) ListByBusiness(businessID string, limit, offset int, since, until *time.Time) ([]*model.AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	query := `
		SELECT id, business_id, user_id, user_role, action, entity_type,
		       entity_id, before_state, after_state, reason, ip_address, user_agent, timestamp
		FROM audit_events
		WHERE business_id = $1`
	args := []interface{}{businessID}
	argIdx := 2

	if since != nil {
		query += fmt.Sprintf(" AND timestamp >= $%d", argIdx)
		args = append(args, *since)
		argIdx++
	}
	if until != nil {
		query += fmt.Sprintf(" AND timestamp <= $%d", argIdx)
		args = append(args, *until)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY timestamp DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit events by business: %w", err)
	}
	defer rows.Close()

	return scanAuditEvents(rows)
}

// ListByEntity returns audit events for a specific entity
func (r *AuditEventRepository) ListByEntity(entityType, entityID string, limit, offset int) ([]*model.AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	query := `
		SELECT id, business_id, user_id, user_role, action, entity_type,
		       entity_id, before_state, after_state, reason, ip_address, user_agent, timestamp
		FROM audit_events
		WHERE entity_type = $1 AND entity_id = $2
		ORDER BY timestamp DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.db.Query(query, entityType, entityID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list audit events by entity: %w", err)
	}
	defer rows.Close()

	return scanAuditEvents(rows)
}

// ListByUser returns audit events for a specific user
func (r *AuditEventRepository) ListByUser(userID string, limit, offset int) ([]*model.AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	query := `
		SELECT id, business_id, user_id, user_role, action, entity_type,
		       entity_id, before_state, after_state, reason, ip_address, user_agent, timestamp
		FROM audit_events
		WHERE user_id = $1
		ORDER BY timestamp DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.db.Query(query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list audit events by user: %w", err)
	}
	defer rows.Close()

	return scanAuditEvents(rows)
}

// scanAuditEvents iterates over rows and scans audit events
func scanAuditEvents(rows *sql.Rows) ([]*model.AuditEvent, error) {
	var events []*model.AuditEvent
	for rows.Next() {
		e := &model.AuditEvent{}
		var userID, ipAddr, userAgent sql.NullString
		var beforeState, afterState sql.NullString

		err := rows.Scan(
			&e.ID, &e.BusinessID, &userID, &e.UserRole, &e.Action, &e.EntityType,
			&e.EntityID, &beforeState, &afterState, &e.Reason, &ipAddr, &userAgent, &e.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}

		if userID.Valid {
			e.UserID = &userID.String
		}
		if ipAddr.Valid {
			e.IPAddress = &ipAddr.String
		}
		if userAgent.Valid {
			e.UserAgent = &userAgent.String
		}
		if beforeState.Valid {
			e.BeforeState = &beforeState.String
		}
		if afterState.Valid {
			e.AfterState = &afterState.String
		}

		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}

	return events, nil
}