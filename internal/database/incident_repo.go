package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mega-magic/internal/model"
)

// newIncidentID returns a fresh 32-char hex id (UUID-compatible column) for
// incidents created without a caller-supplied ID. Matches the evaluator/handler
// generator so an empty ID is never written.
func newIncidentID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// IncidentRepository handles incident database operations
type IncidentRepository struct {
	db *DB
}

// NewIncidentRepository creates a new incident repository
func NewIncidentRepository(db *DB) *IncidentRepository {
	return &IncidentRepository{db: db}
}

// incidentCols is the single source of truth for the incident column list, so
// every read path stays in lock-step with the positional Scan. suggested_action
// sits immediately before created_at/updated_at.
const incidentCols = `id, business_id, device_id, policy_id, kind, severity,
		       subject_id, subject_label, detail, opened_at,
		       acknowledged_at, acknowledged_by, resolved_at, resolved_by,
		       suggested_action,
		       created_at, updated_at`

// suggestedActionArg converts a typed pointer to a JSON argument for the jsonb
// column (nil when absent).
func suggestedActionArg(sa *model.SuggestedAction) interface{} {
	if sa == nil {
		return nil
	}
	b, err := json.Marshal(sa)
	if err != nil {
		return nil
	}
	return string(b)
}

// Create inserts a new incident record
func (r *IncidentRepository) Create(inc *model.Incident) (*model.Incident, error) {
	if strings.TrimSpace(inc.ID) == "" {
		inc.ID = newIncidentID()
	}
	query := `
		INSERT INTO incidents (id, business_id, device_id, policy_id, kind, severity,
		                       subject_id, subject_label, detail, opened_at,
		                       acknowledged_at, acknowledged_by, resolved_at, resolved_by,
		                       suggested_action)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING ` + incidentCols

	out := &model.Incident{}
	var deviceID, policyID, detail, acknowledgedBy, resolvedBy, suggested sql.NullString
	var acknowledgedAt, resolvedAt sql.NullTime

	err := r.db.QueryRow(query,
		inc.ID, inc.BusinessID, inc.DeviceID, inc.PolicyID,
		inc.Kind, inc.Severity, inc.SubjectID, inc.SubjectLabel, inc.Detail,
		inc.OpenedAt,
		inc.AcknowledgedAt, inc.AcknowledgedBy,
		inc.ResolvedAt, inc.ResolvedBy,
		suggestedActionArg(inc.SuggestedAction),
	).Scan(
		&out.ID, &out.BusinessID, &deviceID, &policyID,
		&out.Kind, &out.Severity, &out.SubjectID, &out.SubjectLabel, &detail,
		&out.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
		&suggested,
		&out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create incident: %w", err)
	}

	scanIncidentNulls(out, deviceID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy, suggested)

	return out, nil
}

// GetByID retrieves an incident by ID
func (r *IncidentRepository) GetByID(id string) (*model.Incident, error) {
	query := `SELECT ` + incidentCols + ` FROM incidents WHERE id = $1`

	inc := &model.Incident{}
	var deviceID, policyID, detail, acknowledgedBy, resolvedBy, suggested sql.NullString
	var acknowledgedAt, resolvedAt sql.NullTime

	err := r.db.QueryRow(query, id).Scan(
		&inc.ID, &inc.BusinessID, &deviceID, &policyID,
		&inc.Kind, &inc.Severity, &inc.SubjectID, &inc.SubjectLabel, &detail,
		&inc.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
		&suggested,
		&inc.CreatedAt, &inc.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get incident by id: %w", err)
	}

	scanIncidentNulls(inc, deviceID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy, suggested)

	return inc, nil
}

