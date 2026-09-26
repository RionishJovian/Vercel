package database

import (
	"database/sql"
	"fmt"

	"mega-magic/internal/model"
)

// ---------------------------------------------------------------------------
// Platform (DEVELOPER) cross-tenant repositories.
//
// These are SYSTEM-LEVEL by design, exactly like the sweep worker's
// ListAllQuotas / ListAllPeerKeys: they run as raw r.db queries and do NOT go
// through WithTenant, because a DEVELOPER account legitimately sees across
// every business. This is the ONLY layer allowed to bypass tenant scoping, and
// every caller MUST be gated by RequireRole(RoleDeveloper). Never attach these
// methods to a caller-path that is reachable by a customer role.
// ---------------------------------------------------------------------------

// PlatformBusinessRepository provides system-level (cross-tenant) reads for the
// DEVELOPER platform view. It intentionally has no tenant constructor.
type PlatformBusinessRepository struct {
	db *DB
}

func NewPlatformBusinessRepository(db *DB) *PlatformBusinessRepository {
	return &PlatformBusinessRepository{db: db}
}

// ListAll returns every business across all tenants. SYSTEM-LEVEL.
func (r *PlatformBusinessRepository) ListAll() ([]model.Business, error) {
	rows, err := r.db.Query(
		`SELECT business_id, company_name, owner_email, access_tier, is_active,
		        device_quota_limit, created_at, updated_at
		 FROM businesses
		 ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("platform list businesses: %w", err)
	}
	defer rows.Close()

	var out []model.Business
	for rows.Next() {
		var b model.Business
		if err := rows.Scan(&b.BusinessID, &b.CompanyName, &b.OwnerEmail,
			&b.AccessTier, &b.IsActive, &b.DeviceQuotaLimit, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan business: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PlatformBusinessRepository) GetByID(businessID string) (*model.Business, error) {
	var b model.Business
	err := r.db.QueryRow(
		`SELECT business_id, company_name, owner_email, access_tier, is_active,
		        device_quota_limit, created_at, updated_at
		 FROM businesses WHERE business_id = $1`, businessID).
		Scan(&b.BusinessID, &b.CompanyName, &b.OwnerEmail, &b.AccessTier,
			&b.IsActive, &b.DeviceQuotaLimit, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("platform get business: %w", err)
	}
	return &b, nil
}

// ---------------------------------------------------------------------------
// Cross-tenant incident view for the DEVELOPER platform window.
// ---------------------------------------------------------------------------

// PlatformIncidentRepository provides system-level incident reads.
type PlatformIncidentRepository struct {
	db *DB
}

func NewPlatformIncidentRepository(db *DB) *PlatformIncidentRepository {
	return &PlatformIncidentRepository{db: db}
}

// ListOpenAll returns open incidents across every business. SYSTEM-LEVEL.
func (r *PlatformIncidentRepository) ListOpenAll() ([]model.Incident, error) {
	rows, err := r.db.Query(
		`SELECT id, business_id, device_id, policy_id, kind, severity,
		        subject_id, subject_label, detail, opened_at,
		        acknowledged_at, acknowledged_by, resolved_at, resolved_by,
		        created_at, updated_at
		 FROM incidents
		 WHERE resolved_at IS NULL
		 ORDER BY opened_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("platform list incidents: %w", err)
	}
	defer rows.Close()

	var out []model.Incident
	for rows.Next() {
		var inc model.Incident
		var deviceID, policyID, detail, ackdBy, resBy sql.NullString
		var ackdAt, resAt sql.NullTime
		if err := rows.Scan(&inc.ID, &inc.BusinessID, &deviceID, &policyID,
			&inc.Kind, &inc.Severity, &inc.SubjectID, &inc.SubjectLabel, &detail,
			&inc.OpenedAt, &ackdAt, &ackdBy, &resAt, &resBy, &inc.CreatedAt, &inc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}
		scanIncidentNulls(&inc, deviceID, policyID, detail, ackdAt, ackdBy, resAt, resBy)
		out = append(out, inc)
	}
	return out, rows.Err()
}

// CountAll returns per-status cross-tenant incident counts for the platform
// overview. SYSTEM-LEVEL.
func (r *PlatformIncidentRepository) CountAll() (model.PlatformIncidentCounts, error) {
	var counts model.PlatformIncidentCounts
	err := r.db.QueryRow(
		`SELECT
		   count(*) FILTER (WHERE resolved_at IS NULL) AS open,
		   count(*) FILTER (WHERE resolved_at IS NOT NULL) AS resolved
		 FROM incidents`).
		Scan(&counts.Open, &counts.Resolved)
	if err != nil {
		return counts, fmt.Errorf("platform count incidents: %w", err)
	}
	return counts, nil
}
