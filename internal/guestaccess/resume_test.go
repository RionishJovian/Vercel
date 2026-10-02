package guestaccess

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type resumeFixture struct {
	g           BindingSnapshot
	events      []string
	used        int64
	lookups     int
	failLookup  int
	change      func(*BindingSnapshot)
	afterVerify func()
	verifyErr   error
	deadline    time.Time
}

func (f *resumeFixture) Lookup(_ context.Context, req RecoveryRequest, hash []byte) (BindingSnapshot, error) {
	f.lookups++
	f.events = append(f.events, "lookup")
	if req.Secret != "" || len(hash) != 32 {
		return BindingSnapshot{}, errors.New("plaintext leaked")
	}
	if f.lookups == f.failLookup {
		return BindingSnapshot{}, ErrRecoveryDenied
	}
	g := f.g
	if f.lookups == 2 && f.change != nil {
		f.change(&g)
	}
	return g, nil
}
func (f *resumeFixture) Verify(ctx context.Context, g BindingSnapshot) (int64, error) {
	f.events = append(f.events, "verify-existing")
	f.deadline, _ = ctx.Deadline()
	if f.afterVerify != nil {
		f.afterVerify()
	}
	return f.used, f.verifyErr
}
func newResumeFixture(t *testing.T) (*resumeFixture, RecoveryRequest, ResumeService) {
	t.Helper()
	now := time.Now()
	secret, err := RecoverySecret()
	if err != nil {
		t.Fatal(err)
	}
	f := &resumeFixture{g: BindingSnapshot{BusinessID: "biz", DeviceID: "gateway", VoucherID: "voucher", ClientMAC: "02:00:00:00:00:02", TotalBytes: 75000000, PriorUsedBytes: 40000000, ExpiresAt: now.Add(time.Hour), Revision: now}, used: 10000000}
	req := RecoveryRequest{BusinessID: "biz", DeviceID: "gateway", VoucherID: "voucher", NewMAC: f.g.ClientMAC, Secret: secret}
	return f, req, ResumeService{Store: f, Router: f}
}
func TestResumePreservesTransferredQuotaAndDeadline(t *testing.T) {
	f, req, s := newResumeFixture(t)
	for i := 0; i < 3; i++ {
		result, err := s.Resume(context.Background(), req)
		if err != nil || result.RemainingBytes != 25000000 || !result.ExpiresAt.Equal(f.g.ExpiresAt) {
			t.Fatal("resume replenished allowance or changed expiry")
		}
	}
	if !f.deadline.Equal(f.g.ExpiresAt) {
		t.Fatal("router readback escaped paid deadline")
	}
	want := []string{"lookup", "verify-existing", "lookup", "lookup", "verify-existing", "lookup", "lookup", "verify-existing", "lookup"}
	if !reflect.DeepEqual(f.events, want) {
		t.Fatalf("unsafe resume order: %v", f.events)
	}
}
func TestResumeRechecksAuthorityAfterRouterIO(t *testing.T) {
	for name, change := range map[string]func(*BindingSnapshot){
		"tenant":   func(g *BindingSnapshot) { g.BusinessID = "other" },
		"gateway":  func(g *BindingSnapshot) { g.DeviceID = "other" },
		"voucher":  func(g *BindingSnapshot) { g.VoucherID = "other" },
		"MAC":      func(g *BindingSnapshot) { g.ClientMAC = "02:00:00:00:00:04" },
		"revision": func(g *BindingSnapshot) { g.Revision = g.Revision.Add(time.Microsecond) },
		"usage":    func(g *BindingSnapshot) { g.PriorUsedBytes++ },
		"quota":    func(g *BindingSnapshot) { g.TotalBytes++ },
		"expiry":   func(g *BindingSnapshot) { g.ExpiresAt = g.ExpiresAt.Add(time.Minute) },
	} {
		t.Run(name, func(t *testing.T) {
			f, req, s := newResumeFixture(t)
			f.change = change
			result, err := s.Resume(context.Background(), req)
			if !errors.Is(err, ErrRecoveryDenied) || result.RemainingBytes != 0 {
				t.Fatal("changed authority confirmed resume")
			}
		})
	}
}
func TestResumeFailuresNeverProvisionAccess(t *testing.T) {
	for _, mode := range []string{"bad secret", "first lookup", "second lookup", "foreign initial", "wrong initial MAC", "no revision", "router offline", "negative usage", "exhausted", "canceled", "expired"} {
		t.Run(mode, func(t *testing.T) {
			f, req, s := newResumeFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "bad secret":
				req.Secret = "CODE"
			case "first lookup":
				f.failLookup = 1
			case "second lookup":
				f.failLookup = 2
			case "foreign initial":
				f.g.BusinessID = "other"
			case "wrong initial MAC":
				f.g.ClientMAC = "02:00:00:00:00:04"
			case "no revision":
				f.g.Revision = time.Time{}
			case "router offline":
				f.verifyErr = errors.New("offline")
			case "negative usage":
				f.used = -1
			case "exhausted":
				f.used = f.g.TotalBytes - f.g.PriorUsedBytes
			case "canceled":
				f.afterVerify = cancel
			case "expired":
				var now time.Time
				s.Now = func() time.Time {
					if now.IsZero() {
						return time.Now()
					}
					return now
				}
				f.afterVerify = func() { now = f.g.ExpiresAt }
			}
			result, err := s.Resume(ctx, req)
			if !errors.Is(err, ErrRecoveryDenied) || result.RemainingBytes != 0 {
				t.Fatal("unverified resume reported success")
			}
			if (mode == "foreign initial" || mode == "wrong initial MAC" || mode == "no revision" || mode == "first lookup") && len(f.events) != 1 {
				t.Fatal("unproven authority reached router")
			}
		})
	}
}
