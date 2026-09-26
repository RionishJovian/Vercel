package database

import (
	"database/sql"
	"testing"
	"time"

	"mega-magic/internal/model"
)

// TestDB_SecurityPosture_isolation proves the C1 posture path: a device's
// RouterOS version + posture is written and read tenant-scoped, and another
// business cannot see it (RLS).
func TestDB_SecurityPosture_isolation(t *testing.T) {
	db := requireHarness(t)
	drepo := NewDeviceRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)

	devA, err := drepo.CreateDevice(&model.Device{BusinessID: bizA, HardwareName: "ap-a",
		DeploymentLayer: "ACCESS_POINT", Vendor: "mikrotik", Category: "ap", Transport: "direct"})
	if err != nil {
		t.Fatalf("create device: %v", err)
	}

	now := time.Now().UTC()
	// Own tenant: save the observed version + posture.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		return drepo.SavePostureTXN(tx, bizA, devA.DeviceID, "6.42.1", "stable", "affected", now)
	})

	// Own tenant reads it back.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		rs, err := drepo.ListPostureTXN(tx, bizA)
		if err != nil {
			return err
		}
		if len(rs) != 1 {
			t.Fatalf("tenant A posture rows = %d, want 1", len(rs))
		}
		if rs[0].Version == nil || *rs[0].Version != "6.42.1" || rs[0].Posture != "affected" {
			t.Fatalf("posture row = %+v, want 6.42.1/affected", rs[0])
		}
		if rs[0].CheckedAt == nil {
			t.Fatal("version_checked_at must be set")
		}
		return nil
	})

	// Cross-tenant read is invisible (no existence leak).
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		got, err := drepo.GetPostureTXN(tx, bizB, devA.DeviceID)
		if err != nil {
			return err
		}
		if got != nil {
			t.Fatal("cross-business posture visible to tenant B")
		}
		return nil
	})

	// Cross-tenant write affects 0 rows (returns an error, tenant-guarded).
	var writeErr error
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		writeErr = drepo.SavePostureTXN(tx, bizB, devA.DeviceID, "7.99", "stable", "safe", now)
		return nil
	})
	if writeErr == nil {
		t.Fatal("cross-business posture write must not succeed")
	}
}
