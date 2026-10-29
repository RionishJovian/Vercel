package server

import (
	"os"
	"strings"
	"testing"
)

func TestDB_GuestVoucherSchemaReadiness(t *testing.T) {
	db := subscriptionHTTPDatabase(t)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if guestVoucherSchemaReady(nil) == nil || guestVoucherSchemaReady(db) == nil {
		t.Fatal("missing guest schema accepted")
	}
	for _, path := range []string{"../../migrations/20261001200000_guest_voucher_grants.sql", "../../migrations/20261001210000_guest_access_recovery.sql", "../../migrations/20261002200000_guest_recovery_completion.sql", "../../migrations/20261021090000_guest_voucher_activation.sql", "../../migrations/20261021180000_guest_voucher_issue_batches.sql"} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(b)); err != nil {
			t.Fatal(err)
		}
	}
	if err := guestVoucherSchemaReady(db); err != nil {
		t.Fatal(err)
	}
	// The serving login has explicit service-only RLS policies instead of
	// SUPERUSER/BYPASSRLS. Tenant-visible or unexpected policies remain refused.
	t.Run("validated runtime service policy", func(t *testing.T) {
		b, err := os.ReadFile("../../deploy/runtime-db-role.sql")
		if err != nil {
			t.Fatal(err)
		}
		script := strings.ReplaceAll(strings.ReplaceAll(string(b), "\nBEGIN;", ""), "\nCOMMIT;", "")
		if _, err = db.Exec("BEGIN;\n" + script); err != nil {
			t.Fatal(err)
		}
		defer db.Exec("ROLLBACK")
		if err = guestVoucherSchemaReady(db); err != nil {
			t.Fatalf("service role rejected: %v", err)
		}
		for _, mutation := range []string{
			`ALTER POLICY mm_runtime_service ON guest_voucher_grants TO authenticated`,
			`ALTER POLICY mm_runtime_service ON guest_voucher_grants USING (business_id IS NOT NULL)`,
			`ALTER ROLE mega_magic_runtime BYPASSRLS`,
			`ALTER ROLE mega_magic_runtime INHERIT`,
			`CREATE POLICY unexpected_service_policy ON guest_access_recovery USING(true)`,
			`REVOKE authenticated FROM mega_magic_runtime; GRANT mega_magic_runtime TO authenticated`,
		} {
			if _, err = db.Exec("SAVEPOINT runtime_policy_mutation; " + mutation); err != nil {
				t.Fatal(err)
			}
			if err = guestVoucherSchemaReady(db); err == nil {
				t.Fatalf("unsafe service policy accepted: %s", mutation)
			}
			if _, err = db.Exec("ROLLBACK TO SAVEPOINT runtime_policy_mutation"); err != nil {
				t.Fatal(err)
			}
		}
	})
	// Each mutation is only in this test's randomly named disposable database.
	for _, stmt := range []string{
		`ALTER TABLE guest_voucher_issue_batches NO FORCE ROW LEVEL SECURITY`,
		`ALTER TABLE guest_voucher_issue_items DISABLE TRIGGER guest_voucher_issue_item_guard`,
		`ALTER TABLE guest_voucher_grants DROP COLUMN activated_at`,
		`ALTER TABLE guest_voucher_grants DISABLE TRIGGER guest_voucher_activation_guard`,
		`ALTER TABLE guest_access_recovery NO FORCE ROW LEVEL SECURITY`,
		`ALTER TABLE guest_voucher_grants DISABLE ROW LEVEL SECURITY`,
		`ALTER TABLE guest_access_recovery DISABLE TRIGGER guest_access_recovery_guard`,
		`DROP TRIGGER guest_voucher_snapshot_guard ON guest_voucher_grants`,
		`DROP POLICY guest_access_recovery_tenant ON guest_access_recovery`,
		`CREATE POLICY unexpected_policy ON guest_access_recovery USING(true)`,
		`ALTER TABLE guest_access_recovery DROP COLUMN window_started_at`,
		`ALTER TABLE guest_access_recovery DROP COLUMN completed_transfer_id`,
		`REVOKE UPDATE ON guest_access_recovery FROM authenticated`,
	} {
		t.Run(stmt, func(t *testing.T) {
			if _, err := db.Exec("BEGIN; " + stmt); err != nil {
				t.Fatal(err)
			}
			// Single pool connection keeps the uncommitted schema visible to the
			// preflight. Roll back even if its assertion fails.
			defer db.Exec("ROLLBACK")
			if err := guestVoucherSchemaReady(db); err == nil {
				t.Fatal("incomplete protections accepted")
			}
		})
	}
}

func TestDB_GuestVoucherBatchMigrationRollsBackAtomically(t *testing.T) {
	db := subscriptionHTTPDatabase(t)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	base, err := os.ReadFile("../../migrations/20261001200000_guest_voucher_grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(base)); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/20261021180000_guest_voucher_issue_batches.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Put the migration's DDL in a caller-owned transaction, then deliberately
	// abort it. A failed rollout must leave neither half-created batch tables
	// nor a half-installed guard. This database is disposable and local only.
	ddl := strings.ReplaceAll(strings.ReplaceAll(string(migration), "\nBEGIN;", ""), "\nCOMMIT;", "")
	if _, err := db.Exec("BEGIN;\n" + ddl + "\nSELECT 1/0;"); err == nil {
		t.Fatal("deliberately failed migration unexpectedly succeeded")
	}
	if _, err := db.Exec("ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	var absent bool
	if err := db.QueryRow(`SELECT to_regclass('public.guest_voucher_issue_batches') IS NULL AND to_regclass('public.guest_voucher_issue_items') IS NULL`).Scan(&absent); err != nil || !absent {
		t.Fatalf("failed migration left partial schema: absent=%v err=%v", absent, err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(string(migration)); err != nil {
			t.Fatalf("idempotent migration attempt %d: %v", i+1, err)
		}
	}
}
