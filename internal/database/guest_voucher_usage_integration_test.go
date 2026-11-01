package database

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"mega-magic/internal/model"
)

func applyMigrationFile(t *testing.T, db *DB, name string) {
	t.Helper()
	b, err := os.ReadFile("../../migrations/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(b)); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

func TestDBGuestVoucherUsageKeepsCountersAndIsolatesTenants(t *testing.T) {
	db := requireHarness(t)
	for _, m := range []string{"20261001200000_guest_voucher_grants.sql", "20261021180000_guest_voucher_issue_batches.sql"} {
		applyMigrationFile(t, db, m)
	}
	// The migration is operator-run and must be safe to apply twice.
	applyMigrationFile(t, db, "20261101120000_guest_voucher_usage.sql")
	applyMigrationFile(t, db, "20261101120000_guest_voucher_usage.sql")

	bizA, bizB := insertBusiness(t, db), insertBusiness(t, db)
	device := insertDeviceRow(t, db, bizA, nil, "usage test", "MAIN_GATEWAY")
	var v *model.Voucher
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		var e error
		v, e = NewVoucherRepository(db).CreateVoucherTXN(tx, &model.Voucher{BusinessID: bizA, DeviceID: &device, PlanKey: "guest-internet", PlanLabel: "Hour", Code: "USG" + strings.ToUpper(randHex(8)), PriceMMK: 1000, Status: "unused"})
		return e
	})
	repo := GuestVoucherRepository{}
	now := time.Now().UTC().Truncate(time.Second)
	read := func(biz string) (int64, int64, bool) {
		var bytes, up int64
		found := false
		inTenant(t, db, biz, func(tx *sql.Tx) error {
			err := tx.QueryRow(`SELECT bytes_used,uptime_seconds FROM guest_voucher_usage WHERE voucher_id=$1`, v.ID).Scan(&bytes, &up)
			found = err == nil
			if err == sql.ErrNoRows {
				return nil
			}
			return err
		})
		return bytes, up, found
	}

	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		if !GuestVoucherUsageTableExists(tx) {
			t.Error("table should exist after migration")
		}
		return repo.UpsertUsageTXN(tx, bizA, v.ID, device, 3_200_000, 1223, now)
	})
	if b, u, ok := read(bizA); !ok || b != 3_200_000 || u != 1223 {
		t.Fatalf("stored usage = %d/%d found=%v", b, u, ok)
	}
	// A recreated router user reports zeroed counters; usage must not regress.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		return repo.UpsertUsageTXN(tx, bizA, v.ID, device, 10, 5, now.Add(time.Minute))
	})
	if b, u, _ := read(bizA); b != 3_200_000 || u != 1223 {
		t.Fatalf("counters moved backwards: %d/%d", b, u)
	}
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		return repo.UpsertUsageTXN(tx, bizA, v.ID, device, 4_000_000, 1500, now.Add(2*time.Minute))
	})
	if b, u, _ := read(bizA); b != 4_000_000 || u != 1500 {
		t.Fatalf("higher counters not stored: %d/%d", b, u)
	}
	if _, _, found := read(bizB); found {
		t.Fatal("another tenant can read this usage row")
	}
}
