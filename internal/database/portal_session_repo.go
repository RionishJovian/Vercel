package database

import (
	"database/sql"
	"fmt"
	"net"

	"mega-magic/internal/model"
)

// PortalSessionRepository handles portal session database operations
type PortalSessionRepository struct {
	db *DB
}

// NewPortalSessionRepository creates a new portal session repository
func NewPortalSessionRepository(db *DB) *PortalSessionRepository {
	return &PortalSessionRepository{db: db}
}

// Create inserts a new portal session
func (r *PortalSessionRepository) Create(session *model.PortalSession) (*model.PortalSession, error) {
	query := `
		INSERT INTO portal_sessions (business_id, device_id, voucher_id, client_mac, client_ip,
		                             authenticated_at, expires_at, is_active, consent_given, consent_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, business_id, device_id, voucher_id, client_mac, client_ip,
		          authenticated_at, expires_at, is_active, consent_given, consent_at`

	s := &model.PortalSession{}
	var deviceID, voucherID sql.NullString
	var clientIP sql.NullString
	var expiresAt, consentAt sql.NullTime

	err := r.db.QueryRow(query,
		session.BusinessID, session.DeviceID, session.VoucherID, session.ClientMAC,
		session.ClientIP, session.AuthenticatedAt, session.ExpiresAt,
		session.IsActive, session.ConsentGiven, session.ConsentAt,
	).Scan(
		&s.ID, &s.BusinessID, &deviceID, &voucherID, &s.ClientMAC, &clientIP,
		&s.AuthenticatedAt, &expiresAt, &s.IsActive, &s.ConsentGiven, &consentAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create portal session: %w", err)
	}

	if deviceID.Valid {
		s.DeviceID = &deviceID.String
	}
	if voucherID.Valid {
		s.VoucherID = &voucherID.String
	}
	if clientIP.Valid {
		parsedIP := net.ParseIP(clientIP.String)
		s.ClientIP = &parsedIP
	}
	if expiresAt.Valid {
		s.ExpiresAt = &expiresAt.Time
	}
	if consentAt.Valid {
		s.ConsentAt = &consentAt.Time
	}

	return s, nil
}

// GetActiveByMAC returns the active session for a given client MAC address
func (r *PortalSessionRepository) GetActiveByMAC(clientMAC string) (*model.PortalSession, error) {
	query := `
		SELECT id, business_id, device_id, voucher_id, client_mac, client_ip,
		       authenticated_at, expires_at, is_active, consent_given, consent_at
		FROM portal_sessions
		WHERE client_mac = $1 AND is_active = true`

	s := &model.PortalSession{}
	var deviceID, voucherID sql.NullString
	var clientIP sql.NullString
	var expiresAt, consentAt sql.NullTime

	err := r.db.QueryRow(query, clientMAC).Scan(
		&s.ID, &s.BusinessID, &deviceID, &voucherID, &s.ClientMAC, &clientIP,
		&s.AuthenticatedAt, &expiresAt, &s.IsActive, &s.ConsentGiven, &consentAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active session by mac: %w", err)
	}

	if deviceID.Valid {
		s.DeviceID = &deviceID.String
	}
	if voucherID.Valid {
		s.VoucherID = &voucherID.String
	}
	if clientIP.Valid {
		parsedIP := net.ParseIP(clientIP.String)
		s.ClientIP = &parsedIP
	}
	if expiresAt.Valid {
		s.ExpiresAt = &expiresAt.Time
	}
	if consentAt.Valid {
		s.ConsentAt = &consentAt.Time
	}

	return s, nil
}

