package database

import (
	"database/sql"
	"testing"

	"mega-magic/internal/model"
)

// TestDB_DeviceBackup_isolation proves C2 tenant isolation: a device's config
// backups and its change-history timeline are readable only by their own
// business, and cross-tenant reads/writes are blocked (RLS).
func TestDB_DeviceBackup_isolation(t *testing.T) {
	db := requireHarness(t)
	drepo := NewDeviceRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)

	devA, err := drepo.CreateDevice(&model.Device{BusinessID: bizA, HardwareName: "bk-a",
		DeploymentLayer: "ACCESS_POINT", Vendor: "mikrotik", Category: "ap", Transport: "direct"})
	if err != nil {
		t.Fatalf("create device: %v", err)
	}

	var backupID string
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		b, e := drepo.CreateBackupTXN(tx, bizA, devA.DeviceID, "nightly", "snapshot", `{"peer_id":"x"}`, "")
		if e != nil {
			return e
		}
		backupID = b.ID
		return drepo.RecordChangeTXN(tx, bizA, devA.DeviceID, "OWNER", "backup.create", "captured")
	})

	// Own tenant sees the backup + timeline.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		bs, e := drepo.ListBackupsByDeviceTXN(tx, bizA, devA.DeviceID)
		if e != nil {
			return e
		}
		if len(bs) != 1 {
			t.Fatalf("tenant A backups = %d, want 1", len(bs))
		}
		cs, e := drepo.ListDeviceChangesTXN(tx, bizA, devA.DeviceID)
		if e != nil {
			return e
		}
		if len(cs) != 1 || cs[0].Action != "backup.create" {
			t.Fatalf("tenant A timeline = %+v, want one backup.create", cs)
		}
		return nil
	})

	// Cross-tenant: invisible.
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		bs, e := drepo.ListBackupsByDeviceTXN(tx, bizB, devA.DeviceID)
		if e != nil {
			return e
		}
		if len(bs) != 0 {
			t.Fatal("cross-business backups visible to tenant B")
		}
		got, e := drepo.GetBackupTXN(tx, bizB, backupID)
		if e != nil {
			return e
		}
		if got != nil {
			t.Fatal("cross-business backup fetch returned a row")
		}
		cs, e := drepo.ListDeviceChangesTXN(tx, bizB, devA.DeviceID)
		if e != nil {
			return e
		}
		if len(cs) != 0 {
			t.Fatal("cross-business change timeline visible to tenant B")
		}
		return nil
	})

	// Cross-tenant delete must fail (tenant-guarded).
	var delErr error
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		delErr = drepo.DeleteBackupTXN(tx, bizB, backupID)
		return nil
	})
	if delErr == nil {
		t.Fatal("cross-business backup delete must not succeed")
	}
}
