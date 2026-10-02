package guestaccess

import (
	"context"
	"errors"
	"time"
)

// RepairStore fences stale transfers durably BEFORE returning their claims.
// It must return only claims belonging to the supplied tenant and never return
// active, successfully completed transfers. Blocked claims remain retryable for
// containment when a router is offline; this workflow never reopens access.
type RepairStore interface {
	FenceInterrupted(context.Context, string, int) ([]RecoveryClaim, error)
}

type RecoveryReconciler struct {
	Store  RepairStore
	Router interface {
		Contain(context.Context, RecoveryClaim) error
	}
}

type RepairResult struct{ Contained, Pending int }

// Reconcile is bounded and cancellation-aware. A caller supplies a server-owned
// tenant, not guest JSON. No credentials, secrets or raw router errors escape.
func (s RecoveryReconciler) Reconcile(ctx context.Context, businessID string) (RepairResult, error) {
	var result RepairResult
	if businessID == "" || s.Store == nil || s.Router == nil || ctx.Err() != nil {
		return result, ErrRecoveryDenied
	}
	claims, err := s.Store.FenceInterrupted(ctx, businessID, 25)
	if err != nil || len(claims) > 25 {
		return result, errors.New("recovery reconciliation unavailable")
	}
	for _, g := range claims {
		if g.BusinessID != businessID || g.DeviceID == "" || g.VoucherID == "" || g.TransferID == "" {
			return result, errors.New("invalid reconciliation scope")
		}
	}
	for _, g := range claims {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		work, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := s.Router.Contain(work, g)
		cancel()
		if err != nil {
			result.Pending++
		} else {
			result.Contained++
		}
	}
	return result, nil
}
