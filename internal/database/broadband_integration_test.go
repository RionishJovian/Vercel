package database

import (
	"database/sql"
	"mega-magic/internal/broadband"
	"os"
	"testing"
	"time"
)

func TestBroadbandLedgerIsolatesTenantsAndRetries(t *testing.T) {
	db := requireHarness(t)
	src, e := os.ReadFile("../../db/migrations/20261104000000_broadband.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(src)); e != nil {
		t.Fatal(e)
	}
	a, b := insertBusiness(t, db), insertBusiness(t, db)
	device := insertDeviceRow(t, db, a, nil, "broadband", "MAIN_GATEWAY")
	account, operation := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM broadband_session_usage WHERE business_id=$1`, a)
		_, _ = db.Exec(`DELETE FROM broadband_operations WHERE business_id=$1`, a)
		_, _ = db.Exec(`DELETE FROM broadband_accounts WHERE business_id=$1`, a)
	})
	repo := BroadbandRepository{}
	inTenant(t, db, a, func(tx *sql.Tx) error {
		_, e := tx.Exec(`INSERT INTO broadband_accounts(id,business_id,device_id,username,profile) VALUES($1,$2,$3,'home001','default')`, account, a, device)
		return e
	})
	inTenant(t, db, a, func(tx *sql.Tx) error {
		return repo.ReserveTXN(tx, a, device, BroadbandOperation{ID: operation, AccountID: account, Kind: "create", AmountMMK: 15000, PeriodDays: 30, RequestHash: "same", Payload: "sealed"})
	})
	inTenant(t, db, a, func(tx *sql.Tx) error {
		return repo.ReserveTXN(tx, a, device, BroadbandOperation{ID: operation, AccountID: account, Kind: "create", AmountMMK: 15000, PeriodDays: 30, RequestHash: "same", Payload: "sealed"})
	})
	inTenant(t, db, b, func(tx *sql.Tx) error {
		list, e := repo.ListTXN(tx, b, device)
		if len(list) != 0 {
			t.Fatal("foreign account leaked")
		}
		return e
	})
	observed := broadband.State{PeriodSeconds: 30 * 86400, FirstUse: 1793750400, ExpiresAt: 1796342400, LastClock: 1793750400}
	for i := 0; i < 2; i++ {
		inTenant(t, db, a, func(tx *sql.Tx) error {
			return repo.ObserveTXN(tx, a, device, account, operation, observed, time.Now().UTC())
		})
	}
	inTenant(t, db, a, func(tx *sql.Tx) error {
		return repo.ObserveSessionTXN(tx, a, device, account, &broadband.SessionUsage{ID: "80000001", UptimeSeconds: 60, DownloadBytes: 1000, UploadBytes: 200}, time.Now())
	})
	inTenant(t, db, a, func(tx *sql.Tx) error {
		return repo.ObserveSessionTXN(tx, a, device, account, &broadband.SessionUsage{ID: "80000001", UptimeSeconds: 10, DownloadBytes: 100, UploadBytes: 20}, time.Now().Add(time.Minute))
	})
	inTenant(t, db, a, func(tx *sql.Tx) error {
		var down int
		var reset bool
		e := tx.QueryRow(`SELECT download_bytes,counter_reset FROM broadband_session_usage WHERE business_id=$1 AND account_id=$2`, a, account).Scan(&down, &reset)
		if down != 1000 || !reset {
			t.Fatal("reset silently lost or duplicated observed usage", down, reset)
		}
		return e
	})
	inTenant(t, db, a, func(tx *sql.Tx) error {
		var amount, count int
		e := tx.QueryRow(`SELECT COALESCE(sum(amount_mmk),0),count(*) FROM broadband_operations WHERE business_id=$1 AND state='applied'`, a).Scan(&amount, &count)
		if amount != 15000 || count != 1 {
			t.Fatal("reconciliation duplicated payment", amount, count)
		}
		return e
	})
	newOp := "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	inTenant(t, db, a, func(tx *sql.Tx) error {
		return repo.ReserveTXN(tx, a, device, BroadbandOperation{ID: newOp, AccountID: account, Kind: "suspend", RequestHash: "suspend", Payload: "sealed"})
	})
	suspended := observed
	suspended.Suspended = true
	at := time.Now().UTC()
	inTenant(t, db, a, func(tx *sql.Tx) error { return repo.ObserveTXN(tx, a, device, account, newOp, suspended, at) })
	inTenant(t, db, a, func(tx *sql.Tx) error {
		return repo.ObserveTXN(tx, a, device, account, operation, observed, at.Add(time.Second))
	})
	inTenant(t, db, a, func(tx *sql.Tx) error {
		list, e := repo.ListTXN(tx, a, device)
		if e == nil && (list[0].RouterOperation != newOp || !list[0].Observation.Suspended) {
			t.Fatal("stale operation rolled back account", list[0])
		}
		return e
	})
	// A proven pre-install rejection releases the name, but its immutable
	// operation remains rejected and contributes no payment.
	rejected := "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	inTenant(t, db, a, func(tx *sql.Tx) error {
		_, e := tx.Exec(`INSERT INTO broadband_accounts(id,business_id,device_id,username,profile) VALUES($1,$2,$3,'correctable','default')`, rejected, a, device)
		if e != nil {
			return e
		}
		return repo.ReserveTXN(tx, a, device, BroadbandOperation{ID: rejected, AccountID: rejected, Kind: "create", AmountMMK: 5000, PeriodDays: 7, RequestHash: "rejected", Payload: "sealed"})
	})
	inTenant(t, db, a, func(tx *sql.Tx) error { return repo.RejectCreateTXN(tx, a, device, rejected) })
	inTenant(t, db, a, func(tx *sql.Tx) error {
		_, e := tx.Exec(`INSERT INTO broadband_accounts(id,business_id,device_id,username,profile) VALUES('eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee',$1,$2,'correctable','default')`, a, device)
		return e
	})
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(`UPDATE broadband_operations SET state='pending' WHERE business_id=$1 AND id=$2`, a, rejected); e == nil {
		t.Fatal("a rejected operation could be revived")
	}
	_ = tx.Rollback()
}
