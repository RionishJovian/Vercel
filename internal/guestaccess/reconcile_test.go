package guestaccess

import (
	"context"
	"errors"
	"testing"
	"time"
)

type repairFixture struct {
	claims  []RecoveryClaim
	err     error
	fenced  bool
	calls   int
	offline bool
}

func (f *repairFixture) FenceInterrupted(_ context.Context, biz string, limit int) ([]RecoveryClaim, error) {
	if biz != "tenant" || limit != 25 {
		panic("unexpected repair scope")
	}
	f.fenced = f.err == nil
	return f.claims, f.err
}
func (f *repairFixture) Contain(ctx context.Context, g RecoveryClaim) error {
	if !f.fenced {
		panic("remote write before committed fence")
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 10*time.Second {
		panic("unbounded repair")
	}
	f.calls++
	if f.offline {
		return errors.New("sensitive router error")
	}
	return nil
}
func TestRecoveryReconciler(t *testing.T) {
	claim := RecoveryClaim{BusinessID: "tenant", DeviceID: "gateway", VoucherID: "voucher", TransferID: "transfer"}
	t.Run("committed fence then containment", func(t *testing.T) {
		f := &repairFixture{claims: []RecoveryClaim{claim}}
		r, err := (RecoveryReconciler{Store: f, Router: f}).Reconcile(context.Background(), "tenant")
		if err != nil || r.Contained != 1 || r.Pending != 0 || f.calls != 1 {
			t.Fatal(r, err)
		}
	})
	t.Run("offline retry", func(t *testing.T) {
		f := &repairFixture{claims: []RecoveryClaim{claim}, offline: true}
		s := RecoveryReconciler{Store: f, Router: f}
		r, err := s.Reconcile(context.Background(), "tenant")
		if err != nil || r.Pending != 1 {
			t.Fatal(r, err)
		}
		f.offline = false
		r, err = s.Reconcile(context.Background(), "tenant")
		if err != nil || r.Contained != 1 || f.calls != 2 {
			t.Fatal(r, err)
		}
	})
	t.Run("uncertain database commit", func(t *testing.T) {
		f := &repairFixture{claims: []RecoveryClaim{claim}, err: errors.New("lost commit")}
		_, err := (RecoveryReconciler{Store: f, Router: f}).Reconcile(context.Background(), "tenant")
		if err == nil || f.calls != 0 {
			t.Fatal("uncertain claim reached router")
		}
	})
	t.Run("reject whole mixed tenant batch", func(t *testing.T) {
		foreign := claim
		foreign.BusinessID = "foreign"
		f := &repairFixture{claims: []RecoveryClaim{claim, foreign}}
		_, err := (RecoveryReconciler{Store: f, Router: f}).Reconcile(context.Background(), "tenant")
		if err == nil || f.calls != 0 {
			t.Fatal("mixed tenant batch reached router")
		}
	})
	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		f := &repairFixture{claims: []RecoveryClaim{claim}}
		_, err := (RecoveryReconciler{Store: f, Router: f}).Reconcile(ctx, "tenant")
		if err == nil || f.fenced || f.calls != 0 {
			t.Fatal("canceled reconciliation proceeded")
		}
	})
}
