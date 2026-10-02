package guestaccess

import (
	"context"
	"errors"
	"testing"
	"time"

	"mega-magic/internal/model"
)

type provisionFixture struct {
	g             model.GuestVoucherGrant
	events        []string
	ensureErr     error
	finalizeOK    bool
	advance       func()
	afterFinalize func()
}

type credentialIssuerStub struct {
	f      *provisionFixture
	secret string
	err    error
	after  func()
}

func (s credentialIssuerStub) Issue(ctx context.Context, g model.GuestVoucherGrant) (string, error) {
	s.f.events = append(s.f.events, "credential-committed")
	if ctx.Err() != nil || g.State != "active" || g.LeaseToken != "" || g.LeaseUntil != nil {
		return "", errors.New("unverified issuance")
	}
	if s.after != nil {
		s.after()
	}
	return s.secret, s.err
}

func TestProvisionCredentialsOnlyAfterVerifiedFinalization(t *testing.T) {
	for _, mode := range []string{"first", "retry", "failed", "malformed", "canceled", "finalize-failed"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Now()
			expires, lease := now.Add(time.Minute), now.Add(30*time.Second)
			f := &provisionFixture{g: model.GuestVoucherGrant{BusinessID: "business", DeviceID: "gateway", VoucherID: "voucher", ClientMAC: "02:00:00:00:00:01", State: "provisioning", Minutes: 1, DataMB: 10, ExpiresAt: &expires, LeaseUntil: &lease, LeaseToken: "worker"}, finalizeOK: true}
			secret, err := RecoverySecret()
			if err != nil {
				t.Fatal(err)
			}
			issuer := credentialIssuerStub{f: f, secret: secret}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "retry":
				issuer.secret = ""
			case "failed":
				issuer.err = errors.New("ambiguous commit")
			case "malformed":
				issuer.secret = "CODE"
			case "canceled":
				issuer.after = cancel
			case "finalize-failed":
				f.finalizeOK = false
			}
			s := ProvisionService{Store: f, Router: f, Credentials: issuer}
			g, err := s.Redeem(ctx, ProvisionRequest{BusinessID: "business", DeviceID: "gateway", Code: "CODE", ClientMAC: f.g.ClientMAC})
			if mode == "first" || mode == "retry" {
				if err != nil || g.RecoverySecret != issuer.secret || len(f.events) != 4 || f.events[2] != "finalize" || f.events[3] != "credential-committed" {
					t.Fatal("credential disclosed before finalization or lost on success")
				}
			} else {
				if !errors.Is(err, ErrProvisionNeedsRepair) || g.RecoverySecret != "" {
					t.Fatal("failed issuance disclosed credential or access")
				}
				n := len(f.events)
				if f.events[n-2] != "contain" || f.events[n-1] != "block" {
					t.Fatal("issuance failure not fenced")
				}
				if mode == "finalize-failed" && n != 5 {
					t.Fatal("unverified grant reached issuer")
				}
			}
		})
	}
}

func (f *provisionFixture) Reserve(_ context.Context, _ ProvisionRequest) (model.GuestVoucherGrant, error) {
	f.events = append(f.events, "reserve-committed")
	return f.g, nil
}
func (f *provisionFixture) Finalize(_ context.Context, _ model.GuestVoucherGrant) (bool, error) {
	f.events = append(f.events, "finalize")
	if f.afterFinalize != nil {
		f.afterFinalize()
	}
	return f.finalizeOK, nil
}

func TestProvisionFinalizationCannotMaskExpiryLeaseOrCancellation(t *testing.T) {
	for _, state := range []string{"active", "provisioning"} {
		for _, failure := range []string{"expiry", "lease", "cancel"} {
			t.Run(state+"/"+failure, func(t *testing.T) {
				now := time.Now()
				expires, lease := now.Add(time.Minute), now.Add(30*time.Second)
				f := &provisionFixture{g: model.GuestVoucherGrant{BusinessID: "business", DeviceID: "gateway", VoucherID: "voucher", ClientMAC: "02:00:00:00:00:01", State: state, Minutes: 1, DataMB: 10, ExpiresAt: &expires, LeaseUntil: &lease, LeaseToken: "worker"}, finalizeOK: true}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				f.afterFinalize = func() {
					switch failure {
					case "expiry":
						now = expires
					case "lease":
						now = lease
					case "cancel":
						cancel()
					}
				}
				g, err := (ProvisionService{Store: f, Router: f, Now: func() time.Time { return now }}).Redeem(ctx, ProvisionRequest{BusinessID: "business", DeviceID: "gateway", Code: "CODE", ClientMAC: f.g.ClientMAC})
				if state == "active" && failure == "lease" {
					if err != nil || g.State != "active" {
						t.Fatalf("active binding incorrectly uses stale worker lease: %v", err)
					}
					return
				}
				if !errors.Is(err, ErrProvisionNeedsRepair) || g.State != "" {
					t.Fatalf("expired/canceled finalization confirmed access: %+v %v", g, err)
				}
				n := len(f.events)
				if n != 5 || f.events[n-2] != "contain" || f.events[n-1] != "block" {
					t.Fatalf("missing failure fence: %v", f.events)
				}
			})
		}
	}
}

