package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"mega-magic/internal/provisioner"
)

// TopologyActionConsumer tests. Reuses the fakes from consumer_test.go (same
// package): fakePublisher, stubApprover, stubOtp, seqProber, fakeApplier.
//
// Reboot is HIGH-RISK (gate highRiskActions), so — per the real gate rule —
// an approved reboot needs a valid OTP before it is ever applied.

func newTopoConsumer(t *testing.T, prober provisioner.Prober, approver provisioner.Approver, otp provisioner.OtpVerifier, ap *fakeApplier, pub *fakePublisher) *TopologyActionConsumer {
	t.Helper()
	svc := provisioner.NewApplyService(ap, prober).WithRollbackWindow(time.Second)
	return NewTopologyActionConsumer(svc, prober, approver, otp, pub, nil)
}

// upProber returns a single UP observation — enough for reboot, which issues
// the command then verifies the device returns (no plan-time observe step).
func upProber() provisioner.Prober {
	return &seqProber{obs: []provisioner.Observation{
		{State: provisioner.StateUp, Keepalive: provisioner.ExpectedKeepalive, KeepaliveKnown: true},
	}}
}

func rebootIntent(deviceID string) ActionIntent {
	return ActionIntent{BusinessID: "biz-1", DeviceID: deviceID, Action: provisioner.ActionReboot}
}

func TestOnAction_rebootApprovedWithOTPApplies(t *testing.T) {
	ap := &fakeApplier{}
	pub := &fakePublisher{}
	c := newTopoConsumer(t, upProber(), stubApprover{dec: ApprovalDec{Approved: true, OTP: "123456"}}, stubOtp{code: "123456"}, ap, pub)

	out, err := c.OnAction(context.Background(), rebootIntent("device-1"))
	if err != nil {
		t.Fatalf("OnAction: %v", err)
	}
	if !out.Requested || !out.Approval.Allowed || !out.Applied {
		t.Fatalf("outcome = %+v, want requested+allowed+applied", out)
	}
	if ap.applied != 1 || ap.restored != 0 {
		t.Fatalf("applied=%d restored=%d, want 1/0 (fire-and-validate, no rollback)", ap.applied, ap.restored)
	}
	// Reboot must NEVER snapshot or restore — there is no config to revert.
	if ap.snapped != 0 {
		t.Fatalf("reboot must not snapshot, snapped=%d", ap.snapped)
	}
	for _, s := range []string{provisioner.SubjectActionRequested, provisioner.SubjectApprovalResult, provisioner.SubjectApplyResult} {
		if !pub.has(s) {
			t.Fatalf("missing event %s; got %v", s, pub.subjects())
		}
	}
	if out.Result == nil || !out.Result.Verified {
		t.Fatalf("expected verified apply, got %+v", out.Result)
	}
	if out.Result.RolledBack {
		t.Fatal("reboot result must not be rolled back")
	}
}

func TestOnAction_rebootApprovedNoOTPDenied(t *testing.T) {
	ap := &fakeApplier{}
	pub := &fakePublisher{}
	c := newTopoConsumer(t, upProber(), stubApprover{dec: ApprovalDec{Approved: true}}, stubOtp{code: "999999"}, ap, pub)

	out, err := c.OnAction(context.Background(), rebootIntent("device-1"))
	if err != nil {
		t.Fatalf("OnAction: %v", err)
	}
	if out.Applied || ap.applied != 0 {
		t.Fatalf("high-risk reboot without OTP must not apply: %+v", out)
	}
	if out.Approval.Reason != provisioner.ReasonOTPRequired {
		t.Fatalf("reason = %q, want otp_required", out.Approval.Reason)
	}
	if !out.Approval.OTPRequired {
		t.Fatalf("reboot must be flagged OTP-required, got %+v", out.Approval)
	}
	if pub.has(provisioner.SubjectApplyResult) {
		t.Fatalf("denied reboot must not emit apply.result, got %v", pub.subjects())
	}
}

func TestOnAction_rebootWrongOTPDenied(t *testing.T) {
	ap := &fakeApplier{}
	pub := &fakePublisher{}
	c := newTopoConsumer(t, upProber(), stubApprover{dec: ApprovalDec{Approved: true, OTP: "wrong"}}, stubOtp{code: "123456"}, ap, pub)

	out, err := c.OnAction(context.Background(), rebootIntent("device-1"))
	if err != nil {
		t.Fatalf("OnAction: %v", err)
	}
	if out.Applied || ap.applied != 0 {
		t.Fatalf("wrong OTP must not apply: %+v", out)
	}
	if out.Approval.Reason != provisioner.ReasonOTPInvalid {
		t.Fatalf("reason = %q, want otp_invalid", out.Approval.Reason)
	}
}

