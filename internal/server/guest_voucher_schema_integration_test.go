package server

import (
	"os"
	"testing"
)

func TestDB_GuestVoucherSchemaReadiness(t *testing.T) {
	db := subscriptionHTTPDatabase(t)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if guestVoucherSchemaReady(nil) == nil || guestVoucherSchemaReady(db) == nil {
		t.Fatal("missing guest schema accepted")
	}
	for _, path := range []string{"../../migrations/20261001200000_guest_voucher_grants.sql", "../../migrations/20261001210000_guest_access_recovery.sql", "../../migrations/20261002200000_guest_recovery_completion.sql", "../../migrations/20261021090000_guest_voucher_activation.sql"} {
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
	// Each mutation is only in this test's randomly named disposable database.
	for _, stmt := range []string{
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
