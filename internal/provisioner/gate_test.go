package provisioner

import (
	"context"
	"errors"
	"testing"
)

// lowMedPlan is a plan with only low/medium actions (needs no OTP).
func lowMedPlan() []Change {
	return PlanRecovery(Observation{PeerID: "p", State: StateStale, Keepalive: ExpectedKeepalive, KeepaliveKnown: true})
}

// highRiskPlan contains rotate_listen_port/escalate (needs OTP).
func highRiskPlan() []Change {
	return PlanRecovery(Observation{PeerID: "p", State: StateOffline, Keepalive: ExpectedKeepalive, KeepaliveKnown: true})
}

func TestIsHighRisk_rule(t *testing.T) {
	high := []string{ActionRotateListenPort, ActionEscalate, ActionReboot}
	low := []string{ActionReprobe, ActionResetPeer, ActionEnsureKeepalive}
	for _, a := range high {
		if !IsHighRisk(a) {
			t.Fatalf("action %q should be high-risk", a)
		}
	}
	for _, a := range low {
		if IsHighRisk(a) {
			t.Fatalf("action %q should NOT be high-risk", a)
		}
	}
}

func TestRequiresOTP(t *testing.T) {
	if RequiresOTP(lowMedPlan()) {
		t.Fatal("low/medium plan must not require OTP")
	}
	if !RequiresOTP(highRiskPlan()) {
		t.Fatal("plan with rotate_listen_port/escalate must require OTP")
	}
	// Mixed: one high-risk action is enough.
	mixed := []Change{{Action: ActionReprobe, Risk: "low"}, {Action: ActionEscalate, Risk: "high"}}
	if !RequiresOTP(mixed) {
		t.Fatal("any high-risk action must trigger the OTP requirement")
	}
	if RequiresOTP(nil) {
		t.Fatal("empty plan must not require OTP")
	}
}

func TestEvaluateApproval(t *testing.T) {
	ctx := context.Background()
	otp := stubOtp{code: "123456"}

	cases := []struct {
		name        string
		changes     []Change
		dec         ApprovalDecision
		otp         OtpVerifier
		wantAllowed bool
		wantReason  string
		wantOTPReq  bool
	}{
		{
			name: "low/med approved no otp", changes: lowMedPlan(),
			dec: ApprovalDecision{Approved: true}, otp: otp,
			wantAllowed: true, wantReason: ReasonApproved, wantOTPReq: false,
		},
		{
			name: "low/med rejected", changes: lowMedPlan(),
			dec: ApprovalDecision{Approved: false}, otp: otp,
			wantAllowed: false, wantReason: ReasonRejected, wantOTPReq: false,
		},
		{
			name: "high-risk approved without otp is denied", changes: highRiskPlan(),
			dec: ApprovalDecision{Approved: true}, otp: otp,
			wantAllowed: false, wantReason: ReasonOTPRequired, wantOTPReq: true,
		},
		{
			name: "high-risk approved with wrong otp", changes: highRiskPlan(),
			dec: ApprovalDecision{Approved: true, OTP: "000000"}, otp: otp,
			wantAllowed: false, wantReason: ReasonOTPInvalid, wantOTPReq: true,
		},
		{
			name: "high-risk approved with correct otp", changes: highRiskPlan(),
			dec: ApprovalDecision{Approved: true, OTP: "123456"}, otp: otp,
			wantAllowed: true, wantReason: ReasonApproved, wantOTPReq: true,
		},
		{
			name: "rejection wins even with valid otp", changes: highRiskPlan(),
			dec: ApprovalDecision{Approved: false, OTP: "123456"}, otp: otp,
			wantAllowed: false, wantReason: ReasonRejected, wantOTPReq: true,
		},
		{
			name: "empty plan", changes: nil,
			dec: ApprovalDecision{Approved: true}, otp: otp,
			wantAllowed: false, wantReason: ReasonNoChanges, wantOTPReq: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EvaluateApproval(ctx, "peer-1", tc.changes, tc.dec, tc.otp)
			if err != nil {
				t.Fatalf("EvaluateApproval: %v", err)
			}
			if got.Allowed != tc.wantAllowed || got.Reason != tc.wantReason || got.OTPRequired != tc.wantOTPReq {
				t.Fatalf("outcome = %+v, want allowed=%v reason=%q otpReq=%v", got, tc.wantAllowed, tc.wantReason, tc.wantOTPReq)
			}
		})
	}
}

func TestEvaluateApproval_failsClosed(t *testing.T) {
	ctx := context.Background()
	// Approved high-risk plan but no verifier configured: must error, never allow.
	if _, err := EvaluateApproval(ctx, "peer-1", highRiskPlan(), ApprovalDecision{Approved: true, OTP: "x"}, nil); err == nil {
		t.Fatal("missing OTP verifier on a high-risk plan must fail closed")
	}
	// Verifier error propagates.
	if _, err := EvaluateApproval(ctx, "peer-1", highRiskPlan(), ApprovalDecision{Approved: true, OTP: "x"}, stubOtp{err: errors.New("boom")}); err == nil {
		t.Fatal("OTP verifier error must propagate")
	}
}

type stubOtp struct {
	code string
	err  error
}

func (s stubOtp) Verify(_ context.Context, _, code string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return code == s.code, nil
}
