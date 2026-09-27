package database

import (
	"database/sql"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

// PlatformAdminRepository — SYSTEM-LEVEL tenant administration used ONLY by the
// DEVELOPER/global-admin platform console. These run outside any tenant context
// (no WithTenant): the caller is the platform operator, and creating a tenant
// necessarily precedes there being a tenant to bind to. Every method is reached
// only through routes gated by RequireRole(DEVELOPER).

// TierDefaults maps an access tier to its device limit and AI-token seed.
// Mirrors the registration seed (ai_token_balance 50000) unless a tier overrides.
type TierDefaults struct {
	DeviceQuotaLimit int
	AITokenBalance   int
}

// TierDefaultsFor returns the per-tier defaults (device limit per the admin
// brief; AI tokens mirror the registration seed).
func TierDefaultsFor(tier string) TierDefaults {
	switch tier {
	case "ENTERPRISE_ISP":
		return TierDefaults{DeviceQuotaLimit: 100, AITokenBalance: 50000}
	case "GLOBAL_ADMIN":
		return TierDefaults{DeviceQuotaLimit: 99999, AITokenBalance: 50000}
	default: // SMALL_WISP
		return TierDefaults{DeviceQuotaLimit: 10, AITokenBalance: 50000}
	}
}

// CreateTenant creates a business + its OWNER user + support rows in ONE
// transaction. Any failure rolls the whole thing back — never a half tenant.
// Returns (businessID, ownerUserID). key = owner_email (duplicate => error).
func (r *PlatformBusinessRepository) CreateTenant(companyName, ownerEmail, passwordHash, accessTier string, deviceQuotaLimit int) (string, string, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return "", "", fmt.Errorf("begin tenant tx: %w", err)
	}
	defer tx.Rollback()

	// Duplicate owner email => conflict (businesses.owner_email is UNIQUE; we
	// pre-check for a clean 409 rather than a raw constraint error).
	var exists bool
	if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, ownerEmail).Scan(&exists); err != nil {
		return "", "", fmt.Errorf("check owner email: %w", err)
	}
	if exists {
		return "", "", ErrEmailInUse
	}

	def := TierDefaultsFor(accessTier)
	if deviceQuotaLimit <= 0 {
		deviceQuotaLimit = def.DeviceQuotaLimit
	}

	var businessID string
	err = tx.QueryRow(
		`INSERT INTO businesses (company_name, owner_email, access_tier, device_quota_limit, is_active)
		 VALUES ($1, $2, $3, $4, true)
		 RETURNING business_id`,
		companyName, ownerEmail, accessTier, deviceQuotaLimit,
	).Scan(&businessID)
	if err != nil {
		return "", "", fmt.Errorf("create business: %w", err)
	}

	// Same support rows the registration path creates, so a platform-created
	// tenant is indistinguishable from a self-registered one.
	if _, err := tx.Exec(
		`INSERT INTO nats_subjects (business_id, subject_prefix) VALUES ($1, $2)`,
		businessID, "tenant."+businessID[:8]+"."); err != nil {
		return "", "", fmt.Errorf("create nats subject: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO portal_configs (business_id, business_name) VALUES ($1, $2)`,
		businessID, companyName+" Wi-Fi"); err != nil {
		return "", "", fmt.Errorf("create portal config: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO business_metered_quotas (business_id, purchased_extra_slots, ai_token_balance)
		 VALUES ($1, 0, $2)`, businessID, def.AITokenBalance); err != nil {
		return "", "", fmt.Errorf("create quota: %w", err)
	}

	// Owner user (system-level: no tenant context exists until this commits).
	owner, err := NewUserRepository(r.db).CreateTXN(tx, businessID, companyName, ownerEmail, passwordHash, string(model.RoleOwner))
	if err != nil {
		return "", "", fmt.Errorf("create owner: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", "", fmt.Errorf("commit tenant: %w", err)
	}
	return businessID, owner.UserID, nil
}

// SetBusinessActive suspends/unsuspends a tenant. It ALSO flips the tenant's
// users.is_active, because the login path already refuses inactive users — this
// achieves "suspended tenant cannot log in" WITHOUT touching auth/login core.
func (r *PlatformBusinessRepository) SetBusinessActive(businessID string, active bool) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE businesses SET is_active=$2, updated_at=now() WHERE business_id=$1`, businessID, active)
	if err != nil {
		return fmt.Errorf("set business active: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTenantNotFound
	}
	if _, err := tx.Exec(`UPDATE users SET is_active=$2, updated_at=now() WHERE business_id=$1`, businessID, active); err != nil {
		return fmt.Errorf("set users active: %w", err)
	}
	return tx.Commit()
}

// ResetOwnerPassword sets a new bcrypt hash on the tenant's OWNER user. The
// plaintext is never stored or logged.
func (r *PlatformBusinessRepository) ResetOwnerPassword(businessID, passwordHash string) error {
	res, err := r.db.Exec(
		`UPDATE users SET password_hash=$2, updated_at=now()
		  WHERE business_id=$1 AND user_role='OWNER'`, businessID, passwordHash)
	if err != nil {
		return fmt.Errorf("reset owner password: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTenantNotFound
	}
	return nil
}

// TenantState is the per-tenant admin read.
type TenantState struct {
	BusinessID       string
	CompanyName      string
	AccessTier       string
	IsActive         bool
	OwnerEmail       string
	DeviceQuotaLimit int
	DeviceCount      int
	OpenIncidents    int
	PurchasedExtra   int
	AITokenBalance   int
}

// State returns a tenant's admin state in one place.
func (r *PlatformBusinessRepository) State(businessID string) (*TenantState, error) {
	s := &TenantState{}
	var ownerEmail sql.NullString
	err := r.db.QueryRow(
		`SELECT business_id, company_name, access_tier, is_active, owner_email, device_quota_limit
		   FROM businesses WHERE business_id=$1`, businessID,
	).Scan(&s.BusinessID, &s.CompanyName, &s.AccessTier, &s.IsActive, &ownerEmail, &s.DeviceQuotaLimit)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("tenant state: %w", err)
	}
	s.OwnerEmail = ownerEmail.String

	if err := r.db.QueryRow(`SELECT count(*) FROM devices WHERE business_id=$1`, businessID).Scan(&s.DeviceCount); err != nil {
		return nil, fmt.Errorf("tenant devices: %w", err)
	}
	if err := r.db.QueryRow(`SELECT count(*) FROM incidents WHERE business_id=$1 AND resolved_at IS NULL`, businessID).Scan(&s.OpenIncidents); err != nil {
		return nil, fmt.Errorf("tenant incidents: %w", err)
	}
	var extra, aiTokens sql.NullInt64
	_ = r.db.QueryRow(
		`SELECT purchased_extra_slots, ai_token_balance FROM business_metered_quotas WHERE business_id=$1`, businessID,
	).Scan(&extra, &aiTokens)
	s.PurchasedExtra = int(extra.Int64)
	s.AITokenBalance = int(aiTokens.Int64)
	return s, nil
}

// Sentinel errors for the handler to map to status codes.
var (
	ErrEmailInUse     = fmt.Errorf("email already in use")
	ErrTenantNotFound = fmt.Errorf("tenant not found")
)

var _ = time.Now
