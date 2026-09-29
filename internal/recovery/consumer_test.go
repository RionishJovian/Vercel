package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"mega-magic/internal/provisioner"
)

// --- fakes ------------------------------------------------------------------

type fakePublisher struct {
	events []firedEvent
	err    error
}

type firedEvent struct {
	businessID string
	subject    string
	data       []byte
}

func (p *fakePublisher) Publish(businessID, subject string, data []byte) error {
	if p.err != nil {
		return p.err
	}
	p.events = append(p.events, firedEvent{businessID, subject, append([]byte(nil), data...)})
	return nil
}

func (p *fakePublisher) subjects() []string {
	out := make([]string, 0, len(p.events))
	for _, e := range p.events {
		out = append(out, e.subject)
	}
	return out
}

func (p *fakePublisher) has(subject string) bool {
	for _, e := range p.events {
		if e.subject == subject {
			return true
		}
	}
	return false
}

type stubApprover struct {
	dec ApprovalDec
	err error
}

type ApprovalDec = provisioner.ApprovalDecision

func (a stubApprover) Await(_ context.Context, _ string, _ provisioner.ApprovalRequest) (provisioner.ApprovalDecision, error) {
	return a.dec, a.err
}

type stubOtp struct{ code string }

func (s stubOtp) Verify(_ context.Context, _, code string) (bool, error) { return code == s.code, nil }

// seqProber returns observations in sequence, repeating the last one. This lets
// a test distinguish the plan-time probe from the post-apply verify probe.
type seqProber struct {
	obs   []provisioner.Observation
	index int
}

func (p *seqProber) Observe(_ context.Context, _, peerID string) (provisioner.Observation, provisioner.Connectivity, error) {
	if len(p.obs) == 0 {
		return provisioner.Observation{PeerID: peerID, State: provisioner.StateUnknown}, provisioner.Connectivity{Checked: true}, nil
	}
	o := p.obs[p.index]
	if p.index < len(p.obs)-1 {
		p.index++
	}
	o.PeerID = peerID
	return o, provisioner.Connectivity{Checked: true, Reachable: o.State == provisioner.StateUp, Endpoint: "peer:443"}, nil
}

type fakeApplier struct {
	snapped  int
	applied  int
	restored int
}

func (a *fakeApplier) Snapshot(_ context.Context, _, peerID string, changes []provisioner.Change) (provisioner.RollbackSnapshot, error) {
	a.snapped++
	return provisioner.RollbackSnapshot{PeerID: peerID, CapturedAt: time.Now().UTC(), Protecting: changes}, nil
}
func (a *fakeApplier) Apply(_ context.Context, _, _ string, _ []provisioner.Change) error {
	a.applied++
	return nil
}
func (a *fakeApplier) Restore(_ context.Context, _ string, _ provisioner.RollbackSnapshot) error {
	a.restored++
	return nil
}

// --- tests ------------------------------------------------------------------

func newConsumer(t *testing.T, prober provisioner.Prober, approver provisioner.Approver, otp provisioner.OtpVerifier, ap *fakeApplier, pub *fakePublisher) *ApplyConsumer {
	t.Helper()
	svc := provisioner.NewApplyService(ap, prober).WithRollbackWindow(time.Second)
	return NewApplyConsumer(svc, prober, approver, otp, pub, nil)
}

func staleThenUp() *seqProber {
	return &seqProber{obs: []provisioner.Observation{
		{State: provisioner.StateStale, Keepalive: provisioner.ExpectedKeepalive, KeepaliveKnown: true},
		{State: provisioner.StateUp, Keepalive: provisioner.ExpectedKeepalive, KeepaliveKnown: true},
	}}
}

