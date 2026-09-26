package provisioner

import (
	"context"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Manual approve-gate (DESIGN-provisioner.md section 2) + OTP gate
// ---------------------------------------------------------------------------

// highRiskActions require a one-time OTP on top of the manual approval. LOW and
// MEDIUM actions approve on the manual gate alone. This rule is REAL and
// unit-tested; only the OTP *provider* (real TOTP) is a later slice.
var highRiskActions = map[string]bool{
	ActionRotateListenPort: true,
	ActionEscalate:         true,
	ActionReboot:           true,
}

// IsHighRisk reports whether a single recovery action is high-risk.
func IsHighRisk(action string) bool { return highRiskActions[action] }

// RequiresOTP reports whether a plan contains any high-risk action, and so
// needs an OTP in addition to the Lead's manual approval.
func RequiresOTP(changes []Change) bool {
	for _, c := range changes {
		if IsHighRisk(c.Action) {
			return true
		}
	}
	return false
}

// Gate reasons, used in ApprovalOutcome.Reason and on the wire.
const (
	ReasonApproved    = "approved"
	ReasonRejected    = "rejected"
	ReasonOTPRequired = "otp_required"
	ReasonOTPInvalid  = "otp_invalid"
	ReasonNoChanges   = "no_changes"
)

// ApprovalRequest is the plan submitted to the Lead for a manual decision.
type ApprovalRequest struct {
	BusinessID  string    `json:"business_id"`
	PeerID      string    `json:"peer_id"`
	Changes     []Change  `json:"changes"`
	RequiresOTP bool      `json:"requires_otp"`
	RequestedAt time.Time `json:"requested_at"`
}

// ApprovalDecision is the Lead's verdict, optionally carrying an OTP.
type ApprovalDecision struct {
	Approved bool      `json:"approved"`
	OTP      string    `json:"otp,omitempty"`
	Approver string    `json:"approver,omitempty"`
	At       time.Time `json:"at,omitempty"`
}

// ApprovalOutcome is the gate's final verdict once the OTP rule is applied.
type ApprovalOutcome struct {
	Allowed     bool   `json:"allowed"`
	Reason      string `json:"reason"`
	OTPRequired bool   `json:"otp_required"`
}

// Approver obtains the Lead's manual approve/reject decision for a plan. The
// real implementation bridges the NATS approval subject to Telegram; tests
// inject a stub.
type Approver interface {
	Await(ctx context.Context, businessID string, req ApprovalRequest) (ApprovalDecision, error)
}

// OtpVerifier verifies a one-time code for a peer. The real TOTP provider is a
// later slice; the decision rule below is real and unit-tested today.
type OtpVerifier interface {
	Verify(ctx context.Context, peerID, code string) (bool, error)
}

// EvaluateApproval applies the approved design's decision rule:
//   - a rejection never applies;
//   - a low/medium plan approves on the manual gate alone;
//   - a high-risk plan additionally requires a valid OTP, else it is denied.
func EvaluateApproval(ctx context.Context, peerID string, changes []Change, dec ApprovalDecision, otp OtpVerifier) (ApprovalOutcome, error) {
	if len(changes) == 0 {
		return ApprovalOutcome{Allowed: false, Reason: ReasonNoChanges}, nil
	}

	required := RequiresOTP(changes)

	if !dec.Approved {
		return ApprovalOutcome{Allowed: false, Reason: ReasonRejected, OTPRequired: required}, nil
	}
	if !required {
		return ApprovalOutcome{Allowed: true, Reason: ReasonApproved}, nil
	}
	if dec.OTP == "" {
		return ApprovalOutcome{Allowed: false, Reason: ReasonOTPRequired, OTPRequired: true}, nil
	}
	if otp == nil {
		// Fail closed: an approved high-risk plan we cannot OTP-verify must not apply.
		return ApprovalOutcome{}, fmt.Errorf("provisioner: high-risk plan approved for peer %s but no OTP verifier is configured", peerID)
	}
	ok, err := otp.Verify(ctx, peerID, dec.OTP)
	if err != nil {
		return ApprovalOutcome{}, fmt.Errorf("provisioner: verify OTP for peer %s: %w", peerID, err)
	}
	if !ok {
		return ApprovalOutcome{Allowed: false, Reason: ReasonOTPInvalid, OTPRequired: true}, nil
	}
	return ApprovalOutcome{Allowed: true, Reason: ReasonApproved, OTPRequired: true}, nil
}
