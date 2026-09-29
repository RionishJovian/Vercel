package provisioner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// recordApplier is a fake Applier that records the call sequence and can be
// told to fail Apply/Snapshot/Restore or to block Restore until the deadline.
type recordApplier struct {
	calls          []string
	snapshotErr    error
	applyErr       error
	restoreErr     error
	restoreBlocks  bool // Restore waits for ctx cancellation (deadline test)
	snapshotConfig string
	onApply        func()
	restoreCtxErr  error
}

func (a *recordApplier) Snapshot(_ context.Context, _, peerID string, changes []Change) (RollbackSnapshot, error) {
	a.calls = append(a.calls, "snapshot")
	if a.snapshotErr != nil {
		return RollbackSnapshot{}, a.snapshotErr
	}
	return RollbackSnapshot{
		PeerID:          peerID,
		CapturedAt:      time.Now().UTC(),
		WireGuardConfig: a.snapshotConfig,
		Protecting:      changes,
	}, nil
}

func (a *recordApplier) Apply(_ context.Context, _, _ string, _ []Change) error {
	a.calls = append(a.calls, "apply")
	if a.onApply != nil {
		a.onApply()
	}
	return a.applyErr
}

func (a *recordApplier) Restore(ctx context.Context, _ string, _ RollbackSnapshot) error {
	a.calls = append(a.calls, "restore")
	a.restoreCtxErr = ctx.Err()
	if a.restoreBlocks {
		<-ctx.Done()
		return ctx.Err()
	}
	return a.restoreErr
}

func TestApplyFixes_rollbackSurvivesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ap := &recordApplier{applyErr: errors.New("partial mutation"), onApply: cancel}
	svc := NewApplyService(ap, upProber()).WithRollbackWindow(time.Second)

	res, err := svc.ApplyFixes(ctx, ApplyRequest{PeerID: "peer-cancel", Changes: lowMedPlan()})
	if err != nil {
		t.Fatalf("ApplyFixes: %v", err)
	}
	if !res.RolledBack || !ap.called("restore") {
		t.Fatalf("cancelled apply must still restore: res=%+v calls=%v", res, ap.calls)
	}
	if ap.restoreCtxErr != nil {
		t.Fatalf("restore inherited cancelled caller context: %v", ap.restoreCtxErr)
	}
}

func (a *recordApplier) called(name string) bool {
	for _, c := range a.calls {
		if c == name {
			return true
		}
	}
	return false
}

func upProber() Prober {
	return StaticProber{
		Obs:  Observation{State: StateUp, Keepalive: ExpectedKeepalive, KeepaliveKnown: true},
		Conn: Connectivity{Checked: true, Reachable: true, Endpoint: "10.0.0.1:443"},
	}
}

func downProber() Prober {
	return StaticProber{
		Obs:  Observation{State: StateOffline, Keepalive: ExpectedKeepalive, KeepaliveKnown: true},
		Conn: Connectivity{Checked: true, Reachable: false, Endpoint: "10.0.0.1:443"},
	}
}

func TestApplyFixes_happyPath(t *testing.T) {
	ap := &recordApplier{snapshotConfig: "wg-pre"}
	svc := NewApplyService(ap, upProber())
	changes := lowMedPlan()

	res, err := svc.ApplyFixes(context.Background(), ApplyRequest{PeerID: "peer-1", Changes: changes})
	if err != nil {
		t.Fatalf("ApplyFixes: %v", err)
	}
	if !res.SnapshotCaptured || !res.Verified || res.RolledBack {
		t.Fatalf("happy path result = %+v, want snapshot+verified, no rollback", res)
	}
	if res.DeadlineExceeded {
		t.Fatal("no rollback happened, deadline must not be flagged")
	}
	if !ap.called("snapshot") || !ap.called("apply") {
		t.Fatalf("expected snapshot then apply, calls = %v", ap.calls)
	}
	if ap.called("restore") {
		t.Fatalf("happy path must not restore, calls = %v", ap.calls)
	}
	if got := actionsOf(res.Applied); len(got) != len(changes) {
		t.Fatalf("applied changes mismatch: %v vs %v", got, actionsOf(changes))
	}
}

