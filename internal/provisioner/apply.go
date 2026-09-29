package provisioner

import (
	"context"
	"fmt"
	"time"
)

// DefaultRollbackWindow is the auto-revert deadline from DESIGN-provisioner.md
// section 4: a failed verification must roll back within 60 seconds.
const DefaultRollbackWindow = 60 * time.Second

// RollbackSnapshot is the pre-change state captured BEFORE any change is
// applied (DESIGN section 4: WireGuard config + IP routes), so a failed
// verification can be reverted exactly.
type RollbackSnapshot struct {
	PeerID          string    `json:"peer_id"`
	CapturedAt      time.Time `json:"captured_at"`
	WireGuardConfig string    `json:"wireguard_config"`
	IPRoutes        []string  `json:"ip_routes,omitempty"`
	// Protecting records the changes this snapshot protects.
	Protecting []Change `json:"protecting"`
	// On-device snapshot artifacts and the parsed WireGuard values the live SSH
	// Applier needs to revert in place. Empty for the in-memory/unit-test path;
	// populated by internal/provisioner/routeros.
	ExportFile           string `json:"export_file,omitempty"`
	BackupName           string `json:"backup_name,omitempty"`
	WGListenPort         int    `json:"wg_listen_port,omitempty"`
	WGPeerKeepalive      string `json:"wg_peer_keepalive,omitempty"`
	WGPeerAllowedAddress string `json:"wg_peer_allowed_address,omitempty"`
}

// Applier applies validated changes to a peer's router and restores a snapshot.
// Restore MUST also re-enable the peer. The real SSH/RouterOS transport is a
// LATER slice (out of scope, matching how dry-run shipped no real transport);
// this boundary is what makes the apply path unit-testable today.
type Applier interface {
	Snapshot(ctx context.Context, businessID, peerID string, changes []Change) (RollbackSnapshot, error)
	Apply(ctx context.Context, businessID, peerID string, changes []Change) error
	Restore(ctx context.Context, businessID string, snap RollbackSnapshot) error
}

// BoundOperation is one logical device operation pinned to a single resolved
// transport. Production implementations resolve the tenant/device target once
// and reuse this object for snapshot, mutation, verification, and rollback.
// This prevents a concurrent credential/address update from moving the middle
// of an operation to a different router.
type BoundOperation interface {
	Snapshot(ctx context.Context, changes []Change) (RollbackSnapshot, error)
	Apply(ctx context.Context, changes []Change) error
	Restore(ctx context.Context, snap RollbackSnapshot) error
	Observe(ctx context.Context) (Observation, Connectivity, error)
}

// OperationBinder is an optional stronger contract implemented by live
// transports. ApplyService falls back to a legacy adapter for in-memory/test
// appliers, while production RouterOS uses a bound transport.
type OperationBinder interface {
	BindOperation(ctx context.Context, businessID, peerID string) (BoundOperation, error)
}

// ApplyRequest is one apply attempt for a peer.
type ApplyRequest struct {
	BusinessID string
	PeerID     string
	Changes    []Change
}

// ApplyResult is the outcome of an apply attempt: what was applied, whether it
// verified, and whether (and how fast) it rolled back.
type ApplyResult struct {
	PeerID             string    `json:"peer_id"`
	Applied            []Change  `json:"applied_changes"`
	SnapshotCaptured   bool      `json:"snapshot_captured"`
	Verified           bool      `json:"verified"`
	VerificationDetail string    `json:"verification_detail"`
	RolledBack         bool      `json:"rolled_back"`
	RollbackDetail     string    `json:"rollback_detail,omitempty"`
	RollbackMs         int64     `json:"rollback_ms,omitempty"`
	DeadlineExceeded   bool      `json:"deadline_exceeded"`
	AppliedAt          time.Time `json:"applied_at"`
}

// ApplyService drives verify-before-apply and rollback-on-failure. It holds an
// injectable Applier and reuses the read-only Prober for verification.
type ApplyService struct {
	applier Applier
	prober  Prober
	window  time.Duration
}