func TestOnAction_rebootRejectedDoesNotApply(t *testing.T) {
	ap := &fakeApplier{}
	pub := &fakePublisher{}
	c := newTopoConsumer(t, upProber(), stubApprover{dec: ApprovalDec{Approved: false}}, stubOtp{code: "123456"}, ap, pub)

	out, err := c.OnAction(context.Background(), rebootIntent("device-1"))
	if err != nil {
		t.Fatalf("OnAction: %v", err)
	}
	if out.Applied || ap.applied != 0 {
		t.Fatalf("rejected reboot must not apply: %+v", out)
	}
	if out.Approval.Reason != provisioner.ReasonRejected {
		t.Fatalf("reason = %q, want rejected", out.Approval.Reason)
	}
	if pub.has(provisioner.SubjectApplyResult) {
		t.Fatalf("rejected reboot must not emit apply.result")
	}
}

func TestOnAction_unknownActionFailsClosed(t *testing.T) {
	ap := &fakeApplier{}
	pub := &fakePublisher{}
	c := newTopoConsumer(t, upProber(), stubApprover{dec: ApprovalDec{Approved: true, OTP: "123456"}}, stubOtp{code: "123456"}, ap, pub)

	out, err := c.OnAction(context.Background(), ActionIntent{BusinessID: "biz-1", DeviceID: "device-1", Action: "format_disk"})
	if err != nil {
		t.Fatalf("OnAction: %v", err)
	}
	if out.Requested || out.Applied || ap.applied != 0 {
		t.Fatalf("unknown action must fail closed (no request, no apply): %+v", out)
	}
	if !pub.has(provisioner.SubjectApplyResult) {
		t.Fatalf("unknown action should report it did not apply, got %v", pub.subjects())
	}
}

func TestOnAction_approverErrorPropagates(t *testing.T) {
	c := newTopoConsumer(t, upProber(), stubApprover{err: errors.New("telegram down")}, stubOtp{}, &fakeApplier{}, &fakePublisher{})
	if _, err := c.OnAction(context.Background(), rebootIntent("device-1")); err == nil {
		t.Fatal("approver error must propagate")
	}
}

func TestParseActionIntent(t *testing.T) {
	cases := []struct {
		name    string
		subject string
		data    string
		wantBiz string
		wantDev string
		wantOK  bool
	}{
		{"payload wins", "mm.biz-x.topology.action", `{"business_id":"biz-1","device_id":"dev-1","action":"reboot"}`, "biz-1", "dev-1", true},
		{"subject fallback", "mm.biz-2.topology.action", `{"device_id":"dev-2","action":"reboot"}`, "biz-2", "dev-2", true},
		{"empty body", "mm.biz-3.topology.action", "", "biz-3", "", false},
		{"no device", "mm.biz-4.topology.action", `{"business_id":"biz-4","action":"reboot"}`, "biz-4", "", false},
		{"no action", "mm.biz-5.topology.action", `{"business_id":"biz-5","device_id":"dev-5"}`, "biz-5", "dev-5", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			intent, ok := parseActionIntent(tc.subject, []byte(tc.data))
			if intent.BusinessID != tc.wantBiz || intent.DeviceID != tc.wantDev || ok != tc.wantOK {
				t.Fatalf("parseActionIntent = (%q,%q,%v), want (%q,%q,%v)", intent.BusinessID, intent.DeviceID, ok, tc.wantBiz, tc.wantDev, tc.wantOK)
			}
		})
	}
}

func TestOnAction_requiresOTPFieldOnRebootRequest(t *testing.T) {
	// The ApprovalRequest the consumer emits for a reboot must carry
	// RequiresOTP=true — the binding contract for the approval bridge.
	pub := &fakePublisher{}
	c := newTopoConsumer(t, upProber(), stubApprover{dec: ApprovalDec{Approved: false}}, stubOtp{}, &fakeApplier{}, pub)

	_, _ = c.OnAction(context.Background(), rebootIntent("device-1"))

	var req provisioner.ApprovalRequest
	found := false
	for _, e := range pub.events {
		if e.subject == provisioner.SubjectActionRequested {
			_ = json.Unmarshal(e.data, &req)
			found = true
		}
	}
	if !found {
		t.Fatal("no action_requested event emitted")
	}
	if !req.RequiresOTP {
		t.Fatalf("reboot approval request must require OTP, got %+v", req)
	}
	if len(req.Changes) != 1 || req.Changes[0].Action != provisioner.ActionReboot || req.Changes[0].Risk != "high" {
		t.Fatalf("reboot request changes wrong: %+v", req.Changes)
	}
}