// ListActiveByBusiness returns all active sessions for a business
func (r *PortalSessionRepository) ListActiveByBusiness(businessID string) ([]*model.PortalSession, error) {
	query := `
		SELECT id, business_id, device_id, voucher_id, client_mac, client_ip,
		       authenticated_at, expires_at, is_active, consent_given, consent_at
		FROM portal_sessions
		WHERE business_id = $1 AND is_active = true
		ORDER BY authenticated_at DESC`

	rows, err := r.db.Query(query, businessID)
	if err != nil {
		return nil, fmt.Errorf("list active sessions by business: %w", err)
	}
	defer rows.Close()

	var sessions []*model.PortalSession
	for rows.Next() {
		s := &model.PortalSession{}
		var deviceID, voucherID sql.NullString
		var clientIP sql.NullString
		var expiresAt, consentAt sql.NullTime

		err := rows.Scan(
			&s.ID, &s.BusinessID, &deviceID, &voucherID, &s.ClientMAC, &clientIP,
			&s.AuthenticatedAt, &expiresAt, &s.IsActive, &s.ConsentGiven, &consentAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan portal session row: %w", err)
		}

		if deviceID.Valid {
			s.DeviceID = &deviceID.String
		}
		if voucherID.Valid {
			s.VoucherID = &voucherID.String
		}
		if clientIP.Valid {
			parsedIP := net.ParseIP(clientIP.String)
			s.ClientIP = &parsedIP
		}
		if expiresAt.Valid {
			s.ExpiresAt = &expiresAt.Time
		}
		if consentAt.Valid {
			s.ConsentAt = &consentAt.Time
		}

		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate portal session rows: %w", err)
	}

	return sessions, nil
}