// NewApplyService builds the apply service with the 60s rollback window.
func NewApplyService(applier Applier, prober Prober) *ApplyService {
	return &ApplyService{applier: applier, prober: prober, window: DefaultRollbackWindow}
}

// WithRollbackWindow overrides the rollback deadline (tests use a tiny window;
// production keeps the 60s default).
func (s *ApplyService) WithRollbackWindow(d time.Duration) *ApplyService {
	if d > 0 {
		s.window = d
	}
	return s
}

// ApplyFixes implements DESIGN sections 3 and 4:
//
//  1. snapshot the pre-change WG config + routes (never apply without a snapshot),
//  2. apply ONLY the validated changes,
//  3. verify via the read-only prober (tunnel up + connectivity from cloud),
//  4. on any failure, restore the snapshot and re-enable the peer within the
//     rollback window, flagging a deadline overrun.
//
// It returns a result (never a silent error) for every path except a missing
// peer id or a failed snapshot, which are pre-apply failures.
func (s *ApplyService) ApplyFixes(ctx context.Context, req ApplyRequest) (*ApplyResult, error) {
	if req.PeerID == "" {
		return nil, fmt.Errorf("provisioner: peer id is required")
	}
	if s.applier == nil {
		return nil, fmt.Errorf("provisioner: no applier configured")
	}

	res := &ApplyResult{
		PeerID:    req.PeerID,
		Applied:   req.Changes,
		AppliedAt: time.Now().UTC(),
	}
	if len(req.Changes) == 0 {
		res.VerificationDetail = "no changes to apply"
		return res, nil
	}
	op, err := s.bindOperation(ctx, req.BusinessID, req.PeerID)
	if err != nil {
		return nil, fmt.Errorf("provisioner: bind peer %s: %w", req.PeerID, err)
	}

	snap, err := op.Snapshot(ctx, req.Changes)
	if err != nil {
		return nil, fmt.Errorf("provisioner: snapshot peer %s: %w", req.PeerID, err)
	}
	res.SnapshotCaptured = true

	if err := op.Apply(ctx, req.Changes); err != nil {
		// A failed apply may have left partial state: revert to the snapshot.
		detail, exceeded, ms := s.rollback(ctx, op, snap)
		res.RolledBack = true
		res.DeadlineExceeded = exceeded
		res.RollbackMs = ms
		res.VerificationDetail = "not verified (apply error)"
		res.RollbackDetail = "apply failed: " + err.Error() + "; " + detail
		return res, nil
	}

	verified, vdetail, verr := s.verify(ctx, op)
	if verr == nil && verified {
		res.Verified = true
		res.VerificationDetail = vdetail
		return res, nil
	}
	if verr != nil {
		vdetail = "verify error: " + verr.Error()
	}
	res.VerificationDetail = vdetail

	rdetail, exceeded, ms := s.rollback(ctx, op, snap)
	res.RolledBack = true
	res.DeadlineExceeded = exceeded
	res.RollbackMs = ms
	res.RollbackDetail = rdetail
	return res, nil
}

// ApplyAction drives an EXPLICIT device action that is fire-and-validate: an
// owner-initiated command (e.g. reboot) that is applied then verified but is
// NOT a reversible config delta. Unlike ApplyFixes it takes no snapshot and can
// never roll back — a reboot has no "un-reboot"; the contract is apply the
// command, then confirm the device returns. Approval/OTP is the caller's
// (the consumer) responsibility; this only issues and verifies.
//
// It returns a result (never a silent error) for every path except a missing
// device id or a failed apply, which are pre-verify failures.
func (s *ApplyService) ApplyAction(ctx context.Context, req ApplyRequest) (*ApplyResult, error) {
	if req.PeerID == "" {
		return nil, fmt.Errorf("provisioner: peer id is required")
	}
	if s.applier == nil {
		return nil, fmt.Errorf("provisioner: no applier configured")
	}

	res := &ApplyResult{
		PeerID:    req.PeerID,
		Applied:   req.Changes,
		AppliedAt: time.Now().UTC(),
	}
	if len(req.Changes) == 0 {
		res.VerificationDetail = "no action to apply"
		return res, nil
	}
	op, err := s.bindOperation(ctx, req.BusinessID, req.PeerID)
	if err != nil {
		return nil, fmt.Errorf("provisioner: bind peer %s: %w", req.PeerID, err)
	}

	if err := op.Apply(ctx, req.Changes); err != nil {
		res.VerificationDetail = "action failed: " + err.Error()
		return res, nil
	}

	verified, vdetail, verr := s.verify(ctx, op)
	res.VerificationDetail = vdetail
	if verr == nil && verified {
		res.Verified = true
		return res, nil
	}
	if verr != nil {
		res.VerificationDetail = "verify error: " + verr.Error()
	}
	return res, nil
}

