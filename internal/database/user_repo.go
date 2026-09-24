package database

import (
	"database/sql"
	"fmt"
	"time"

	"mega-magic/internal/model"
)

// UserRepository handles user database operations
type UserRepository struct {
	db *DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{db: db}
}

// FindByEmail looks up a user by email
func (r *UserRepository) FindByEmail(email string) (*model.User, error) {
	query := `
		SELECT user_id, business_id, full_name, email, password_hash,
		       user_role, is_active, last_login_at, created_at, updated_at
		FROM users
		WHERE email = $1 AND is_active = true`

	u := &model.User{}
	var lastLogin sql.NullTime

	err := r.db.QueryRow(query, email).Scan(
		&u.UserID, &u.BusinessID, &u.FullName, &u.Email, &u.PasswordHash,
		&u.UserRole, &u.IsActive, &lastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find user by email: %w", err)
	}

	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}

	return u, nil
}

// FindByID looks up a user by ID
func (r *UserRepository) FindByID(userID string) (*model.User, error) {
	query := `
		SELECT user_id, business_id, full_name, email, password_hash,
		       user_role, is_active, last_login_at, created_at, updated_at
		FROM users
		WHERE user_id = $1 AND is_active = true`

	u := &model.User{}
	var lastLogin sql.NullTime

	err := r.db.QueryRow(query, userID).Scan(
		&u.UserID, &u.BusinessID, &u.FullName, &u.Email, &u.PasswordHash,
		&u.UserRole, &u.IsActive, &lastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}

	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}

	return u, nil
}

// Create registers a new user within a business
func (r *UserRepository) Create(businessID, fullName, email, passwordHash, userRole string) (*model.User, error) {
	query := `
		INSERT INTO users (business_id, full_name, email, password_hash, user_role)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING user_id, business_id, full_name, email, password_hash,
		          user_role, is_active, last_login_at, created_at, updated_at`

	u := &model.User{}
	var lastLogin sql.NullTime

	err := r.db.QueryRow(query, businessID, fullName, email, passwordHash, userRole).Scan(
		&u.UserID, &u.BusinessID, &u.FullName, &u.Email, &u.PasswordHash,
		&u.UserRole, &u.IsActive, &lastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return u, nil
}

// CreateTXN registers a new user within a business using an existing transaction
func (r *UserRepository) CreateTXN(tx *sql.Tx, businessID, fullName, email, passwordHash, userRole string) (*model.User, error) {
	query := `
		INSERT INTO users (business_id, full_name, email, password_hash, user_role)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING user_id, business_id, full_name, email, password_hash,
		          user_role, is_active, last_login_at, created_at, updated_at`

	u := &model.User{}
	var lastLogin sql.NullTime

	err := tx.QueryRow(query, businessID, fullName, email, passwordHash, userRole).Scan(
		&u.UserID, &u.BusinessID, &u.FullName, &u.Email, &u.PasswordHash,
		&u.UserRole, &u.IsActive, &lastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create user txn: %w", err)
	}

	return u, nil
}

// UpdateLastLogin sets the last_login_at timestamp
func (r *UserRepository) UpdateLastLogin(userID string) error {
	_, err := r.db.Exec("UPDATE users SET last_login_at = $1 WHERE user_id = $2", time.Now().UTC(), userID)
	return err
}

// FindByEmailTXN looks up a user by email with a specific transaction (for check-then-create race)
func (r *UserRepository) FindByEmailTXN(tx *sql.Tx, email string) (*model.User, error) {
	query := `
		SELECT user_id, business_id, full_name, email, password_hash,
		       user_role, is_active, last_login_at, created_at, updated_at
		FROM users
		WHERE email = $1`

	u := &model.User{}
	var lastLogin sql.NullTime

	err := tx.QueryRow(query, email).Scan(
		&u.UserID, &u.BusinessID, &u.FullName, &u.Email, &u.PasswordHash,
		&u.UserRole, &u.IsActive, &lastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find user by email txn: %w", err)
	}

	if lastLogin.Valid {
		u.LastLoginAt = &lastLogin.Time
	}

	return u, nil
}

// BeginTx starts a transaction
func (r *UserRepository) BeginTx() (*sql.Tx, error) {
	return r.db.Begin()
}