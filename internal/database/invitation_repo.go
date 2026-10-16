package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"mega-magic/internal/model"
)

const invitationColumns = `invitation_id, business_id, email, full_name, role,
    invited_by, expires_at, accepted_at, revoked_at, created_at`

func scanInvitation(row interface{ Scan(...interface{}) error }) (*model.BusinessInvitation, error) {
	i := &model.BusinessInvitation{}
	var accepted, revoked sql.NullTime
	if err := row.Scan(&i.InvitationID, &i.BusinessID, &i.Email, &i.FullName, &i.Role,
		&i.InvitedBy, &i.ExpiresAt, &accepted, &revoked, &i.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan invitation: %w", err)
	}
	if accepted.Valid {
		i.AcceptedAt = &accepted.Time
	}
	if revoked.Valid {
		i.RevokedAt = &revoked.Time
	}
	return i, nil
}

// CreateInvitationTXN stores only the token digest. The raw token is returned
// by the handler once and is never recoverable from the database.
func (r *UserRepository) CreateInvitationTXN(tx *sql.Tx, businessID, email, fullName, role, tokenDigest, invitedBy string, expiresAt time.Time) (*model.BusinessInvitation, error) {
	return scanInvitation(tx.QueryRow(
		`INSERT INTO business_invitations
         (business_id, email, full_name, role, token_hash, invited_by, expires_at)
         VALUES ($1,$2,$3,$4,$5,$6,$7)
         RETURNING `+invitationColumns,
		businessID, strings.ToLower(strings.TrimSpace(email)), strings.TrimSpace(fullName), role, tokenDigest, invitedBy, expiresAt.UTC()))
}

func (r *UserRepository) ListInvitationsTXN(tx *sql.Tx, businessID string) ([]*model.BusinessInvitation, error) {
	rows, err := tx.Query(`SELECT `+invitationColumns+` FROM business_invitations WHERE business_id=$1 ORDER BY created_at DESC`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer rows.Close()
	var out []*model.BusinessInvitation
	for rows.Next() {
		i, e := scanInvitation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *UserRepository) RevokeInvitationTXN(tx *sql.Tx, businessID, invitationID string) error {
	res, err := tx.Exec(`UPDATE business_invitations SET revoked_at=now() WHERE business_id=$1 AND invitation_id=$2 AND accepted_at IS NULL AND revoked_at IS NULL`, businessID, invitationID)
	if err != nil {
		return fmt.Errorf("revoke invitation: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *UserRepository) FindPendingInvitationTXN(tx *sql.Tx, tokenDigest string) (*model.BusinessInvitation, error) {
	return scanInvitation(tx.QueryRow(`SELECT `+invitationColumns+` FROM business_invitations WHERE token_hash=$1 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at>now() FOR UPDATE`, tokenDigest))
}

func (r *UserRepository) AcceptInvitationTXN(tx *sql.Tx, invitationID, businessID, fullName, email, passwordHash, role string) (*model.User, error) {
	u := &model.User{}
	var lastLogin sql.NullTime
	err := tx.QueryRow(`INSERT INTO users (business_id, full_name, email, password_hash, user_role)
        VALUES ($1,$2,$3,$4,$5)
        RETURNING user_id,business_id,full_name,email,password_hash,user_role,is_active,last_login_at,created_at,updated_at`,
		businessID, fullName, email, passwordHash, role).Scan(&u.UserID, &u.BusinessID, &u.FullName, &u.Email, &u.PasswordHash, &u.UserRole, &u.IsActive, &lastLogin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create invited user: %w", err)
	}
	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}
	if _, err = tx.Exec(`INSERT INTO business_memberships(business_id,user_id,role,status,created_at,updated_at) VALUES($1,$2,$3,'active',now(),now())`, businessID, u.UserID, role); err != nil {
		return nil, fmt.Errorf("activate membership: %w", err)
	}
	if _, err = tx.Exec(`UPDATE business_invitations SET accepted_at=now() WHERE invitation_id=$1 AND business_id=$2 AND accepted_at IS NULL AND revoked_at IS NULL`, invitationID, businessID); err != nil {
		return nil, fmt.Errorf("accept invitation: %w", err)
	}
	return u, nil
}
