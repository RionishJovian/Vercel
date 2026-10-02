package guestaccess

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type recoveryFixture struct {
	claim             RecoveryClaim
	steps             []string
	fail              string
	remaining         int64
	now               time.Time
	containContext    context.Context
	blockContextValid bool
	workContexts      []context.Context
	afterStop         func()
	afterFreeze       func()
	afterComplete     func()
	corruptFrozen     bool
}

func (f *recoveryFixture) Reserve(_ context.Context, req RecoveryRequest, hash []byte) (RecoveryClaim, error) {
	f.steps = append(f.steps, "reserve-committed")
	if req.Secret != "" || len(hash) != 32 {
		return RecoveryClaim{}, errors.New("plaintext leaked or missing proof")
	}
	if f.fail == "reserve" {
		return RecoveryClaim{}, errors.New("denied")
	}
	return f.claim, nil
}
func (f *recoveryFixture) FreezeUsage(ctx context.Context, g RecoveryClaim, used int64) (RecoveryClaim, error) {
	f.workContexts = append(f.workContexts, ctx)
	f.steps = append(f.steps, "usage-committed")
	if f.fail == "freeze" {
		return g, errors.New("db down")
	}
	g.UsedBytes += used
	if f.afterFreeze != nil {
		f.afterFreeze()
	}
	if f.corruptFrozen {
		g.BusinessID = "foreign"
	}
	return g, nil
}
func (f *recoveryFixture) Complete(ctx context.Context, g RecoveryClaim) error {
	f.workContexts = append(f.workContexts, ctx)
	f.steps = append(f.steps, "binding-committed")
	if f.fail == "complete" {
		return errors.New("revoked")
	}
	f.claim = g
	if f.afterComplete != nil {
		f.afterComplete()
	}
	return nil
}
func (f *recoveryFixture) Block(ctx context.Context, _ RecoveryClaim) error {
	f.steps = append(f.steps, "blocked")
	f.blockContextValid = ctx.Err() == nil && ctx != f.containContext
	if ctx.Err() != nil || ctx == f.containContext {
		return errors.New("blocking shares exhausted containment context")
	}
	return nil
}
func (f *recoveryFixture) StopAndRead(ctx context.Context, _ RecoveryClaim) (int64, error) {
	f.workContexts = append(f.workContexts, ctx)
	f.steps = append(f.steps, "old-session-stopped-and-verified")
	if f.fail == "stop" {
		return 0, errors.New("offline")
	}
	if f.afterStop != nil {
		f.afterStop()
	}
	return 15000000, nil
}
func (f *recoveryFixture) Activate(ctx context.Context, g RecoveryClaim, remaining int64) error {
	f.workContexts = append(f.workContexts, ctx)
	f.steps = append(f.steps, "new-session-verified")
	f.remaining = remaining
	if f.fail == "activate" {
		return errors.New("readback mismatch")
	}
	if f.fail == "expired" {
		f.now = g.ExpiresAt
	}
	return nil
}
func (f *recoveryFixture) Contain(ctx context.Context, _ RecoveryClaim) error {
	f.containContext = ctx
	f.steps = append(f.steps, "contained")
	return nil
}
func newRecoveryFixture(t *testing.T) (*recoveryFixture, RecoveryRequest, RecoveryService) {
	t.Helper()
	now := time.Now().UTC()
	secret, err := RecoverySecret()
	if err != nil {
		t.Fatal(err)
	}
	f := &recoveryFixture{now: now, claim: RecoveryClaim{BusinessID: "biz", DeviceID: "gateway", VoucherID: "purchase", TransferID: "transfer", OldMAC: "AA:BB:CC:DD:EE:FE", NewMAC: "AA:BB:CC:DD:EE:FC", TotalBytes: 75000000, UsedBytes: 25000000, ExpiresAt: now.Add(time.Hour)}}
	return f, RecoveryRequest{BusinessID: "biz", DeviceID: "gateway", VoucherID: "purchase", Secret: secret, NewMAC: f.claim.NewMAC}, RecoveryService{Store: f, Router: f, Now: func() time.Time { return f.now }}
}