// DeactivateSession marks a session as inactive
func (r *PortalSessionRepository) DeactivateSession(id string) error {
	result, err := r.db.Exec(
		"UPDATE portal_sessions SET is_active = false WHERE id = $1 AND is_active = true",
		id,
	)
	if err != nil {
		return fmt.Errorf("deactivate session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("deactivate session rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("deactivate session: no active session found with id %s", id)
	}

	return nil
}

// GetByID returns a session by its ID
func (r *PortalSessionRepository) GetByID(id string) (*model.PortalSession, error) {
	query := `
		SELECT id, business_id, device_id, voucher_id, client_mac, client_ip,
		       authenticated_at, expires_at, is_active, consent_given, consent_at
		FROM portal_sessions
		WHERE id = $1`

	s := &model.PortalSession{}
	var deviceID, voucherID sql.NullString
	var clientIP sql.NullString
	var expiresAt, consentAt sql.NullTime

	err := r.db.QueryRow(query, id).Scan(
		&s.ID, &s.BusinessID, &deviceID, &voucherID, &s.ClientMAC, &clientIP,
		&s.AuthenticatedAt, &expiresAt, &s.IsActive, &s.ConsentGiven, &consentAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session by id: %w", err)
	}

	if deviceID.Valid {
		s.DeviceID = &deviceID.String
	}
	if voucherID.Valid {
		s.VoucherID = &voucherID.String
	}
	if clientIP.Valid {
		parsedIP := net.ParseIP(clientIP.String)
		s.ClientIP = &parsedIP
	}
	if expiresAt.Valid {
		s.ExpiresAt = &expiresAt.Time
	}
	if consentAt.Valid {
		s.ConsentAt = &consentAt.Time
	}

	return s, nil
}

// ---------------------------------------------------------------------------
// Transaction-scoped variants (RLS enforcement rollout: portal sessions). Same
// SQL on the caller's *sql.Tx so they can run under WithTenant. Bare versions
// stay for the later full rollout.
//
// NOTE: GetActiveByMAC is deliberately NOT converted — it looks a session up by
// client MAC with NO tenant context (the anonymous portal-redemption flow).
// RLS-scoping it with a fabricated business would be wrong; it stays a
// system-level lookup.
// ---------------------------------------------------------------------------

// CreateTXN is the transaction-scoped variant of Create.
func (r *PortalSessionRepository) CreateTXN(tx *sql.Tx, session *model.PortalSession) (*model.PortalSession, error) {
	query := `
		INSERT INTO portal_sessions (business_id, device_id, voucher_id, client_mac, client_ip,
		                             authenticated_at, expires_at, is_active, consent_given, consent_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, business_id, device_id, voucher_id, client_mac, client_ip,
		          authenticated_at, expires_at, is_active, consent_given, consent_at`

	s := &model.PortalSession{}
	var deviceID, voucherID sql.NullString
	var clientIP sql.NullString
	var expiresAt, consentAt sql.NullTime

	err := tx.QueryRow(query,
		session.BusinessID, session.DeviceID, session.VoucherID, session.ClientMAC,
		session.ClientIP, session.AuthenticatedAt, session.ExpiresAt,
		session.IsActive, session.ConsentGiven, session.ConsentAt,
	).Scan(
		&s.ID, &s.BusinessID, &deviceID, &voucherID, &s.ClientMAC, &clientIP,
		&s.AuthenticatedAt, &expiresAt, &s.IsActive, &s.ConsentGiven, &consentAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create portal session: %w", err)
	}
	if deviceID.Valid {
		s.DeviceID = &deviceID.String
	}
	if voucherID.Valid {
		s.VoucherID = &voucherID.String
	}
	if clientIP.Valid {
		parsedIP := net.ParseIP(clientIP.String)
		s.ClientIP = &parsedIP
	}
	if expiresAt.Valid {
		s.ExpiresAt = &expiresAt.Time
	}
	if consentAt.Valid {
		s.ConsentAt = &consentAt.Time
	}
	return s, nil
}

// ListActiveByBusinessTXN is the transaction-scoped variant of ListActiveByBusiness.
func (r *PortalSessionRepository) ListActiveByBusinessTXN(tx *sql.Tx, businessID string) ([]*model.PortalSession, error) {
	query := `
		SELECT id, business_id, device_id, voucher_id, client_mac, client_ip,
		       authenticated_at, expires_at, is_active, consent_given, consent_at
		FROM portal_sessions
		WHERE business_id = $1 AND is_active = true
		ORDER BY authenticated_at DESC`

	rows, err := tx.Query(query, businessID)
	if err != nil {
		return nil, fmt.Errorf("list active sessions by business: %w", err)
	}
	defer rows.Close()

	var sessions []*model.PortalSession
	for rows.Next() {
		s := &model.PortalSession{}
		var deviceID, voucherID sql.NullString
		var clientIP sql.NullString
		var expiresAt, consentAt sql.NullTime
		if err := rows.Scan(
			&s.ID, &s.BusinessID, &deviceID, &voucherID, &s.ClientMAC, &clientIP,
			&s.AuthenticatedAt, &expiresAt, &s.IsActive, &s.ConsentGiven, &consentAt,
		); err != nil {
			return nil, fmt.Errorf("scan portal session row: %w", err)
		}
		if deviceID.Valid {
			s.DeviceID = &deviceID.String
		}
		if voucherID.Valid {
			s.VoucherID = &voucherID.String
		}
		if clientIP.Valid {
			parsedIP := net.ParseIP(clientIP.String)
			s.ClientIP = &parsedIP
		}
		if expiresAt.Valid {
			s.ExpiresAt = &expiresAt.Time
		}
		if consentAt.Valid {
			s.ConsentAt = &consentAt.Time
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate portal session rows: %w", err)
	}
	return sessions, nil
}

// DeactivateSessionTXN is the transaction-scoped variant of DeactivateSession.
func (r *PortalSessionRepository) DeactivateSessionTXN(tx *sql.Tx, id string) error {
	result, err := tx.Exec(
		"UPDATE portal_sessions SET is_active = false WHERE id = $1 AND is_active = true",
		id,
	)
	if err != nil {
		return fmt.Errorf("deactivate session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("deactivate session rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("deactivate session: no active session found with id %s", id)
	}
	return nil
}

// GetByIDTXN is the transaction-scoped variant of GetByID.
func (r *PortalSessionRepository) GetByIDTXN(tx *sql.Tx, id string) (*model.PortalSession, error) {
	query := `
		SELECT id, business_id, device_id, voucher_id, client_mac, client_ip,
		       authenticated_at, expires_at, is_active, consent_given, consent_at
		FROM portal_sessions
		WHERE id = $1`

	s := &model.PortalSession{}
	var deviceID, voucherID sql.NullString
	var clientIP sql.NullString
	var expiresAt, consentAt sql.NullTime

	err := tx.QueryRow(query, id).Scan(
		&s.ID, &s.BusinessID, &deviceID, &voucherID, &s.ClientMAC, &clientIP,
		&s.AuthenticatedAt, &expiresAt, &s.IsActive, &s.ConsentGiven, &consentAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session by id: %w", err)
	}
	if deviceID.Valid {
		s.DeviceID = &deviceID.String
	}
	if voucherID.Valid {
		s.VoucherID = &voucherID.String
	}
	if clientIP.Valid {
		parsedIP := net.ParseIP(clientIP.String)
		s.ClientIP = &parsedIP
	}
	if expiresAt.Valid {
		s.ExpiresAt = &expiresAt.Time
	}
	if consentAt.Valid {
		s.ConsentAt = &consentAt.Time
	}
	return s, nil
}
