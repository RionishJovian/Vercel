package guestaccess

import (
	"context"
	"net"
	"strings"
	"time"
)

// BindingSnapshot is authority for the CURRENT connection, not the immutable
// first-use MAC. Revision fences a transfer that changes during router readback.
type BindingSnapshot struct {
	BusinessID, DeviceID, VoucherID, ClientMAC string
	TotalBytes, PriorUsedBytes                 int64
	ExpiresAt, Revision                        time.Time
}

type BindingStore interface {
	// Lookup must verify the credential hash, live paid entitlement and current
	// MAC in a short committed transaction. It must not mutate any allowance.
	Lookup(context.Context, RecoveryRequest, []byte) (BindingSnapshot, error)
}

type BindingVerifier interface {
	// Verify must read the existing owned router user, current host, fixed expiry
	// guard and allocated quota. It must not create, enable or reset a user.
	// Return verified consumption for this binding (not prior transferred usage).
	Verify(context.Context, BindingSnapshot) (int64, error)
}

type ResumeResult struct {
	ExpiresAt      time.Time
	RemainingBytes int64
}

type ResumeService struct {
	Store  BindingStore
	Router BindingVerifier
	Now    func() time.Time
}

// Resume verifies an existing same-MAC binding. It is deliberately distinct
// from transfer: no rate-window reset, grant provisioning or free grace path.
func (s ResumeService) Resume(ctx context.Context, req RecoveryRequest) (ResumeResult, error) {
	mac, err := net.ParseMAC(req.NewMAC)
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" || req.BusinessID == "" || req.DeviceID == "" || req.VoucherID == "" || ctx.Err() != nil || s.Store == nil || s.Router == nil {
		return ResumeResult{}, ErrRecoveryDenied
	}
	req.NewMAC = strings.ToUpper(mac.String())
	hash, err := HashSecret(req.Secret)
	if err != nil {
		return ResumeResult{}, ErrRecoveryDenied
	}
	req.Secret = ""
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	g, err := s.Store.Lookup(ctx, req, hash)
	if err != nil || g.BusinessID != req.BusinessID || g.DeviceID != req.DeviceID || g.VoucherID != req.VoucherID || g.ClientMAC != req.NewMAC || g.Revision.IsZero() || g.TotalBytes <= 0 || g.PriorUsedBytes < 0 || g.PriorUsedBytes >= g.TotalBytes || !now().Before(g.ExpiresAt) {
		return ResumeResult{}, ErrRecoveryDenied
	}
	work, cancel := context.WithDeadline(ctx, g.ExpiresAt)
	defer cancel()
	if work.Err() != nil {
		return ResumeResult{}, ErrRecoveryDenied
	}
	used, err := s.Router.Verify(work, g)
	if err != nil || used < 0 || used >= g.TotalBytes-g.PriorUsedBytes || work.Err() != nil || !now().Before(g.ExpiresAt) {
		return ResumeResult{}, ErrRecoveryDenied
	}
	// Re-read after remote I/O, without holding database locks over a network
	// call. A changed ledger or revision invalidates even a valid router readback.
	latest, err := s.Store.Lookup(work, req, hash)
	if err != nil || latest != g || work.Err() != nil || !now().Before(g.ExpiresAt) {
		return ResumeResult{}, ErrRecoveryDenied
	}
	return ResumeResult{ExpiresAt: g.ExpiresAt, RemainingBytes: g.TotalBytes - g.PriorUsedBytes - used}, nil
}
