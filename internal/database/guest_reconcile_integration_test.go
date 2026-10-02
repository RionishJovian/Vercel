package database

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

func TestDB_GuestRecoveryRestartFence(t *testing.T) {
	db := requireHarness(t)
	for _, name := range []string{"20261001200000_guest_voucher_grants.sql", "20261001210000_guest_access_recovery.sql", "20261002200000_guest_recovery_completion.sql"} {
		b, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(b)); err != nil {
			t.Fatal(err)
		}
	}
	biz, foreign := insertBusiness(t, db), insertBusiness(t, db)
	rr := GuestRecoveryRepository{DB: db}
	makeClaim := func(tenant string) guestaccess.RecoveryClaim {
		device := insertDeviceRow(t, db, tenant, nil, "repair test", "MAIN_GATEWAY")
		var v *model.Voucher
		var secret string
		inTenant(t, db, tenant, func(tx *sql.Tx) error {
			var err error
			v, err = NewVoucherRepository(db).CreateVoucherTXN(tx, &model.Voucher{BusinessID: tenant, PlanKey: "guest-recovery", PlanLabel: "Guest", Code: "REPAIR" + strings.ToUpper(randHex(8)), Status: "unused"})
			if err != nil {
				return err
			}
			vr := GuestVoucherRepository{}
			if err = vr.IssueTXN(tx, tenant, v.ID, device, 60, 75); err != nil {
				return err
			}
			g, err := vr.ReserveTXN(tx, tenant, device, v.Code, "02:00:00:00:00:01")
			if err != nil {
				return err
			}
			ok, err := vr.FinalizeTXN(tx, tenant, v.ID, g.LeaseToken)
			if err != nil {
				return err
			}
			if !ok {
				t.Fatal("fixture finalize")
			}
			secret, err = rr.IssueTXN(tx, tenant, v.ID, device)
			return err
		})
		hash, _ := guestaccess.HashSecret(secret)
		g, err := rr.Reserve(context.Background(), guestaccess.RecoveryRequest{BusinessID: tenant, DeviceID: device, VoucherID: v.ID, NewMAC: "02:00:00:00:00:02"}, hash)
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	stale, recent, other, complete := makeClaim(biz), makeClaim(biz), makeClaim(foreign), makeClaim(biz)
	var err error
	stale, err = rr.FreezeUsage(context.Background(), stale, 12000000)
	if err != nil {
		t.Fatal(err)
	}
	complete, err = rr.FreezeUsage(context.Background(), complete, 5000000)
	if err != nil {
		t.Fatal(err)
	}
	if err = rr.Complete(context.Background(), complete); err != nil {
		t.Fatal(err)
	}
	for _, g := range []guestaccess.RecoveryClaim{stale, other} {
		inTenant(t, db, g.BusinessID, func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE guest_access_recovery SET updated_at=clock_timestamp()-interval '3 minutes' WHERE voucher_id=$1`, g.VoucherID)
			return err
		})
	}
	claims, err := rr.FenceInterrupted(context.Background(), biz, 25)
	if err != nil || len(claims) != 1 || claims[0].VoucherID != stale.VoucherID || claims[0].UsedBytes != 12000000 || !claims[0].ExpiresAt.Equal(stale.ExpiresAt) {
		t.Fatal("restart snapshot differs", claims, err)
	}
	if err = rr.Complete(context.Background(), stale); err == nil {
		t.Fatal("late completion reopened blocked transfer")
	}
	// A router that was offline can be contained on the next scan. No new
	// allowance or expiry is generated, and fresh/completed/foreign rows stay out.
	claims, err = rr.FenceInterrupted(context.Background(), biz, 25)
	if err != nil || len(claims) != 1 || claims[0].TransferID != stale.TransferID {
		t.Fatal("blocked retry lost", claims, err)
	}
	for _, g := range []guestaccess.RecoveryClaim{recent, other, complete} {
		inTenant(t, db, g.BusinessID, func(tx *sql.Tx) error {
			var state string
			if err := tx.QueryRow(`SELECT state FROM guest_access_recovery WHERE voucher_id=$1`, g.VoucherID).Scan(&state); err != nil {
				return err
			}
			want := "transferring"
			if g.VoucherID == complete.VoucherID {
				want = "active"
			}
			if state != want {
				t.Fatal("unrelated recovery state changed", state, want)
			}
			return nil
		})
	}
}
