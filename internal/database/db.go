package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"mega-magic/internal/config"
)

// DB wraps sql.DB with helper methods
type DB struct {
	*sql.DB
}

// New creates a new database connection pool
func New(cfg config.DatabaseConfig) (*DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		quoteConnectionValue(cfg.Host), cfg.Port, quoteConnectionValue(cfg.User), quoteConnectionValue(cfg.Password), quoteConnectionValue(cfg.Name), quoteConnectionValue(cfg.SSLMode),
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{db}, nil
}

// lib/pq keyword values must be quoted, including empty passwords. Otherwise an
// empty value consumes the following keyword, and spaces or quotes can alter
// how the remainder of the connection settings are interpreted.
func quoteConnectionValue(value string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value) + "'"
}

// SetTenantContext sets the current_business_id session variable for RLS
func (db *DB) SetTenantContext(businessID string) error {
	_, err := db.Exec(fmt.Sprintf("SET SESSION app.current_business_id = '%s'", businessID))
	return err
}

// ClearTenantContext clears the app session variables
func (db *DB) ClearTenantContext() error {
	_, err := db.Exec("RESET SESSION app.current_business_id")
	return err
}

// Now returns the current time in UTC
func Now() time.Time {
	return time.Now().UTC()
}