func TestRecoveryRejectsInconsistentScopeWithoutCleanupWrites(t *testing.T) {
	mutations := map[string]func(*RecoveryClaim){
		"tenant":            func(g *RecoveryClaim) { g.BusinessID = "other" },
		"gateway":           func(g *RecoveryClaim) { g.DeviceID = "other" },
		"voucher":           func(g *RecoveryClaim) { g.VoucherID = "other" },
		"new MAC":           func(g *RecoveryClaim) { g.NewMAC = "AA:BB:CC:DD:EE:FA" },
		"old MAC invalid":   func(g *RecoveryClaim) { g.OldMAC = "not-a-mac" },
		"old MAC multicast": func(g *RecoveryClaim) { g.OldMAC = "FF:BB:CC:DD:EE:FE" },
		"old MAC zero":      func(g *RecoveryClaim) { g.OldMAC = "00:00:00:00:00:00" },
		"same MAC":          func(g *RecoveryClaim) { g.OldMAC = g.NewMAC },
		"missing transfer":  func(g *RecoveryClaim) { g.TransferID = "" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			f, req, s := newRecoveryFixture(t)
			mutate(&f.claim)
			if _, err := s.Restore(context.Background(), req); !errors.Is(err, ErrRecoveryDenied) {
				t.Fatalf("malformed authority accepted: %v", err)
			}
			if !reflect.DeepEqual(f.steps, []string{"reserve-committed"}) {
				t.Fatalf("malformed authority reached router or cleanup: %v", f.steps)
			}
		})
	}
}

func TestRecoveryWorkIsBoundedByOriginalExpiryAndCallerDeadline(t *testing.T) {
	for _, callerDeadline := range []bool{false, true} {
		f, req, s := newRecoveryFixture(t)
		ctx := context.Background()
		want := f.claim.ExpiresAt
		if callerDeadline {
			want = time.Now().Add(time.Minute)
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, want)
			defer cancel()
		}
		started := time.Now()
		if _, err := s.Restore(ctx, req); err != nil {
			t.Fatal(err)
		}
		if len(f.workContexts) != 4 {
			t.Fatalf("missing work steps: %v", f.steps)
		}
		for _, work := range f.workContexts {
			deadline, ok := work.Deadline()
			if !ok || deadline.After(want) || deadline.After(started.Add(time.Minute+time.Second)) {
				t.Fatalf("work escaped original expiry/caller deadline: %v", deadline)
			}
		}
	}
}

func TestRecoveryCancellationOrExpiryNeverConfirmsAccess(t *testing.T) {
	for _, stage := range []string{"stop", "freeze", "complete", "expired-stop", "expired-freeze", "expired-complete", "corrupt-freeze"} {
		t.Run(stage, func(t *testing.T) {
			f, req, s := newRecoveryFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			expire := func() { f.now = f.claim.ExpiresAt }
			switch stage {
			case "stop":
				f.afterStop = cancel
			case "freeze":
				f.afterFreeze = cancel
			case "complete":
				f.afterComplete = cancel
			case "expired-stop":
				f.afterStop = expire
			case "expired-freeze":
				f.afterFreeze = expire
			case "expired-complete":
				f.afterComplete = expire
			case "corrupt-freeze":
				f.corruptFrozen = true
			}
			if _, err := s.Restore(ctx, req); !errors.Is(err, ErrRecoveryNeedsRepair) {
				t.Fatalf("uncertain recovery reported success: %v", err)
			}
			if !reflect.DeepEqual(f.steps[len(f.steps)-2:], []string{"contained", "blocked"}) || !f.blockContextValid {
				t.Fatalf("failure escaped independent cleanup: %v", f.steps)
			}
			if (stage == "stop" || stage == "freeze" || stage == "expired-stop" || stage == "expired-freeze" || stage == "corrupt-freeze") && f.remaining != 0 {
				t.Fatal("new binding enabled after lost authority")
			}
		})
	}
}

