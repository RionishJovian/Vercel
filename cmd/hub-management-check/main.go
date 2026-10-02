// Read-only production preflight. Never selects or changes customer credentials.
package main

import (
	"context"
	"fmt"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/secretbox"
	"os"
	"time"
)

func main() {
	if err := check(); err != nil {
		fmt.Fprintln(os.Stderr, "Management preflight failed:", err)
		os.Exit(1)
	}
	fmt.Println("Management preflight passed: encryption round-trip, tenant role/context, credential-column update privileges and empty foreign-tenant visibility. No customer writes.")
}
func check() error {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		return fmt.Errorf("configuration invalid")
	}
	box, err := secretbox.New(cfg.Provisioner.Applier.CredentialKey)
	if err != nil {
		return fmt.Errorf("credential cipher unavailable")
	}
	encrypted, err := box.Seal("non-secret-preflight-fixture")
	if err != nil {
		return fmt.Errorf("encryption failed")
	}
	plain, err := box.Open(encrypted)
	if err != nil || plain != "non-secret-preflight-fixture" {
		return fmt.Errorf("cipher round-trip failed")
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("database unavailable")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return database.WithTenant(tx, "00000000-0000-0000-0000-000000000000", func() error {
		for _, column := range []string{"management_ip", "ssh_port", "ssh_username", "ssh_secret_ciphertext", "ssh_secret_kind", "ssh_host_key_fingerprint", "updated_at"} {
			var allowed bool
			if err := tx.QueryRowContext(ctx, `SELECT has_column_privilege(current_user, 'public.devices', $1, 'UPDATE')`, column).Scan(&allowed); err != nil {
				return fmt.Errorf("privilege lookup failed")
			}
			if !allowed {
				return fmt.Errorf("tenant role lacks credential update permission")
			}
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM devices`).Scan(&count); err != nil {
			return fmt.Errorf("tenant visibility query failed")
		}
		if count != 0 {
			return fmt.Errorf("tenant RLS visibility failed")
		}
		return nil
	})
}