func TestProvisionCanceledCallerDoesNotReserve(t *testing.T) {
	f := &provisionFixture{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (ProvisionService{Store: f, Router: f}).Redeem(ctx, ProvisionRequest{BusinessID: "business", DeviceID: "gateway", Code: "CODE", ClientMAC: "02:00:00:00:00:01"})
	if !errors.Is(err, ErrProvisionDenied) || len(f.events) != 0 {
		t.Fatalf("canceled request touched storage: %v %v", err, f.events)
	}
}
func (f *provisionFixture) Block(ctx context.Context, _ model.GuestVoucherGrant) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	f.events = append(f.events, "block")
	return nil
}
func (f *provisionFixture) Ensure(_ context.Context, _ model.GuestVoucherGrant) error {
	f.events = append(f.events, "router")
	if f.advance != nil {
		f.advance()
	}
	return f.ensureErr
}
func (f *provisionFixture) Contain(ctx context.Context, _ model.GuestVoucherGrant) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	f.events = append(f.events, "contain")
	return nil
}

func TestProvisionOrderingAndFailureFence(t *testing.T) {
	for _, tc := range []struct {
		name      string
		routerErr bool
		revoked   bool
		expired   bool
		active    bool
	}{
		{name: "success"}, {name: "uncertain router", routerErr: true}, {name: "revoke race", revoked: true}, {name: "expiry during IO", expired: true}, {name: "active resume", active: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			expires := now.Add(time.Minute)
			lease := now.Add(30 * time.Second)
			f := &provisionFixture{g: model.GuestVoucherGrant{BusinessID: "business", DeviceID: "gateway", VoucherID: "voucher", ClientMAC: "02:00:00:00:00:01", State: "provisioning", Minutes: 1, DataMB: 10, ExpiresAt: &expires, LeaseUntil: &lease, LeaseToken: "worker"}, finalizeOK: !tc.revoked}
			if tc.active {
				f.g.State = "active"
				f.g.LeaseToken = ""
				f.g.LeaseUntil = nil
			}
			if tc.routerErr {
				f.ensureErr = errors.New("timeout after write")
			}
			if tc.expired {
				f.advance = func() { now = expires }
			}
			s := ProvisionService{Store: f, Router: f, Now: func() time.Time { return now }}
			g, err := s.Redeem(context.Background(), ProvisionRequest{BusinessID: "business", DeviceID: "gateway", Code: "CODE", ClientMAC: "02:00:00:00:00:01"})
			failure := tc.routerErr || tc.revoked || tc.expired
			if failure {
				if !errors.Is(err, ErrProvisionNeedsRepair) {
					t.Fatalf("error %v", err)
				}
				n := len(f.events)
				if f.events[n-2] != "contain" || f.events[n-1] != "block" {
					t.Fatalf("events %v", f.events)
				}
			} else {
				if err != nil || g.State != "active" || g.DataMB != 10 || !g.ExpiresAt.Equal(expires) {
					t.Fatalf("result %+v %v", g, err)
				}
				if len(f.events) != 3 || f.events[0] != "reserve-committed" || f.events[1] != "router" || f.events[2] != "finalize" {
					t.Fatalf("events %v", f.events)
				}
			}
		})
	}
}

func TestProvisionRejectsForeignStoreResultBeforeRouter(t *testing.T) {
	f := &provisionFixture{g: model.GuestVoucherGrant{BusinessID: "foreign"}}
	_, err := (ProvisionService{Store: f, Router: f}).Redeem(context.Background(), ProvisionRequest{BusinessID: "business", DeviceID: "gateway", Code: "CODE", ClientMAC: "02:00:00:00:00:01"})
	if !errors.Is(err, ErrProvisionDenied) || len(f.events) != 1 {
		t.Fatalf("%v %v", err, f.events)
	}
}