func TestOnRecover_approvedLowRiskApplies(t *testing.T) {
	ap := &fakeApplier{}
	pub := &fakePublisher{}
	c := newConsumer(t, staleThenUp(), stubApprover{dec: ApprovalDec{Approved: true}}, stubOtp{code: "123456"}, ap, pub)

	out, err := c.OnRecover(context.Background(), "biz-1", "peer-1")
	if err != nil {
		t.Fatalf("OnRecover: %v", err)
	}
	if !out.Requested || !out.Approval.Allowed || !out.Applied {
		t.Fatalf("outcome = %+v, want requested+allowed+applied", out)
	}
	if ap.applied != 1 || ap.restored != 0 {
		t.Fatalf("applied=%d restored=%d, want 1/0", ap.applied, ap.restored)
	}
	for _, s := range []string{provisioner.SubjectActionRequested, provisioner.SubjectApprovalResult, provisioner.SubjectApplyResult} {
		if !pub.has(s) {
			t.Fatalf("missing event %s; got %v", s, pub.subjects())
		}
	}
	if pub.has(provisioner.SubjectRollbackResult) {
		t.Fatal("no rollback should have been emitted")
	}
	if out.Result == nil || !out.Result.Verified {
		t.Fatalf("expected verified apply, got %+v", out.Result)
	}
}

func TestOnRecover_rejectedDoesNotApply(t *testing.T) {
	ap := &fakeApplier{}
	pub := &fakePublisher{}
	c := newConsumer(t, staleThenUp(), stubApprover{dec: ApprovalDec{Approved: false}}, stubOtp{code: "123456"}, ap, pub)

	out, err := c.OnRecover(context.Background(), "biz-1", "peer-1")
	if err != nil {
		t.Fatalf("OnRecover: %v", err)
	}
	if out.Applied || ap.applied != 0 {
		t.Fatalf("rejected plan must not apply: %+v", out)
	}
	if out.Approval.Reason != provisioner.ReasonRejected {
		t.Fatalf("reason = %q, want rejected", out.Approval.Reason)
	}
	if !pub.has(provisioner.SubjectActionRequested) || !pub.has(provisioner.SubjectApprovalResult) {
		t.Fatalf("expected request+result events, got %v", pub.subjects())
	}
	if pub.has(provisioner.SubjectApplyResult) {
		t.Fatal("rejected plan must not emit apply.result")
	}
}

func TestOnRecover_highRiskNeedsOTP(t *testing.T) {
	// Offline peer -> plan includes rotate_listen_port + escalate (high risk).
	offlineThenUp := &seqProber{obs: []provisioner.Observation{
		{State: provisioner.StateOffline, Keepalive: provisioner.ExpectedKeepalive, KeepaliveKnown: true},
		{State: provisioner.StateUp, Keepalive: provisioner.ExpectedKeepalive, KeepaliveKnown: true},
	}}

	// Approved but no OTP -> denied.
	ap := &fakeApplier{}
	c := newConsumer(t, offlineThenUp, stubApprover{dec: ApprovalDec{Approved: true}}, stubOtp{code: "424242"}, ap, &fakePublisher{})
	out, err := c.OnRecover(context.Background(), "biz-1", "peer-1")
	if err != nil {
		t.Fatalf("OnRecover: %v", err)
	}
	if out.Applied || ap.applied != 0 {
		t.Fatalf("high-risk plan without OTP must not apply: %+v", out)
	}
	if out.Approval.Reason != provisioner.ReasonOTPRequired {
		t.Fatalf("reason = %q, want otp_required", out.Approval.Reason)
	}

	// Approved with the correct OTP -> applies.
	ap2 := &fakeApplier{}
	prober2 := &seqProber{obs: []provisioner.Observation{
		{State: provisioner.StateOffline, Keepalive: provisioner.ExpectedKeepalive, KeepaliveKnown: true},
		{State: provisioner.StateUp, Keepalive: provisioner.ExpectedKeepalive, KeepaliveKnown: true},
	}}
	c2 := newConsumer(t, prober2, stubApprover{dec: ApprovalDec{Approved: true, OTP: "424242"}}, stubOtp{code: "424242"}, ap2, &fakePublisher{})
	out2, err := c2.OnRecover(context.Background(), "biz-1", "peer-1")
	if err != nil {
		t.Fatalf("OnRecover: %v", err)
	}
	if !out2.Applied || ap2.applied != 1 {
		t.Fatalf("high-risk plan with valid OTP should apply: %+v", out2)
	}
}

