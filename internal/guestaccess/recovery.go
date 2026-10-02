// Package guestaccess keeps purchase ownership separate from its current LAN
// connection. No MAC, typed phone number or voucher ID alone proves ownership.
package guestaccess

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net"
	"strings"
	"time"
)

var ErrRecoveryDenied = errors.New("access recovery unavailable")
var ErrRecoveryNeedsRepair = errors.New("access recovery needs reconciliation; no new access was confirmed")

// RecoverySecret must only be issued after purchase ownership and provisioning
// are verified. Return the plaintext once over HTTPS; store only HashSecret.
func RecoverySecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func HashSecret(secret string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(secret)
	if err != nil || len(b) != 32 || base64.RawURLEncoding.EncodeToString(b) != secret {
		return nil, ErrRecoveryDenied
	}
	h := sha256.Sum256(b)
	return h[:], nil
}

type RecoveryRequest struct {
	BusinessID, DeviceID, VoucherID string
	Secret                          string
	NewMAC                          string
}

type RecoveryClaim struct {
	BusinessID, DeviceID, VoucherID, TransferID string
	OldMAC, NewMAC                              string
	ExpiresAt                                   time.Time
	TotalBytes, UsedBytes                       int64
}

// Store implementations must commit each operation before returning. Reserve
// verifies the hashed secret, tenant/gateway, live purchase and transfer rate;
// serializes one transfer; and never changes the original allowance/deadline.
// A failed transfer stays blocked until reconciled, not silently retryable.
type RecoveryStore interface {
	Reserve(context.Context, RecoveryRequest, []byte) (RecoveryClaim, error)
	FreezeUsage(context.Context, RecoveryClaim, int64) (RecoveryClaim, error)
	Complete(context.Context, RecoveryClaim) error
	Block(context.Context, RecoveryClaim) error
}

// Router is intentionally a verified-operation seam, not an arbitrary command
// interface. StopAndRead must verify the old owned user is disabled, its active
// session/cookies revoked, then return FINAL bytes consumed by that binding.
// Activate must verify new host presence, remaining quota, fixed expiry and
// the old binding still disabled. No permissive IP binding is permitted.
type RecoveryRouter interface {
	StopAndRead(context.Context, RecoveryClaim) (int64, error)
	Activate(context.Context, RecoveryClaim, int64) error
	Contain(context.Context, RecoveryClaim) error
}

type RecoveryService struct {
	Store  RecoveryStore
	Router RecoveryRouter
	Now    func() time.Time
}

func (s RecoveryService) Restore(ctx context.Context, req RecoveryRequest) (RecoveryClaim, error) {
	mac, err := net.ParseMAC(req.NewMAC)
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
		return RecoveryClaim{}, ErrRecoveryDenied
	}
	req.NewMAC = strings.ToUpper(mac.String())
	hash, err := HashSecret(req.Secret)
	if err != nil || s.Store == nil || s.Router == nil || req.BusinessID == "" || req.DeviceID == "" || req.VoucherID == "" || ctx.Err() != nil {
		return RecoveryClaim{}, ErrRecoveryDenied
	}
	// Never pass the raw secret into storage or router operations.
	req.Secret = ""
	g, err := s.Store.Reserve(ctx, req, hash)
	if err != nil {
		return RecoveryClaim{}, ErrRecoveryDenied
	}
	// An inconsistent result is not authority to touch another tenant, gateway
	// or MAC, including through cleanup. Leave reconciliation to an owner-scoped
	// operation instead of trusting a malformed claim returned by storage.
	oldMAC, oldErr := net.ParseMAC(g.OldMAC)
	if g.NewMAC != req.NewMAC || g.BusinessID != req.BusinessID || g.DeviceID != req.DeviceID || g.VoucherID != req.VoucherID || g.TransferID == "" ||
		oldErr != nil || len(oldMAC) != 6 || oldMAC[0]&1 != 0 || oldMAC.String() == "00:00:00:00:00:00" || strings.ToUpper(oldMAC.String()) != g.OldMAC || g.OldMAC == g.NewMAC {
		return RecoveryClaim{}, ErrRecoveryDenied
	}
	fail := func() (RecoveryClaim, error) {
		// Remote writes may have succeeded even if their request timed out.
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = s.Router.Contain(cleanup, g)
		cancel()
		// An offline router may consume the whole cleanup deadline. Give the
		// durable failure fence its own budget, independent of router I/O.
		block, cancelBlock := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelBlock()
		_ = s.Store.Block(block, g)
		return RecoveryClaim{}, ErrRecoveryNeedsRepair
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	if !now().Before(g.ExpiresAt) || g.TotalBytes <= 0 || g.UsedBytes < 0 || g.UsedBytes >= g.TotalBytes {
		return fail()
	}
	// The purchase deadline also bounds every remote and durable work step.
	// Cleanup has separate budgets so a canceled browser request cannot leave
	// uncertain access unfenced.
	deadline := time.Now().Add(time.Minute)
	if g.ExpiresAt.Before(deadline) {
		deadline = g.ExpiresAt
	}
	work, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if work.Err() != nil {
		return fail()
	}
	consumed, err := s.Router.StopAndRead(work, g)
	if err != nil || work.Err() != nil || !now().Before(g.ExpiresAt) || consumed < 0 || consumed >= g.TotalBytes-g.UsedBytes {
		return fail()
	}
	prior := g
	g, err = s.Store.FreezeUsage(work, g, consumed)
	// Persist cumulative consumption exactly once BEFORE enabling a new MAC.
	if err != nil || g.BusinessID != prior.BusinessID || g.DeviceID != prior.DeviceID || g.VoucherID != prior.VoucherID || g.TransferID != prior.TransferID || g.OldMAC != prior.OldMAC || g.NewMAC != prior.NewMAC || !g.ExpiresAt.Equal(prior.ExpiresAt) || g.TotalBytes != prior.TotalBytes || g.UsedBytes != prior.UsedBytes+consumed {
		g = prior
		return fail()
	}
	if work.Err() != nil || !now().Before(g.ExpiresAt) {
		return fail()
	}
	if err = s.Router.Activate(work, g, g.TotalBytes-g.UsedBytes); err != nil {
		return fail()
	}
	if work.Err() != nil || !now().Before(g.ExpiresAt) {
		return fail()
	}
	if err = s.Store.Complete(work, g); err != nil || work.Err() != nil || !now().Before(g.ExpiresAt) {
		return fail()
	}
	return g, nil
}

// IdentityProof is populated only by a server-side verifier after a real OTP or
// equivalent authenticated identity check. Never bind it from guest JSON.
type IdentityProof struct {
	BusinessID, SubjectID  string
	VerifiedAt, ValidUntil time.Time
}

func (p IdentityProof) EligibleForGrace(businessID string, now time.Time, retryAt *time.Time) bool {
	return p.BusinessID == businessID && p.SubjectID != "" && !p.VerifiedAt.IsZero() && !p.VerifiedAt.After(now) && now.Before(p.ValidUntil) && (retryAt == nil || !now.Before(*retryAt))
}
