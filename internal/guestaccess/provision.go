package guestaccess

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"mega-magic/internal/model"
)

var ErrProvisionDenied = errors.New("guest access is unavailable")
var ErrProvisionNeedsRepair = errors.New("guest access requires reconciliation")

type ProvisionRequest struct {
	BusinessID, DeviceID, Code, ClientMAC string
}

// Reserve must commit its tenant-scoped claim before returning. Finalize must
// serialize with revocation and check the worker lease and immutable expiry.
// Block must durably fence uncertain access, never release it for a new grant.
type ProvisionStore interface {
	Reserve(context.Context, ProvisionRequest) (model.GuestVoucherGrant, error)
	Finalize(context.Context, model.GuestVoucherGrant) (bool, error)
	Block(context.Context, model.GuestVoucherGrant) error
}

type ProvisionRouter interface {
	// Ensure verifies gateway ownership, binding, remaining usage and deadline;
	// an existing user must not have its counters or deadline reset.
	Ensure(context.Context, model.GuestVoucherGrant) error
	Contain(context.Context, model.GuestVoucherGrant) error
}

// Issue returns a new secret only after a short committed transaction verifies
// the finalized grant. A retry must return an empty secret, never rotate an
// existing credential based on a voucher code or MAC alone.
type RecoveryCredentialIssuer interface {
	Issue(context.Context, model.GuestVoucherGrant) (string, error)
}

type ProvisionService struct {
	Store       ProvisionStore
	Router      ProvisionRouter
	Now         func() time.Time
	Credentials RecoveryCredentialIssuer
}

func (s ProvisionService) Redeem(ctx context.Context, req ProvisionRequest) (model.GuestVoucherGrant, error) {
	mac, err := net.ParseMAC(req.ClientMAC)
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" ||
		req.BusinessID == "" || req.DeviceID == "" || strings.TrimSpace(req.Code) == "" || s.Store == nil || s.Router == nil || ctx.Err() != nil {
		return model.GuestVoucherGrant{}, ErrProvisionDenied
	}
	req.ClientMAC = strings.ToUpper(mac.String())
	g, err := s.Store.Reserve(ctx, req)
	if err != nil {
		return model.GuestVoucherGrant{}, err
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	fail := func() (model.GuestVoucherGrant, error) {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = s.Router.Contain(cleanup, g)
		cancel()
		block, cancelBlock := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelBlock()
		_ = s.Store.Block(block, g)
		return model.GuestVoucherGrant{}, ErrProvisionNeedsRepair
	}
	// Never send inconsistent store data to a router, even for containment.
	if g.BusinessID != req.BusinessID || g.DeviceID != req.DeviceID || g.ClientMAC != req.ClientMAC || g.VoucherID == "" {
		return model.GuestVoucherGrant{}, ErrProvisionDenied
	}
	// Stored grants cannot supply plaintext credentials. Only the post-readback
	// issuer may populate this transient response field.
	g.RecoverySecret = ""
	if g.ExpiresAt == nil || !now().Before(*g.ExpiresAt) || g.Minutes < 1 || g.DataMB < 1 ||
		(g.State != "active" && g.State != "provisioning") ||
		(g.State == "provisioning" && (g.LeaseToken == "" || g.LeaseUntil == nil || !now().Before(*g.LeaseUntil))) {
		return fail()
	}
	// Finish router I/O before the fenced lease ends. A delayed worker must not
	// return success or acquire another allowance after its deadline.
	deadline := *g.ExpiresAt
	if g.State == "provisioning" && g.LeaseUntil.Before(deadline) {
		deadline = *g.LeaseUntil
	}
	work, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if work.Err() != nil {
		return fail()
	}
	if err = s.Router.Ensure(work, g); err != nil || work.Err() != nil || !now().Before(deadline) {
		return fail()
	}
	// Re-check authoritative revocation/expiry after router readback even for
	// retries. A successful commit must not mask a canceled or expired worker.
	ok, err := s.Store.Finalize(work, g)
	if err != nil || !ok || work.Err() != nil || !now().Before(deadline) {
		return fail()
	}
	g.State, g.LeaseToken, g.LeaseUntil = "active", "", nil
	if s.Credentials != nil {
		secret, issueErr := s.Credentials.Issue(work, g)
		if issueErr != nil || work.Err() != nil || !now().Before(deadline) {
			return fail()
		}
		if secret != "" {
			if _, issueErr = HashSecret(secret); issueErr != nil {
				return fail()
			}
		}
		g.RecoverySecret = secret
	}
	return g, nil
}
