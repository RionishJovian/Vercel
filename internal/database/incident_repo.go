package database

import (
	"database/sql"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

// IncidentRepository handles incident database operations
type IncidentRepository struct {
	db *DB
}

// NewIncidentRepository creates a new incident repository
func NewIncidentRepository(db *DB) *IncidentRepository {
	return &IncidentRepository{db: db}
}

// Create inserts a new incident record
func (r *IncidentRepository) Create(inc *model.Incident) (*model.Incident, error) {
	query := `
		INSERT INTO incidents (id, business_id, device_id, policy_id, kind, severity,
		                       subject_id, subject_label, detail, opened_at,
		                       acknowledged_at, acknowledged_by, resolved_at, resolved_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, business_id, device_id, policy_id, kind, severity,
		          subject_id, subject_label, detail, opened_at,
		          acknowledged_at, acknowledged_by, resolved_at, resolved_by,
		          created_at, updated_at`

	out := &model.Incident{}
	var deviceID, policyID, detail, acknowledgedBy, resolvedBy sql.NullString
	var acknowledgedAt, resolvedAt sql.NullTime

	err := r.db.QueryRow(query,
		inc.ID, inc.BusinessID, inc.DeviceID, inc.PolicyID,
		inc.Kind, inc.Severity, inc.SubjectID, inc.SubjectLabel, inc.Detail,
		inc.OpenedAt,
		inc.AcknowledgedAt, inc.AcknowledgedBy,
		inc.ResolvedAt, inc.ResolvedBy,
	).Scan(
		&out.ID, &out.BusinessID, &deviceID, &policyID,
		&out.Kind, &out.Severity, &out.SubjectID, &out.SubjectLabel, &detail,
		&out.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
		&out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create incident: %w", err)
	}

	scanIncidentNulls(out, deviceID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy)

	return out, nil
}

// GetByID retrieves an incident by ID
func (r *IncidentRepository) GetByID(id string) (*model.Incident, error) {
	query := `
		SELECT id, business_id, device_id, policy_id, kind, severity,
		       subject_id, subject_label, detail, opened_at,
		       acknowledged_at, acknowledged_by, resolved_at, resolved_by,
		       created_at, updated_at
		FROM incidents
		WHERE id = $1`

	inc := &model.Incident{}
	var deviceID, policyID, detail, acknowledgedBy, resolvedBy sql.NullString
	var acknowledgedAt, resolvedAt sql.NullTime

	err := r.db.QueryRow(query, id).Scan(
		&inc.ID, &inc.BusinessID, &deviceID, &policyID,
		&inc.Kind, &inc.Severity, &inc.SubjectID, &inc.SubjectLabel, &detail,
		&inc.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
		&inc.CreatedAt, &inc.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get incident by id: %w", err)
	}

	scanIncidentNulls(inc, deviceID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy)

	return inc, nil
}

// HasOpenIncidentBySubject reports whether an unresolved incident already exists
// for the same business+subject+kind (used by the evaluator to dedupe alerts).
func (r *IncidentRepository) HasOpenIncidentBySubject(businessID, subjectID, kind string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(
		`SELECT EXISTS(
			SELECT 1 FROM incidents
			WHERE business_id = $1 AND subject_id = $2 AND kind = $3 AND resolved_at IS NULL
		)`,
		businessID, subjectID, kind,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check open incident: %w", err)
	}
	return exists, nil
}

// ListOpenByBusiness returns all open (unresolved) incidents for a business
func (r *IncidentRepository) ListOpenByBusiness(businessID string) ([]model.Incident, error) {
	query := `
		SELECT id, business_id, device_id, policy_id, kind, severity,
		       subject_id, subject_label, detail, opened_at,
		       acknowledged_at, acknowledged_by, resolved_at, resolved_by,
		       created_at, updated_at
		FROM incidents
		WHERE business_id = $1 AND resolved_at IS NULL
		ORDER BY opened_at DESC`

	rows, err := r.db.Query(query, businessID)
	if err != nil {
		return nil, fmt.Errorf("list open incidents by business: %w", err)
	}
	defer rows.Close()

	var incidents []model.Incident
	for rows.Next() {
		var inc model.Incident
		var devID, policyID, detail, acknowledgedBy, resolvedBy sql.NullString
		var acknowledgedAt, resolvedAt sql.NullTime

		err := rows.Scan(
			&inc.ID, &inc.BusinessID, &devID, &policyID,
			&inc.Kind, &inc.Severity, &inc.SubjectID, &inc.SubjectLabel, &detail,
			&inc.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
			&inc.CreatedAt, &inc.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}

		scanIncidentNulls(&inc, devID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy)
		incidents = append(incidents, inc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return incidents, nil
}

// ListByDevice returns all incidents for a specific device
func (r *IncidentRepository) ListByDevice(businessID, deviceID string) ([]model.Incident, error) {
	query := `
		SELECT id, business_id, device_id, policy_id, kind, severity,
		       subject_id, subject_label, detail, opened_at,
		       acknowledged_at, acknowledged_by, resolved_at, resolved_by,
		       created_at, updated_at
		FROM incidents
		WHERE business_id = $1 AND device_id = $2
		ORDER BY opened_at DESC`

	rows, err := r.db.Query(query, businessID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list incidents by device: %w", err)
	}
	defer rows.Close()

	var incidents []model.Incident
	for rows.Next() {
		var inc model.Incident
		var devID, policyID, detail, acknowledgedBy, resolvedBy sql.NullString
		var acknowledgedAt, resolvedAt sql.NullTime

		err := rows.Scan(
			&inc.ID, &inc.BusinessID, &devID, &policyID,
			&inc.Kind, &inc.Severity, &inc.SubjectID, &inc.SubjectLabel, &detail,
			&inc.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
			&inc.CreatedAt, &inc.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}

		scanIncidentNulls(&inc, devID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy)
		incidents = append(incidents, inc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return incidents, nil
}

// Acknowledge marks an incident as acknowledged
func (r *IncidentRepository) Acknowledge(id, acknowledgedBy string) error {
	now := time.Now().UTC()
	result, err := r.db.Exec(
		`UPDATE incidents SET acknowledged_at = $1, acknowledged_by = $2, updated_at = $3 WHERE id = $4`,
		now, acknowledgedBy, now, id,
	)
	if err != nil {
		return fmt.Errorf("acknowledge incident: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("incident not found: %s", id)
	}
	return nil
}

// Resolve marks an incident as resolved
func (r *IncidentRepository) Resolve(id, resolvedBy string) error {
	now := time.Now().UTC()
	result, err := r.db.Exec(
		`UPDATE incidents SET resolved_at = $1, resolved_by = $2, updated_at = $3 WHERE id = $4`,
		now, resolvedBy, now, id,
	)
	if err != nil {
		return fmt.Errorf("resolve incident: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("incident not found: %s", id)
	}
	return nil
}

// scanIncidentNulls populates nullable Incident fields from sql scan targets
func scanIncidentNulls(inc *model.Incident, deviceID, policyID, detail sql.NullString, acknowledgedAt sql.NullTime, acknowledgedBy sql.NullString, resolvedAt sql.NullTime, resolvedBy sql.NullString) {
	if deviceID.Valid {
		inc.DeviceID = &deviceID.String
	}
	if policyID.Valid {
		inc.PolicyID = &policyID.String
	}
	if detail.Valid {
		inc.Detail = &detail.String
	}
	if acknowledgedAt.Valid {
		inc.AcknowledgedAt = &acknowledgedAt.Time
	}
	if acknowledgedBy.Valid {
		inc.AcknowledgedBy = &acknowledgedBy.String
	}
	if resolvedAt.Valid {
		inc.ResolvedAt = &resolvedAt.Time
	}
	if resolvedBy.Valid {
		inc.ResolvedBy = &resolvedBy.String
	}
}