// verify confirms the tunnel is up AND the peer is reachable from the cloud,
// reusing the same read-only Prober the dry-run path uses.
func (s *ApplyService) verify(ctx context.Context, op BoundOperation) (bool, string, error) {
	obs, conn, err := op.Observe(ctx)
	if err != nil {
		return false, "", err
	}
	if obs.State == StateUp && conn.Checked && conn.Reachable {
		return true, "tunnel up and connectivity from cloud confirmed (state=up)", nil
	}
	return false, fmt.Sprintf("verification failed: state=%s connectivity_checked=%v reachable=%v", obs.State, conn.Checked, conn.Reachable), nil
}

// rollback restores the snapshot within the rollback window, returning the
// human detail, whether the deadline was exceeded, and how long it took.
func (s *ApplyService) rollback(ctx context.Context, op BoundOperation, snap RollbackSnapshot) (string, bool, int64) {
	window := s.window
	if window <= 0 {
		window = DefaultRollbackWindow
	}
	// Emergency restore must survive the HTTP/job cancellation that triggered
	// the failure. Preserve request values for auditing, but give rollback its
	// own strictly bounded lifetime.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), window)
	defer cancel()

	start := time.Now()
	err := op.Restore(rctx, snap)
	ms := time.Since(start).Milliseconds()

	if rctx.Err() == context.DeadlineExceeded {
		return fmt.Sprintf("rollback deadline (%s) exceeded while restoring peer %s", window, snap.PeerID), true, ms
	}
	if err != nil {
		return fmt.Sprintf("rollback failed for peer %s: %v", snap.PeerID, err), false, ms
	}
	return fmt.Sprintf("reverted to snapshot captured at %s and re-enabled peer %s",
		snap.CapturedAt.Format(time.RFC3339), snap.PeerID), false, ms
}

func (s *ApplyService) bindOperation(ctx context.Context, businessID, peerID string) (BoundOperation, error) {
	if binder, ok := s.applier.(OperationBinder); ok {
		return binder.BindOperation(ctx, businessID, peerID)
	}
	return &legacyBoundOperation{
		applier:    s.applier,
		prober:     s.prober,
		businessID: businessID,
		peerID:     peerID,
	}, nil
}

// legacyBoundOperation preserves the existing injectable seams for dry-run and
// unit-test implementations. Live RouterOS never uses this adapter.
type legacyBoundOperation struct {
	applier    Applier
	prober     Prober
	businessID string
	peerID     string
}

func (o *legacyBoundOperation) Snapshot(ctx context.Context, changes []Change) (RollbackSnapshot, error) {
	return o.applier.Snapshot(ctx, o.businessID, o.peerID, changes)
}

func (o *legacyBoundOperation) Apply(ctx context.Context, changes []Change) error {
	return o.applier.Apply(ctx, o.businessID, o.peerID, changes)
}

func (o *legacyBoundOperation) Restore(ctx context.Context, snap RollbackSnapshot) error {
	return o.applier.Restore(ctx, o.businessID, snap)
}

func (o *legacyBoundOperation) Observe(ctx context.Context) (Observation, Connectivity, error) {
	if o.prober == nil {
		return Observation{}, Connectivity{Checked: false}, nil
	}
	return o.prober.Observe(ctx, o.businessID, o.peerID)
}

var _ BoundOperation = (*legacyBoundOperation)(nil)