func TestApplyFixes_verifyFailRollsBack(t *testing.T) {
	ap := &recordApplier{}
	svc := NewApplyService(ap, downProber()).WithRollbackWindow(2 * time.Second)

	res, err := svc.ApplyFixes(context.Background(), ApplyRequest{PeerID: "peer-2", Changes: lowMedPlan()})
	if err != nil {
		t.Fatalf("ApplyFixes: %v", err)
	}
	if res.Verified || !res.RolledBack || !res.SnapshotCaptured {
		t.Fatalf("verify-fail result = %+v, want rolled_back and not verified", res)
	}
	if !ap.called("restore") {
		t.Fatalf("verify failure must restore the snapshot, calls = %v", ap.calls)
	}
	if res.RollbackDetail == "" {
		t.Fatal("rollback detail must be recorded")
	}
	if res.DeadlineExceeded {
		t.Fatal("restore completed in-window; deadline must not be flagged")
	}
}

func TestApplyFixes_applyErrorRollsBack(t *testing.T) {
	ap := &recordApplier{applyErr: errors.New("router refused")}
	svc := NewApplyService(ap, upProber()).WithRollbackWindow(2 * time.Second)

	res, err := svc.ApplyFixes(context.Background(), ApplyRequest{PeerID: "peer-3", Changes: lowMedPlan()})
	if err != nil {
		t.Fatalf("ApplyFixes: %v", err)
	}
	if !res.RolledBack || res.Verified {
		t.Fatalf("apply-error result = %+v, want rolled_back", res)
	}
	if !ap.called("restore") {
		t.Fatalf("failed apply must restore, calls = %v", ap.calls)
	}
}

func TestApplyFixes_rollbackDeadlineEnforced(t *testing.T) {
	ap := &recordApplier{restoreBlocks: true}
	window := 60 * time.Millisecond
	svc := NewApplyService(ap, downProber()).WithRollbackWindow(window)

	start := time.Now()
	res, err := svc.ApplyFixes(context.Background(), ApplyRequest{PeerID: "peer-4", Changes: lowMedPlan()})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ApplyFixes: %v", err)
	}
	if !res.RolledBack || !res.DeadlineExceeded {
		t.Fatalf("stuck restore result = %+v, want rolled_back with deadline exceeded", res)
	}
	if elapsed < window {
		t.Fatalf("deadline fired early: elapsed %v < window %v", elapsed, window)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("deadline not enforced promptly: elapsed %v", elapsed)
	}
}

func TestApplyFixes_defaultWindowIs60s(t *testing.T) {
	if NewApplyService(&recordApplier{}, upProber()).window != DefaultRollbackWindow {
		t.Fatalf("default rollback window = %v, want %v", NewApplyService(&recordApplier{}, upProber()).window, DefaultRollbackWindow)
	}
	if DefaultRollbackWindow != 60*time.Second {
		t.Fatalf("DefaultRollbackWindow = %v, want 60s (DESIGN section 4)", DefaultRollbackWindow)
	}
}

func TestApplyFixes_neverAppliesWithoutSnapshot(t *testing.T) {
	ap := &recordApplier{snapshotErr: errors.New("cannot read config")}
	svc := NewApplyService(ap, upProber())

	if _, err := svc.ApplyFixes(context.Background(), ApplyRequest{PeerID: "peer-5", Changes: lowMedPlan()}); err == nil {
		t.Fatal("snapshot failure must abort the apply")
	}
	if ap.called("apply") {
		t.Fatalf("must never apply without a snapshot, calls = %v", ap.calls)
	}
}

func TestApplyFixes_noopAndValidation(t *testing.T) {
	ap := &recordApplier{}
	svc := NewApplyService(ap, upProber())

	// Empty plan: no snapshot, no apply, not an error.
	res, err := svc.ApplyFixes(context.Background(), ApplyRequest{PeerID: "peer-6"})
	if err != nil {
		t.Fatalf("empty plan should not error: %v", err)
	}
	if res.SnapshotCaptured || res.Verified || len(ap.calls) != 0 {
		t.Fatalf("empty plan must touch nothing: res=%+v calls=%v", res, ap.calls)
	}

	// Missing peer id.
	if _, err := svc.ApplyFixes(context.Background(), ApplyRequest{}); err == nil {
		t.Fatal("missing peer id must error")
	}

	// No applier.
	if _, err := NewApplyService(nil, upProber()).ApplyFixes(context.Background(), ApplyRequest{PeerID: "x", Changes: lowMedPlan()}); err == nil {
		t.Fatalf("nil applier must error")
	}
}

