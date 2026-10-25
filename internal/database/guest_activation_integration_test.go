package database

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"mega-magic/internal/model"
)

func applyVoucherActivationMigration(t *testing.T, db *DB) {
	t.Helper()
	b, err := os.ReadFile("../../migrations/20261021090000_guest_voucher_activation.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(b)); err != nil {
		t.Fatal(err)
	}
}

func TestDBGuestVoucherActivationRecordedOnce(t *testing.T) {
	db := requireHarness(t)
	b, err := os.ReadFile("../../migrations/20261001200000_guest_voucher_grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(b)); err != nil {
		t.Fatal(err)
	}
	applyVoucherActivationMigration(t, db)
	applyVoucherActivationMigration(t, db)
	biz := insertBusiness(t, db)
	device := insertDeviceRow(t, db, biz, nil, "activation test", "MAIN_GATEWAY")
	repo := GuestVoucherRepository{}
	var v *model.Voucher
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		var e error
		v, e = NewVoucherRepository(db).CreateVoucherTXN(tx, &model.Voucher{BusinessID: biz, DeviceID: &device, PlanKey: "guest-internet", PlanLabel: "Daily", Code: "ACT" + strings.ToUpper(randHex(8)), PriceMMK: 1500, Status: "unused"})
		if e != nil {
			return e
		}
		return repo.IssueTXN(tx, biz, v.ID, device, 120, 1024)
	})
	activation := func() sql.NullTime {
		t.Helper()
		var value sql.NullTime
		inTenant(t, db, biz, func(tx *sql.Tx) error {
			return tx.QueryRow(`SELECT activated_at FROM guest_voucher_grants WHERE voucher_id=$1`, v.ID).Scan(&value)
		})
		return value
	}
	if activation().Valid {
		t.Fatal("issuance or printing must not activate")
	}
	var claim *model.GuestVoucherGrant
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		var e error
		claim, e = repo.ReserveTXN(tx, biz, device, v.Code, "02:00:00:00:00:01")
		return e
	})
	if activation().Valid {
		t.Fatal("reservation before router verification must not record activation value")
	}
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		ok, e := repo.FinalizeTXN(tx, biz, v.ID, "wrong-lease")
		if ok {
			t.Fatal("stale worker activated")
		}
		return e
	})
	if activation().Valid {
		t.Fatal("failed finalization recorded activation")
	}
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		ok, e := repo.FinalizeTXN(tx, biz, v.ID, claim.LeaseToken)
		if e == nil && !ok {
			t.Fatal("current worker could not finalize")
		}
		return e
	})
	first := activation()
	if !first.Valid || first.Time.Before(*claim.FirstUsedAt) || !first.Time.Before(*claim.ExpiresAt) {
		t.Fatal("activation timestamp is missing or outside original validity")
	}
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		ok, e := repo.FinalizeTXN(tx, biz, v.ID, claim.LeaseToken)
		if ok {
			t.Fatal("replayed finalization succeeded")
		}
		return e
	})
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		reused, e := repo.ReserveTXN(tx, biz, device, v.Code, "02:00:00:00:00:01")
		if e == nil && (!reused.FirstUsedAt.Equal(*claim.FirstUsedAt) || !reused.ExpiresAt.Equal(*claim.ExpiresAt)) {
			t.Fatal("reconnect reset validity")
		}
		return e
	})
	if !activation().Time.Equal(first.Time) {
		t.Fatal("reconnect moved activation time")
	}
	for _, statement := range []string{`UPDATE guest_voucher_grants SET activated_at=NULL WHERE voucher_id=$1`, `UPDATE guest_voucher_grants SET activated_at=activated_at+interval '1 second' WHERE voucher_id=$1`} {
		tx, e := db.Begin()
		if e != nil {
			t.Fatal(e)
		}
		e = WithTenant(tx, biz, func() error { _, e := tx.Exec(statement, v.ID); return e })
		_ = tx.Rollback()
		if e == nil {
			t.Fatal("activation timestamp mutation accepted")
		}
	}
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		_, e := tx.Exec(`UPDATE vouchers SET status='revoked' WHERE id=$1`, v.ID)
		if e != nil {
			return e
		}
		_, e = tx.Exec(`UPDATE guest_voucher_grants SET state='revoked' WHERE voucher_id=$1`, v.ID)
		return e
	})
	if !activation().Time.Equal(first.Time) {
		t.Fatal("revocation erased historical activation")
	}
	// Legacy active grants are not given an invented activation date by an
	// idempotent schema upgrade.
	var legacy string
	if err = db.QueryRow(`INSERT INTO vouchers(business_id,device_id,plan_key,plan_label,code,status) VALUES($1,$2,'guest-internet','Legacy',$3,'used') RETURNING id`, biz, device, "LEGACY"+randHex(6)).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO guest_voucher_grants(voucher_id,business_id,device_id,minutes,data_mb,state,client_mac,first_used_at,expires_at) VALUES($1,$2,$3,120,1024,'active','02:00:00:00:00:02',now(),now()+interval '120 minutes')`, legacy, biz, device); err != nil {
		t.Fatal(err)
	}
	applyVoucherActivationMigration(t, db)
	var historical sql.NullTime
	if err = db.QueryRow(`SELECT activated_at FROM guest_voucher_grants WHERE voucher_id=$1`, legacy).Scan(&historical); err != nil {
		t.Fatal(err)
	}
	if historical.Valid {
		t.Fatal("legacy activation was fabricated")
	}
}
