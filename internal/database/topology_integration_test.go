package database

import (
	"testing"

	"mega-magic/internal/model"
)

// insertDeviceRow inserts a raw device row via the owner connection and returns
// its device_id. Inserts happen OUTSIDE any tenant scope (owner/admin DDL), which
// is correct: the rows are seeded as tenant-owned data, then isolation is proven
// by reading them back through RLS-bound connections.
func insertDeviceRow(t *testing.T, db *DB, businessID string, parentID *string, hwName, layer string) string {
	t.Helper()
	var id string
	err := db.QueryRow(
		`INSERT INTO devices (business_id, parent_gateway_id, hardware_name, deployment_layer)
		 VALUES ($1, $2, $3, $4) RETURNING device_id`,
		businessID, parentID, hwName, layer,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert device row: %v", err)
	}
	return id
}

// TestDB_Topology_mapCrossBusinessIsolation proves the topology map's data source
// (ListDevicesByBusinessTXN under WithTenant/RLS) isolates by tenant: business A's
// full gateway/switch/AP tree is visible to A, and business B's devices — even B's
// own tree — are invisible to A, and vice versa.
func TestDB_Topology_mapCrossBusinessIsolation(t *testing.T) {
	db := requireHarness(t)
	repo := NewDeviceRepository(db)

	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)

	// A's tree: gateway -> switch -> AP.
	gwA := insertDeviceRow(t, db, bizA, nil, "Gateway-A", "MAIN_GATEWAY")
	swA := insertDeviceRow(t, db, bizA, &gwA, "Switch-A", "DOWNSTREAM_SWITCH")
	apA := insertDeviceRow(t, db, bizA, &swA, "AP-A", "ACCESS_POINT")

	// B's tree: gateway -> AP.
	gwB := insertDeviceRow(t, db, bizB, nil, "Gateway-B", "MAIN_GATEWAY")
	insertDeviceRow(t, db, bizB, &gwB, "AP-B", "ACCESS_POINT")

	// Within A's tenant: A sees its 3 devices (parent links intact), never B's.
	devsA := listInTenant(t, repo, db, bizA)
	if len(devsA) != 3 {
		t.Fatalf("tenant A saw %d devices, want 3", len(devsA))
	}
	devsB := listInTenant(t, repo, db, bizB)
	if len(devsB) != 2 {
		t.Fatalf("tenant B saw %d devices, want 2", len(devsB))
	}

	// Cross-check: no A device leaks into B's list.
	idsB := map[string]bool{}
	for _, d := range devsB {
		idsB[d.DeviceID] = true
	}
	for _, id := range []string{gwA, swA, apA} {
		if idsB[id] {
			t.Fatalf("tenant B saw A device %s — cross-business leak", id)
		}
	}
}

// listInTenant runs ListDevicesByBusinessTXN inside a committed tenant tx and
// returns the rows.
func listInTenant(t *testing.T, repo *DeviceRepository, db *DB, businessID string) []*model.Device {
	t.Helper()
	var out []*model.Device
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	if err := WithTenant(tx, businessID, func() error {
		ds, err := repo.ListDevicesByBusinessTXN(tx, businessID)
		if err != nil {
			return err
		}
		out = ds
		return nil
	}); err != nil {
		t.Fatalf("tenant tx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return out
}