func TestOnRecover_verifyFailEmitsRollback(t *testing.T) {
	// Plan from stale, but the post-apply verification still sees offline.
	staleThenOffline := &seqProber{obs: []provisioner.Observation{
		{State: provisioner.StateStale, Keepalive: provisioner.ExpectedKeepalive, KeepaliveKnown: true},
		{State: provisioner.StateOffline, Keepalive: provisioner.ExpectedKeepalive, KeepaliveKnown: true},
	}}
	ap := &fakeApplier{}
	pub := &fakePublisher{}
	c := newConsumer(t, staleThenOffline, stubApprover{dec: ApprovalDec{Approved: true}}, stubOtp{code: "1"}, ap, pub)

	out, err := c.OnRecover(context.Background(), "biz-1", "peer-1")
	if err != nil {
		t.Fatalf("OnRecover: %v", err)
	}
	if out.Applied != true || out.Result == nil || !out.Result.RolledBack {
		t.Fatalf("expected applied-then-rolled-back, got %+v", out.Result)
	}
	if ap.restored != 1 {
		t.Fatalf("restored=%d, want 1", ap.restored)
	}
	if !pub.has(provisioner.SubjectRollbackResult) {
		t.Fatalf("expected rollback event, got %v", pub.subjects())
	}
	var payload map[string]any
	for _, e := range pub.events {
		if e.subject == provisioner.SubjectRollbackResult {
			_ = json.Unmarshal(e.data, &payload)
		}
	}
	if payload["peer_id"] != "peer-1" {
		t.Fatalf("rollback payload missing peer id: %v", payload)
	}
}

func TestOnRecover_healthyPeerNoChanges(t *testing.T) {
	up := &seqProber{obs: []provisioner.Observation{{State: provisioner.StateUp, Keepalive: provisioner.ExpectedKeepalive, KeepaliveKnown: true}}}
	ap := &fakeApplier{}
	pub := &fakePublisher{}
	c := newConsumer(t, up, stubApprover{dec: ApprovalDec{Approved: true}}, stubOtp{}, ap, pub)

	out, err := c.OnRecover(context.Background(), "biz-1", "peer-1")
	if err != nil {
		t.Fatalf("OnRecover: %v", err)
	}
	if out.Requested || out.Applied || len(out.Plan) != 0 {
		t.Fatalf("healthy peer should plan nothing and never request approval: %+v", out)
	}
	if pub.has(provisioner.SubjectActionRequested) {
		t.Fatal("healthy peer must not ask for approval")
	}
}

func TestOnRecover_approverErrorPropagates(t *testing.T) {
	c := newConsumer(t, staleThenUp(), stubApprover{err: errors.New("telegram down")}, stubOtp{}, &fakeApplier{}, &fakePublisher{})
	if _, err := c.OnRecover(context.Background(), "biz-1", "peer-1"); err == nil {
		t.Fatal("approver error must propagate")
	}
}

func TestParseRecover(t *testing.T) {
	cases := []struct {
		name     string
		subject  string
		data     string
		wantBiz  string
		wantPeer string
		wantOK   bool
	}{
		{"payload wins", "mm.biz-x.peer.recover.cmd", `{"business_id":"biz-1","peer_id":"p1","action":"reset_peer"}`, "biz-1", "p1", true},
		{"subject fallback", "mm.biz-2.peer.recover", `{"peer_id":"p2"}`, "biz-2", "p2", true},
		{"empty body", "mm.biz-3.peer.recover.cmd", "", "biz-3", "", false},
		{"no peer", "mm.biz-4.peer.recover", `{"business_id":"biz-4"}`, "biz-4", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			biz, peer, ok := parseRecover(tc.subject, []byte(tc.data))
			if biz != tc.wantBiz || peer != tc.wantPeer || ok != tc.wantOK {
				t.Fatalf("parseRecover = (%q,%q,%v), want (%q,%q,%v)", biz, peer, ok, tc.wantBiz, tc.wantPeer, tc.wantOK)
			}
		})
	}
}
