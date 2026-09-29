package database

import (
	"context"
	"os"
	"testing"
)

// TestDB_DeviceSSHMigration_preservesLegacyRows proves the production upgrade
// contract against the throwaway PostgreSQL harness:
//   - a legacy credential row without a fingerprint does not abort migration;
//   - new incomplete rows are rejected immediately;
//   - the migration can be applied twice before and after validation;
//   - deployment readiness requires the exact validated constraints.
func TestDB_DeviceSSHMigration_preservesLegacyRows(t *testing.T) {
	db := requireHarness(t)
	businessID := insertBusiness(t, db)

	// Recreate the pre-migration condition deliberately. The throwaway schema is
	// current, so remove only the constraint whose rollout behavior we test.
	if _, err := db.Exec(`ALTER TABLE devices DROP CONSTRAINT IF EXISTS devices_ssh_creds_complete`); err != nil {
		t.Fatalf("drop current credential constraint: %v", err)
	}

	var legacyDeviceID string
	if err := db.QueryRow(`
		INSERT INTO devices (
			business_id, hardware_name, management_ip,
			ssh_username, ssh_secret_ciphertext, ssh_secret_kind
		) VALUES ($1, 'legacy-unpinned-router', '10.99.0.9', 'admin', 'legacy-ciphertext', 'password')
		RETURNING device_id`, businessID).Scan(&legacyDeviceID); err != nil {
		t.Fatalf("seed legacy credential row: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM businesses WHERE business_id = $1`, businessID)
		_, _ = db.Exec(`ALTER TABLE devices VALIDATE CONSTRAINT devices_ssh_creds_complete`)
		_, _ = db.Exec(`ALTER TABLE devices VALIDATE CONSTRAINT devices_ssh_port_valid`)
	})

	migration, err := os.ReadFile("../../migrations/20260929120000_device_ssh_host_key_pinning.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatalf("apply migration over legacy row: %v", err)
	}

	var stillPresent bool
	if err := db.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM devices
		 WHERE device_id = $1
		   AND ssh_username = 'admin'
		   AND ssh_host_key_fingerprint IS NULL
	)`, legacyDeviceID).Scan(&stillPresent); err != nil {
		t.Fatalf("check preserved row: %v", err)
	}
	if !stillPresent {
		t.Fatal("migration deleted or rewrote the legacy credential row")
	}

	var validated bool
	if err := db.QueryRow(`
		SELECT convalidated
		  FROM pg_constraint
		 WHERE conrelid = 'devices'::regclass
		   AND conname = 'devices_ssh_creds_complete'`).Scan(&validated); err != nil {
		t.Fatalf("read constraint state: %v", err)
	}
	if validated {
		t.Fatal("credential constraint was validated despite a known legacy violation")
	}
	if err := CheckDeviceSSHStageOne(context.Background(), db.DB); err != nil {
		t.Fatalf("stage-one schema check: %v", err)
	}
	if err := CheckDeviceSSHSchema(context.Background(), db.DB); err == nil {
		t.Fatal("deploy-ready schema check accepted NOT VALID stage-one constraints")
	}

	if _, err := db.Exec(`
		INSERT INTO devices (
			business_id, hardware_name, management_ip,
			ssh_username, ssh_secret_ciphertext, ssh_secret_kind
		) VALUES ($1, 'new-unpinned-router', '10.99.0.10', 'admin', 'ciphertext', 'password')`, businessID); err == nil {
		t.Fatal("new unpinned credential row was accepted after migration")
	}

	// Reapplying is the deploy/recovery case: it must remain safe and preserve
	// the legacy row while operators are still enrolling fingerprints.
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatalf("reapply migration: %v", err)
	}

	if _, err := db.Exec(`
		UPDATE devices
		   SET ssh_username = NULL,
		       ssh_secret_ciphertext = NULL,
		       ssh_secret_kind = NULL,
		       ssh_host_key_fingerprint = NULL
		 WHERE device_id = $1`, legacyDeviceID); err != nil {
		t.Fatalf("clear legacy credentials after simulated enrollment decision: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE devices VALIDATE CONSTRAINT devices_ssh_creds_complete`); err != nil {
		t.Fatalf("validate credential constraint after remediation: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE devices VALIDATE CONSTRAINT devices_ssh_port_valid`); err != nil {
		t.Fatalf("validate port constraint: %v", err)
	}
	if err := CheckDeviceSSHSchema(context.Background(), db.DB); err != nil {
		t.Fatalf("deploy-ready schema check rejected validated constraints: %v", err)
	}

	// Recovery/replay after stage two must be monotonic: never turn a validated
	// constraint back into NOT VALID.
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatalf("reapply migration after validation: %v", err)
	}
	if err := CheckDeviceSSHSchema(context.Background(), db.DB); err != nil {
		t.Fatalf("migration replay downgraded deploy-ready schema: %v", err)
	}
}