// GetByIDTenant retrieves an incident scoped to a business (never a bare-id
// cross-tenant read).
func (r *IncidentRepository) GetByIDTenant(businessID, id string) (*model.Incident, error) {
	query := `SELECT ` + incidentCols + ` FROM incidents WHERE id = $1 AND business_id = $2`

	inc := &model.Incident{}
	var deviceID, policyID, detail, acknowledgedBy, resolvedBy, suggested sql.NullString
	var acknowledgedAt, resolvedAt sql.NullTime

	err := r.db.QueryRow(query, id, businessID).Scan(
		&inc.ID, &inc.BusinessID, &deviceID, &policyID,
		&inc.Kind, &inc.Severity, &inc.SubjectID, &inc.SubjectLabel, &detail,
		&inc.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
		&suggested,
		&inc.CreatedAt, &inc.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get incident by id (tenant): %w", err)
	}

	scanIncidentNulls(inc, deviceID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy, suggested)
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
	query := `SELECT ` + incidentCols + ` FROM incidents
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
		var devID, policyID, detail, acknowledgedBy, resolvedBy, suggested sql.NullString
		var acknowledgedAt, resolvedAt sql.NullTime

		err := rows.Scan(
			&inc.ID, &inc.BusinessID, &devID, &policyID,
			&inc.Kind, &inc.Severity, &inc.SubjectID, &inc.SubjectLabel, &detail,
			&inc.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
			&suggested,
			&inc.CreatedAt, &inc.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}

		scanIncidentNulls(&inc, devID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy, suggested)
		incidents = append(incidents, inc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return incidents, nil
}

// ListByDevice returns all incidents for a specific device
func (r *IncidentRepository) ListByDevice(businessID, deviceID string) ([]model.Incident, error) {
	query := `SELECT ` + incidentCols + ` FROM incidents
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
		var devID, policyID, detail, acknowledgedBy, resolvedBy, suggested sql.NullString
		var acknowledgedAt, resolvedAt sql.NullTime

		err := rows.Scan(
			&inc.ID, &inc.BusinessID, &devID, &policyID,
			&inc.Kind, &inc.Severity, &inc.SubjectID, &inc.SubjectLabel, &detail,
			&inc.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
			&suggested,
			&inc.CreatedAt, &inc.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}

		scanIncidentNulls(&inc, devID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy, suggested)
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

// scanIncidentNulls populates nullable Incident fields from sql scan targets —
// the single place all read paths funnel through, so suggested_action cannot
// desync between them.
func scanIncidentNulls(inc *model.Incident, deviceID, policyID, detail sql.NullString, acknowledgedAt sql.NullTime, acknowledgedBy sql.NullString, resolvedAt sql.NullTime, resolvedBy sql.NullString, suggestedAction sql.NullString) {
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
	if suggestedAction.Valid && strings.TrimSpace(suggestedAction.String) != "" {
		var sa model.SuggestedAction
		if err := json.Unmarshal([]byte(suggestedAction.String), &sa); err == nil {
			inc.SuggestedAction = &sa
		}
	}
}

// ---------------------------------------------------------------------------
// Transaction-scoped variants (RLS enforcement rollout: operational alerting).
// Same SQL on the caller's *sql.Tx so they can run under WithTenant. Bare
// versions stay for the later full rollout.
// ---------------------------------------------------------------------------

// CreateTXN is the transaction-scoped variant of Create.
func (r *IncidentRepository) CreateTXN(tx *sql.Tx, inc *model.Incident) (*model.Incident, error) {
	query := `
		INSERT INTO incidents (id, business_id, device_id, policy_id, kind, severity,
		                       subject_id, subject_label, detail, opened_at,
		                       acknowledged_at, acknowledged_by, resolved_at, resolved_by,
		                       suggested_action)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING ` + incidentCols

	out := &model.Incident{}
	var deviceID, policyID, detail, acknowledgedBy, resolvedBy, suggested sql.NullString
	var acknowledgedAt, resolvedAt sql.NullTime

	err := tx.QueryRow(query,
		inc.ID, inc.BusinessID, inc.DeviceID, inc.PolicyID,
		inc.Kind, inc.Severity, inc.SubjectID, inc.SubjectLabel, inc.Detail,
		inc.OpenedAt, inc.AcknowledgedAt, inc.AcknowledgedBy, inc.ResolvedAt, inc.ResolvedBy,
		suggestedActionArg(inc.SuggestedAction),
	).Scan(
		&out.ID, &out.BusinessID, &deviceID, &policyID,
		&out.Kind, &out.Severity, &out.SubjectID, &out.SubjectLabel, &detail,
		&out.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
		&suggested,
		&out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create incident: %w", err)
	}
	scanIncidentNulls(out, deviceID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy, suggested)
	return out, nil
}

// GetByIDTXN is the transaction-scoped variant of GetByID.
func (r *IncidentRepository) GetByIDTXN(tx *sql.Tx, id string) (*model.Incident, error) {
	query := `SELECT ` + incidentCols + ` FROM incidents WHERE id = $1`

	inc := &model.Incident{}
	var deviceID, policyID, detail, acknowledgedBy, resolvedBy, suggested sql.NullString
	var acknowledgedAt, resolvedAt sql.NullTime

	err := tx.QueryRow(query, id).Scan(
		&inc.ID, &inc.BusinessID, &deviceID, &policyID,
		&inc.Kind, &inc.Severity, &inc.SubjectID, &inc.SubjectLabel, &detail,
		&inc.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
		&suggested,
		&inc.CreatedAt, &inc.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get incident by id: %w", err)
	}
	scanIncidentNulls(inc, deviceID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy, suggested)
	return inc, nil
}

// GetByIDTenantTXN is the tenant-scoped, transaction-scoped read used by the
// incident-action endpoint (business_id in the WHERE, never a bare-id read).
func (r *IncidentRepository) GetByIDTenantTXN(tx *sql.Tx, businessID, id string) (*model.Incident, error) {
	query := `SELECT ` + incidentCols + ` FROM incidents WHERE id = $1 AND business_id = $2`

	inc := &model.Incident{}
	var deviceID, policyID, detail, acknowledgedBy, resolvedBy, suggested sql.NullString
	var acknowledgedAt, resolvedAt sql.NullTime

	err := tx.QueryRow(query, id, businessID).Scan(
		&inc.ID, &inc.BusinessID, &deviceID, &policyID,
		&inc.Kind, &inc.Severity, &inc.SubjectID, &inc.SubjectLabel, &detail,
		&inc.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
		&suggested,
		&inc.CreatedAt, &inc.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get incident by id (tenant): %w", err)
	}
	scanIncidentNulls(inc, deviceID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy, suggested)
	return inc, nil
}

// HasOpenIncidentBySubjectTXN is the transaction-scoped variant of
// HasOpenIncidentBySubject.
func (r *IncidentRepository) HasOpenIncidentBySubjectTXN(tx *sql.Tx, businessID, subjectID, kind string) (bool, error) {
	var exists bool
	err := tx.QueryRow(
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

// ListOpenByBusinessTXN is the transaction-scoped variant of ListOpenByBusiness.
func (r *IncidentRepository) ListOpenByBusinessTXN(tx *sql.Tx, businessID string) ([]model.Incident, error) {
	query := `SELECT ` + incidentCols + ` FROM incidents
		WHERE business_id = $1 AND resolved_at IS NULL
		ORDER BY opened_at DESC`

	rows, err := tx.Query(query, businessID)
	if err != nil {
		return nil, fmt.Errorf("list open incidents by business: %w", err)
	}
	defer rows.Close()

	var incidents []model.Incident
	for rows.Next() {
		var inc model.Incident
		var devID, policyID, detail, acknowledgedBy, resolvedBy, suggested sql.NullString
		var acknowledgedAt, resolvedAt sql.NullTime
		if err := rows.Scan(
			&inc.ID, &inc.BusinessID, &devID, &policyID,
			&inc.Kind, &inc.Severity, &inc.SubjectID, &inc.SubjectLabel, &detail,
			&inc.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
			&suggested,
			&inc.CreatedAt, &inc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}
		scanIncidentNulls(&inc, devID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy, suggested)
		incidents = append(incidents, inc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return incidents, nil
}

// ListByDeviceTXN is the transaction-scoped variant of ListByDevice.
func (r *IncidentRepository) ListByDeviceTXN(tx *sql.Tx, businessID, deviceID string) ([]model.Incident, error) {
	query := `SELECT ` + incidentCols + ` FROM incidents
		WHERE business_id = $1 AND device_id = $2
		ORDER BY opened_at DESC`

	rows, err := tx.Query(query, businessID, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list incidents by device: %w", err)
	}
	defer rows.Close()

	var incidents []model.Incident
	for rows.Next() {
		var inc model.Incident
		var devID, policyID, detail, acknowledgedBy, resolvedBy, suggested sql.NullString
		var acknowledgedAt, resolvedAt sql.NullTime
		if err := rows.Scan(
			&inc.ID, &inc.BusinessID, &devID, &policyID,
			&inc.Kind, &inc.Severity, &inc.SubjectID, &inc.SubjectLabel, &detail,
			&inc.OpenedAt, &acknowledgedAt, &acknowledgedBy, &resolvedAt, &resolvedBy,
			&suggested,
			&inc.CreatedAt, &inc.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}
		scanIncidentNulls(&inc, devID, policyID, detail, acknowledgedAt, acknowledgedBy, resolvedAt, resolvedBy, suggested)
		incidents = append(incidents, inc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return incidents, nil
}

// AcknowledgeTXN is the transaction-scoped variant of Acknowledge.
func (r *IncidentRepository) AcknowledgeTXN(tx *sql.Tx, id, acknowledgedBy string) error {
	now := time.Now().UTC()
	result, err := tx.Exec(
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

// ResolveTXN is the transaction-scoped variant of Resolve.
func (r *IncidentRepository) ResolveTXN(tx *sql.Tx, id, resolvedBy string) error {
	now := time.Now().UTC()
	result, err := tx.Exec(
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
