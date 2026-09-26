package database

import (
	"database/sql"
	"fmt"

	"mega-magic/internal/model"
)

// Team-read/update methods for the OWNER team-management API (C3). Every method
// is tenant-scoped (business_id in the WHERE clause) and runs on the caller's tx
// so it can run under WithTenant.

const teamUserCols = `user_id, business_id, full_name, email, user_role, is_active, last_login_at, created_at, updated_at`

func scanTeamUser(row interface{ Scan(...interface{}) error }) (*model.User, error) {
	u := &model.User{}
	var lastLogin sql.NullTime
	err := row.Scan(&u.UserID, &u.BusinessID, &u.FullName, &u.Email, &u.UserRole, &u.IsActive, &lastLogin, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan team user: %w", err)
	}
	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}
	return u, nil
}

// ListTeamByBusinessTXN returns every user in the tenant (password hash never
// selected).
func (r *UserRepository) ListTeamByBusinessTXN(tx *sql.Tx, businessID string) ([]*model.User, error) {
	rows, err := tx.Query(`SELECT `+teamUserCols+` FROM users WHERE business_id=$1 ORDER BY created_at`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list team: %w", err)
	}
	defer rows.Close()
	var out []*model.User
	for rows.Next() {
		u, err := scanTeamUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GetTeamUserTXN returns one team member, tenant-scoped (nil = not this tenant's).
func (r *UserRepository) GetTeamUserTXN(tx *sql.Tx, businessID, userID string) (*model.User, error) {
	return scanTeamUser(tx.QueryRow(
		`SELECT `+teamUserCols+` FROM users WHERE user_id=$1 AND business_id=$2`, userID, businessID))
}

// UpdateTeamUserTXN updates a team member's name/role/active flag, tenant-scoped.
func (r *UserRepository) UpdateTeamUserTXN(tx *sql.Tx, businessID, userID, fullName, role string, isActive bool) (*model.User, error) {
	return scanTeamUser(tx.QueryRow(
		`UPDATE users SET full_name=$3, user_role=$4, is_active=$5, updated_at=now()
		  WHERE user_id=$1 AND business_id=$2
		  RETURNING `+teamUserCols, userID, businessID, fullName, role, isActive))
}

// SetTeamActiveTXN enables/disables a team member, tenant-scoped.
func (r *UserRepository) SetTeamActiveTXN(tx *sql.Tx, businessID, userID string, active bool) error {
	res, err := tx.Exec(`UPDATE users SET is_active=$3, updated_at=now() WHERE user_id=$1 AND business_id=$2`,
		userID, businessID, active)
	if err != nil {
		return fmt.Errorf("set team active: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("team member not found in tenant")
	}
	return nil
}

// CountActiveOwnersTXN counts active OWNERs in the tenant — the guard that
// prevents demoting/removing the last owner.
func (r *UserRepository) CountActiveOwnersTXN(tx *sql.Tx, businessID string) (int, error) {
	var n int
	if err := tx.QueryRow(
		`SELECT count(*) FROM users WHERE business_id=$1 AND user_role='OWNER' AND is_active=true`, businessID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count owners: %w", err)
	}
	return n, nil
}

// CreateTeamUserTXN inserts a team member with a password hash, tenant-scoped.
func (r *UserRepository) CreateTeamUserTXN(tx *sql.Tx, businessID, fullName, email, passwordHash, role string) (*model.User, error) {
	return scanTeamUser(tx.QueryRow(
		`INSERT INTO users (business_id, full_name, email, password_hash, user_role)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+teamUserCols, businessID, fullName, email, passwordHash, role))
}
