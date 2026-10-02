package database

import (
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"mega-magic/internal/model"
)

func TestDB_GuestVoucherDurableClaim(t *testing.T) {
	db := requireHarness(t)
	migration, err := os.ReadFile("../../migrations/20261001200000_guest_voucher_grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	// Migration is idempotent and never converts old business-pass vouchers.
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	biz := insertBusiness(t, db)
	other := insertBusiness(t, db)
	device := insertDeviceRow(t, db, biz, nil, "voucher test", "MAIN_GATEWAY")
	otherDevice := insertDeviceRow(t, db, other, nil, "other tenant", "MAIN_GATEWAY")
	repo := GuestVoucherRepository{}
	var v *model.Voucher
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		var err error
		v, err = NewVoucherRepository(db).CreateVoucherTXN(tx, &model.Voucher{BusinessID: biz, PlanKey: "guest-daily", PlanLabel: "Guest daily", Code: "GUEST" + strings.ToUpper(randHex(8)), Status: "unused"})
		if err != nil {
			return err
		}
		return repo.IssueTXN(tx, biz, v.ID, device, 1, 75)
	})
	const mac = "AA:BB:CC:DD:EE:FE"
	// Real row locks must allow only one worker, not eight router operations.
	var wg sync.WaitGroup
	results := make(chan *model.GuestVoucherGrant, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := db.Begin()
			if err != nil {
				errs <- err
				return
			}
			defer tx.Rollback()
			var g *model.GuestVoucherGrant
			err = WithTenant(tx, biz, func() error { var err error; g, err = repo.ReserveTXN(tx, biz, device, v.Code, mac); return err })
			if err == nil {
				err = tx.Commit()
			}
			if err != nil {
				errs <- err
				return
			}
			results <- g
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	var winner *model.GuestVoucherGrant
	wins := 0
	busy := 0
	for g := range results {
		winner = g
		wins++
	}
	for err := range errs {
		if !errors.Is(err, ErrGuestVoucherBusy) {
			t.Fatal(err)
		}
		busy++
	}
	if wins != 1 || busy != 7 {
		t.Fatalf("wins=%d busy=%d", wins, busy)
	}
	if winner.FirstUsedAt == nil || winner.ExpiresAt == nil || winner.ExpiresAt.Sub(*winner.FirstUsedAt).Seconds() != 60 {
		t.Fatal("first-use elapsed deadline not fixed")
	}
	expectError := func(b, d, mac string, want error) {
		t.Helper()
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		err = WithTenant(tx, b, func() error { _, err := repo.ReserveTXN(tx, b, d, v.Code, mac); return err })
		if !errors.Is(err, want) {
			t.Fatalf("got %v want %v", err, want)
		}
	}
	expectError(other, otherDevice, mac, ErrGuestVoucherUnavailable)
	expectError(biz, otherDevice, mac, ErrGuestVoucherUnavailable)
	expectError(biz, device, "AA:BB:CC:DD:EE:FC", ErrGuestVoucherUnavailable)
	// Confirmed worker failure releases only its lease, not the purchase deadline.
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		ok, err := repo.ReleaseWorkerTXN(tx, biz, v.ID, winner.LeaseToken)
		if err == nil && !ok {
			t.Fatal("current worker could not release its lease")
		}
		return err
	})
	var retry *model.GuestVoucherGrant
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		var err error
		retry, err = repo.ReserveTXN(tx, biz, device, v.Code, mac)
		return err
	})
	if !retry.ExpiresAt.Equal(*winner.ExpiresAt) || retry.LeaseToken == winner.LeaseToken {
		t.Fatal("retry changed deadline or reused fencing token")
	}
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		ok, err := repo.ReleaseWorkerTXN(tx, biz, v.ID, winner.LeaseToken)
		if err != nil {
			return err
		}
		if ok {
			t.Fatal("stale worker released the replacement lease")
		}
		ok, err = repo.FinalizeTXN(tx, biz, v.ID, winner.LeaseToken)
		if err != nil {
			return err
		}
		if ok {
			t.Fatal("stale worker finalized")
		}
		ok, err = repo.FinalizeTXN(tx, biz, v.ID, retry.LeaseToken)
		if err == nil && !ok {
			t.Fatal("current worker could not finalize")
		}
		return err
	})
	var active *model.GuestVoucherGrant
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		var err error
		active, err = repo.ReserveTXN(tx, biz, device, v.Code, mac)
		return err
	})
	if active.State != "active" || !active.ExpiresAt.Equal(*winner.ExpiresAt) {
		t.Fatal("active retry not idempotent")
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err = WithTenant(tx, biz, func() error {
		_, err := tx.Exec(`UPDATE guest_voucher_grants SET minutes=2 WHERE voucher_id=$1`, v.ID)
		return err
	})
	if err == nil {
		t.Fatal("snapshot mutation allowed")
	}
}