// --- ApplyAction: explicit fire-and-validate (reboot), NO snapshot/rollback ---

func TestApplyAction_successNoSnapshotNoRestore(t *testing.T) {
	ap := &recordApplier{}
	svc := NewApplyService(ap, upProber())
	changes := []Change{{Action: ActionReboot, Target: "dev-1", Risk: "high"}}

	res, err := svc.ApplyAction(context.Background(), ApplyRequest{PeerID: "dev-1", Changes: changes})
	if err != nil {
		t.Fatalf("ApplyAction: %v", err)
	}
	if !res.Verified || res.RolledBack || res.SnapshotCaptured {
		t.Fatalf("ApplyAction result = %+v, want verified, no rollback, no snapshot", res)
	}
	if ap.called("snapshot") || ap.called("restore") {
		t.Fatalf("fire-and-validate must not snapshot/restore, calls = %v", ap.calls)
	}
	if !ap.called("apply") {
		t.Fatalf("expected apply, calls = %v", ap.calls)
	}
}

func TestApplyAction_verifyFailNoRollback(t *testing.T) {
	ap := &recordApplier{}
	svc := NewApplyService(ap, downProber())
	changes := []Change{{Action: ActionReboot, Target: "dev-1", Risk: "high"}}

	res, err := svc.ApplyAction(context.Background(), ApplyRequest{PeerID: "dev-1", Changes: changes})
	if err != nil {
		t.Fatalf("ApplyAction: %v", err)
	}
	if res.Verified || res.RolledBack {
		t.Fatalf("ApplyAction failed-verify = %+v, want unverified and NOT rolled back (no config to revert)", res)
	}
	if ap.called("restore") || ap.called("snapshot") {
		t.Fatalf("fire-and-validate must never restore/snapshot, calls = %v", ap.calls)
	}
	if res.VerificationDetail == "" {
		t.Fatal("failed verify must record a detail")
	}
}

func TestApplyAction_applyErrorRecordsButNoRestore(t *testing.T) {
	ap := &recordApplier{applyErr: errors.New("router unreachable")}
	svc := NewApplyService(ap, upProber())
	changes := []Change{{Action: ActionReboot, Target: "dev-1", Risk: "high"}}

	res, err := svc.ApplyAction(context.Background(), ApplyRequest{PeerID: "dev-1", Changes: changes})
	if err != nil {
		t.Fatalf("ApplyAction: %v", err)
	}
	if res.Verified || res.RolledBack {
		t.Fatalf("ApplyAction apply-error = %+v, want unverified/no rollback", res)
	}
	if ap.called("restore") {
		t.Fatalf("a failed reboot must not attempt a config restore, calls = %v", ap.calls)
	}
	if !strings.Contains(res.VerificationDetail, "action failed") {
		t.Fatalf("expected a failure detail, got %q", res.VerificationDetail)
	}
}

func TestApplyAction_validation(t *testing.T) {
	// Missing peer id.
	if _, err := NewApplyService(&recordApplier{}, upProber()).ApplyAction(context.Background(), ApplyRequest{}); err == nil {
		t.Fatal("missing peer id must error")
	}
	// No applier.
	if _, err := NewApplyService(nil, upProber()).ApplyAction(context.Background(), ApplyRequest{PeerID: "x", Changes: []Change{{Action: ActionReboot}}}); err == nil {
		t.Fatal("nil applier must error")
	}
	// Empty changes: no apply, not an error.
	ap := &recordApplier{}
	res, err := NewApplyService(ap, upProber()).ApplyAction(context.Background(), ApplyRequest{PeerID: "x"})
	if err != nil {
		t.Fatalf("empty ApplyAction should not error: %v", err)
	}
	if len(ap.calls) != 0 {
		t.Fatalf("empty action must touch nothing, calls = %v", ap.calls)
	}
	if res.Verified {
		t.Fatal("empty action cannot be verified")
	}
}
