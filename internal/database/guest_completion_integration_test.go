package database

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

type lostCompletionReply struct {
	GuestRecoveryRepository
	claim guestaccess.RecoveryClaim
}

func (s *lostCompletionReply) Complete(ctx context.Context, g guestaccess.RecoveryClaim) error {
	s.claim = g
	if err := s.GuestRecoveryRepository.Complete(ctx, g); err != nil {
		return err
	}
	return errors.New("simulated lost commit reply")
}

type completionRouter struct{ activations, containments int }

func (r *completionRouter) StopAndRead(context.Context, guestaccess.RecoveryClaim) (int64, error) {
	return 12000000, nil
}
func (r *completionRouter) Activate(context.Context, guestaccess.RecoveryClaim, int64) error {
	r.activations++
	return nil
}
func (r *completionRouter) Contain(context.Context, guestaccess.RecoveryClaim) error {
	r.containments++
	return nil
}

func TestDB_GuestRecoveryAmbiguousCompletion(t *testing.T) {
	db := requireHarness(t)
	for _, name := range []string{"20261001200000_guest_voucher_grants.sql", "20261001210000_guest_access_recovery.sql", "20261002200000_guest_recovery_completion.sql", "20261021090000_guest_voucher_activation.sql"} {
		b, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(b)); err != nil {
			t.Fatal(err)
		}
	}
	biz := insertBusiness(t, db)
	device := insertDeviceRow(t, db, biz, nil, "completion test", "MAIN_GATEWAY")
	rr := GuestRecoveryRepository{DB: db}
	var v *model.Voucher
	var secret string
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		var err error
		v, err = NewVoucherRepository(db).CreateVoucherTXN(tx, &model.Voucher{BusinessID: biz, PlanKey: "guest-recovery", PlanLabel: "Guest", Code: "COMMIT" + strings.ToUpper(randHex(8)), Status: "unused"})
		if err != nil {
			return err
		}
		vr := GuestVoucherRepository{}
		if err = vr.IssueTXN(tx, biz, v.ID, device, 60, 75); err != nil {
			return err
		}
		g, err := vr.ReserveTXN(tx, biz, device, v.Code, "02:00:00:00:00:01")
		if err != nil {
			return err
		}
		ok, err := vr.FinalizeTXN(tx, biz, v.ID, g.LeaseToken)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatal("fixture finalize denied")
		}
		secret, err = rr.IssueTXN(tx, biz, v.ID, device)
		return err
	})
	hash, _ := guestaccess.HashSecret(secret)
	req := guestaccess.RecoveryRequest{BusinessID: biz, DeviceID: device, VoucherID: v.ID, NewMAC: "02:00:00:00:00:02"}
	// Simulate successful COMMIT whose response was lost: cleanup sees an active
	// row, but must still fence the exact completed transfer using its receipt.
	store := &lostCompletionReply{GuestRecoveryRepository: rr}
	router := &completionRouter{}
	req.Secret = secret
	result, err := (guestaccess.RecoveryService{Store: store, Router: router}).Restore(context.Background(), req)
	if !errors.Is(err, guestaccess.ErrRecoveryNeedsRepair) || result.TransferID != "" || router.activations != 1 || router.containments != 1 {
		t.Fatal("ambiguous completion was reported as successful or not contained")
	}
	req.Secret = ""
	g := store.claim
	if err = rr.Block(context.Background(), g); err != nil {
		t.Fatal("cleanup not idempotent", err)
	}
	if _, err = rr.Lookup(context.Background(), req, hash); err == nil {
		t.Fatal("ambiguous completion remained resumable")
	}
	req.NewMAC = "02:00:00:00:00:03"
	if _, err = rr.Reserve(context.Background(), req, hash); err == nil {
		t.Fatal("ambiguous completion remained transferable")
	}
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		var state, receipt string
		var used int64
		if err := tx.QueryRow(`SELECT state,completed_transfer_id::text,used_bytes FROM guest_access_recovery WHERE voucher_id=$1`, v.ID).Scan(&state, &receipt, &used); err != nil {
			return err
		}
		if state != "blocked" || receipt != g.TransferID || used != 12000000 {
			t.Fatal("cleanup lost receipt or consumption")
		}
		return nil
	})
	for _, value := range []string{"NULL", "gen_random_uuid()"} {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		err = WithTenant(tx, biz, func() error {
			_, err := tx.Exec(`UPDATE guest_access_recovery SET completed_transfer_id=`+value+` WHERE voucher_id=$1`, v.ID)
			return err
		})
		tx.Rollback()
		if err == nil {
			t.Fatal("completion receipt replaced outside verified completion")
		}
	}
}
