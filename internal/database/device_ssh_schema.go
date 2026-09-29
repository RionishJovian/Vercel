package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

var requiredDeviceSSHColumns = []string{
	"ssh_port",
	"ssh_username",
	"ssh_secret_ciphertext",
	"ssh_secret_kind",
	"ssh_host_key_fingerprint",
}

var requiredDeviceSSHConstraints = map[string][]string{
	"devices_ssh_creds_complete": {"ssh_host_key_fingerprint", "SHA256:"},
	"devices_ssh_port_valid":     {"ssh_port", "65535"},
}

// CheckDeviceSSHSchema is the deploy-ready compatibility gate. It verifies the
// exact security-bearing definition markers and requires both constraints to
// be validated, so stage-one migration state can never pass deployment.
func CheckDeviceSSHSchema(ctx context.Context, db *sql.DB) error {
	return checkDeviceSSHSchema(ctx, db, true)
}

// CheckDeviceSSHStageOne accepts correctly defined NOT VALID constraints while
// operators enroll legacy fingerprints. It is for migration tooling only, not
// deployment preflight.
func CheckDeviceSSHStageOne(ctx context.Context, db *sql.DB) error {
	return checkDeviceSSHSchema(ctx, db, false)
}

func checkDeviceSSHSchema(ctx context.Context, db *sql.DB, requireValidated bool) error {
	if db == nil {
		return fmt.Errorf("device SSH schema check: nil database")
	}
	var missing []string
	for _, column := range requiredDeviceSSHColumns {
		var exists bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1
				  FROM information_schema.columns
				 WHERE table_schema = 'public'
				   AND table_name = 'devices'
				   AND column_name = $1
			)`, column).Scan(&exists); err != nil {
			return fmt.Errorf("device SSH schema check column %s: %w", column, err)
		}
		if !exists {
			missing = append(missing, "column "+column)
		}
	}
	for constraint, markers := range requiredDeviceSSHConstraints {
		var definition string
		var validated bool
		err := db.QueryRowContext(ctx, `
			SELECT pg_get_constraintdef(oid), convalidated
			  FROM pg_constraint
			 WHERE conrelid = 'public.devices'::regclass
			   AND conname = $1`, constraint).Scan(&definition, &validated)
		if err == sql.ErrNoRows {
			missing = append(missing, "constraint "+constraint)
			continue
		}
		if err != nil {
			return fmt.Errorf("device SSH schema check constraint %s: %w", constraint, err)
		}
		for _, marker := range markers {
			if !strings.Contains(definition, marker) {
				missing = append(missing, "expected definition for constraint "+constraint)
				break
			}
		}
		if requireValidated && !validated {
			missing = append(missing, "validated constraint "+constraint)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("device SSH schema is not deploy-ready; missing %s", strings.Join(missing, ", "))
	}
	return nil
}

// ValidateDeviceSSHConstraints completes stage two only after every legacy row
// has either received a pinned fingerprint or had incomplete credentials
// cleared. PostgreSQL validates without rewriting the table.
func ValidateDeviceSSHConstraints(ctx context.Context, db *sql.DB) error {
	if err := CheckDeviceSSHStageOne(ctx, db); err != nil {
		return err
	}
	var invalid int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		  FROM public.devices
		 WHERE NOT (
			(ssh_username IS NULL AND ssh_secret_ciphertext IS NULL
			 AND ssh_secret_kind IS NULL AND ssh_host_key_fingerprint IS NULL)
		 OR (ssh_username IS NOT NULL AND ssh_secret_ciphertext IS NOT NULL
			 AND ssh_secret_kind IN ('password', 'key')
			 AND ssh_host_key_fingerprint IS NOT NULL
			 AND ssh_host_key_fingerprint ~ '^SHA256:[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]$')
		 )`).Scan(&invalid); err != nil {
		return fmt.Errorf("audit device SSH credentials: %w", err)
	}
	if invalid != 0 {
		return fmt.Errorf("cannot validate device SSH constraints: %d legacy row(s) still need fingerprint enrollment or credential cleanup", invalid)
	}
	if _, err := db.ExecContext(ctx, `
		ALTER TABLE public.devices VALIDATE CONSTRAINT devices_ssh_creds_complete;
		ALTER TABLE public.devices VALIDATE CONSTRAINT devices_ssh_port_valid;
	`); err != nil {
		return fmt.Errorf("validate device SSH constraints: %w", err)
	}
	return nil
}