func TestRecoveryMissingScopeOrCanceledCallerNeverReserves(t *testing.T) {
	for _, missing := range []string{"tenant", "gateway", "voucher", "canceled"} {
		f, req, s := newRecoveryFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		switch missing {
		case "tenant":
			req.BusinessID = ""
		case "gateway":
			req.DeviceID = ""
		case "voucher":
			req.VoucherID = ""
		case "canceled":
			cancel()
		}
		_, err := s.Restore(ctx, req)
		cancel()
		if !errors.Is(err, ErrRecoveryDenied) || len(f.steps) != 0 {
			t.Fatalf("invalid request reserved access: %s %v %v", missing, err, f.steps)
		}
	}
}
func TestRecoveryStopsOldBeforeGrantAndNeverReplenishes(t *testing.T) {
	f, req, s := newRecoveryFixture(t)
	g, err := s.Restore(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if f.remaining != 35000000 || g.UsedBytes != 40000000 || !g.ExpiresAt.Equal(f.now.Add(time.Hour)) {
		t.Fatal("purchase quota or deadline changed")
	}
	want := []string{"reserve-committed", "old-session-stopped-and-verified", "usage-committed", "new-session-verified", "binding-committed"}
	if !reflect.DeepEqual(f.steps, want) {
		t.Fatalf("unsafe operation order: %v", f.steps)
	}
}
func TestRecoveryFailureContainsAndDoesNotClaimSuccess(t *testing.T) {
	for _, failure := range []string{"reserve", "stop", "freeze", "activate", "expired", "complete"} {
		t.Run(failure, func(t *testing.T) {
			f, req, s := newRecoveryFixture(t)
			f.fail = failure
			if _, err := s.Restore(context.Background(), req); err == nil {
				t.Fatal("failure reported success")
			}
			if failure == "reserve" {
				if len(f.steps) != 1 {
					t.Fatal("unproven owner touched router")
				}
				return
			}
			if !reflect.DeepEqual(f.steps[len(f.steps)-2:], []string{"contained", "blocked"}) {
				t.Fatal("uncertain remote changes not contained")
			}
			if f.containContext.Err() == nil {
				t.Fatal("containment context not closed before independent durable fence")
			}
			if !f.blockContextValid {
				t.Fatal("durable fence did not receive a fresh live context")
			}
		})
	}
}
func TestRecoverySecretCannotBeGuessedFromPurchaseOrMAC(t *testing.T) {
	a, err := RecoverySecret()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := RecoverySecret()
	ah, _ := HashSecret(a)
	bh, _ := HashSecret(b)
	if a == b || bytes.Equal(ah, bh) {
		t.Fatal("credential reused")
	}
	for _, invalid := range []string{"", "purchase", "AA:BB:CC:DD:EE:FE", a + "="} {
		if _, err := HashSecret(invalid); err == nil {
			t.Fatal("invalid credential accepted")
		}
	}
}
func TestGraceIdentityCannotResetCooldownWithDifferentMAC(t *testing.T) {
	now := time.Now()
	retry := now.Add(6 * time.Hour)
	p := IdentityProof{BusinessID: "biz", SubjectID: "verified-subject", VerifiedAt: now, ValidUntil: now.Add(24 * time.Hour)}
	if p.EligibleForGrace("biz", now, &retry) {
		t.Fatal("cooldown bypassed")
	}
	if !p.EligibleForGrace("biz", retry, &retry) {
		t.Fatal("exact retry boundary denied")
	}
	if p.EligibleForGrace("other", retry, &retry) {
		t.Fatal("foreign tenant proof accepted")
	}
	if (IdentityProof{BusinessID: "biz", SubjectID: "typed-phone"}).EligibleForGrace("biz", now, nil) {
		t.Fatal("unverified identity accepted")
	}
	// There is deliberately no MAC/IP/browser field in grace eligibility.
}
