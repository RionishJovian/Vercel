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

type provisionTestRouter struct {
	calls    int
	contains int
	fail     bool
}

func (r *provisionTestRouter) Ensure(context.Context, model.GuestVoucherGrant) error {
	r.calls++
	if r.fail {
		return errors.New("uncertain activation")
	}
	return nil
}
func (r *provisionTestRouter) Contain(context.Context, model.GuestVoucherGrant) error {
	r.contains++
	return nil
}

func TestDB_GuestProvisionOrchestration(t *testing.T) {
	db := requireHarness(t)
	migration, e := os.ReadFile("../../migrations/20261001200000_guest_voucher_grants.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(migration)); e != nil {
		t.Fatal(e)
	}
	applyVoucherActivationMigration(t, db)
	biz := insertBusiness(t, db)
	device := insertDeviceRow(t, db, biz, nil, "provision test", "MAIN_GATEWAY")
	for _, failure := range []bool{false, true} {
		var v *model.Voucher
		inTenant(t, db, biz, func(tx *sql.Tx) error {
			var e error
			v, e = NewVoucherRepository(db).CreateVoucherTXN(tx, &model.Voucher{BusinessID: biz, PlanKey: "guest", PlanLabel: "Guest", Code: "PROVISION" + strings.ToUpper(randHex(8)), Status: "unused"})
			if e != nil {
				return e
			}
			return (GuestVoucherRepository{}).IssueTXN(tx, biz, v.ID, device, 5, 20)
		})
		r := &provisionTestRouter{fail: failure}
		s := guestaccess.ProvisionService{Store: GuestProvisionRepository{DB: db}, Router: r}
		req := guestaccess.ProvisionRequest{BusinessID: biz, DeviceID: device, Code: v.Code, ClientMAC: "02:00:00:00:00:01"}
		g, e := s.Redeem(context.Background(), req)
		if failure {
			if !errors.Is(e, guestaccess.ErrProvisionNeedsRepair) {
				t.Fatal(e)
			}
			r.fail = false
			if _, e = s.Redeem(context.Background(), req); !errors.Is(e, ErrGuestVoucherUnavailable) {
				t.Fatalf("uncertain grant retried: %v", e)
			}
			if r.calls != 1 {
				t.Fatal("blocked grant reached router")
			}
		} else {
			if e != nil {
				t.Fatal(e)
			}
			resume, e := s.Redeem(context.Background(), req)
			if !errors.Is(e, ErrGuestVoucherUnavailable) || resume.State != "" || r.calls != 1 {
				t.Fatalf("active code retry reached provisioning: state=%s calls=%d err=%v", resume.State, r.calls, e)
			}
			inTenant(t, db, biz, func(tx *sql.Tx) error {
				var state string
				var sameExpiry bool
				if e := tx.QueryRow(`SELECT state,expires_at=$2 FROM guest_voucher_grants WHERE voucher_id=$1`, g.VoucherID, g.ExpiresAt).Scan(&state, &sameExpiry); e != nil {
					return e
				}
				if state != "active" || !sameExpiry {
					t.Fatal("denied code retry modified paid grant")
				}
				return nil
			})
			inTenant(t, db, biz, func(tx *sql.Tx) error {
				_, e := tx.Exec(`UPDATE vouchers SET status='revoked' WHERE id=$1`, v.ID)
				return e
			})
			if _, e = s.Redeem(context.Background(), req); !errors.Is(e, ErrGuestVoucherUnavailable) {
				t.Fatalf("revoked grant retried: %v", e)
			}
		}
	}
}
