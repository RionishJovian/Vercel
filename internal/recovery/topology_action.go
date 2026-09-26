package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"mega-magic/internal/messaging"
	"mega-magic/internal/provisioner"
)

// TopologyActionConsumer is the explicit-action path of the provisioning plane:
// an OWNER clicks "reboot" (or a future device action) on the topology map, the
// handler publishes a topology.action intent, and this consumer drives it.
//
// It deliberately reuses the recovery approve-gate (action_requested -> await
// approval -> EvaluateApproval with OTP) and the shared Prober/Applier, but NOT
// ApplyFixes' config snapshot/rollback: a reboot is fire-and-validate — you
// cannot "un-reboot". ApplyService.ApplyAction issues the command and verifies
// the device returns; approval is high-risk (OTP) by default for reboot.
//
// It ships no router transport: the Applier and Prober are injected (the real
// SSH/RouterOS transport is deferred like the rest of the applier slice).
type TopologyActionConsumer struct {
	svc      *provisioner.ApplyService
	probe    provisioner.Prober
	approver provisioner.Approver
	otp      provisioner.OtpVerifier
	events   provisioner.Publisher
	logger   *zap.SugaredLogger
}

// NewTopologyActionConsumer builds the consumer. approver/otp/events may be nil
// in tests; a nil events sink means events are silently dropped.
func NewTopologyActionConsumer(
	svc *provisioner.ApplyService,
	probe provisioner.Prober,
	approver provisioner.Approver,
	otp provisioner.OtpVerifier,
	events provisioner.Publisher,
	logger *zap.SugaredLogger,
) *TopologyActionConsumer {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
	return &TopologyActionConsumer{svc: svc, probe: probe, approver: approver, otp: otp, events: events, logger: logger}
}

// Start subscribes to the topology action intent on every tenant.
func (c *TopologyActionConsumer) Start(client *messaging.Client) error {
	if client == nil {
		return fmt.Errorf("provisioner: nil messaging client")
	}
	if c.events == nil {
		c.events = client
	}
	pattern := fmt.Sprintf("mm.*.%s", provisioner.SubjectTopologyAction)
	if err := client.SubscribePattern(pattern, c.handle); err != nil {
		return err
	}
	c.logger.Infof("provisioner: topology action consumer listening on %s", pattern)
	return nil
}

// ActionIntent is what the topology handler publishes on topology.action.
type ActionIntent struct {
	BusinessID string `json:"business_id"`
	// DeviceID doubles as the peer the provisioner applies to (peer == device
	// in this system: the state-sweep iterates PeerKey{BusinessID, DeviceID}).
	DeviceID string `json:"device_id"`
	Action   string `json:"action"`
}

// handle decodes one topology action intent and drives it through the gate.
func (c *TopologyActionConsumer) handle(m *nats.Msg) {
	intent, ok := parseActionIntent(m.Subject, m.Data)
	if !ok {
		c.logger.Warnf("provisioner: ignoring malformed topology action on %s", m.Subject)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	out, err := c.OnAction(ctx, intent)
	if err != nil {
		c.logger.Warnf("provisioner: topology action %s for device %s failed: %v",
			intent.Action, intent.DeviceID, err)
		return
	}
	c.logger.Infof("provisioner: device %s action=%s approved=%v applied=%v verified=%v",
		intent.DeviceID, intent.Action, out.Approval.Allowed, out.Applied, out.Result != nil && out.Result.Verified)
}

// ActionOutcome is the result of one topology action through the gate.
type ActionOutcome struct {
	DeviceID string
	Action   string
	Changes  []provisioner.Change
	Requested bool
	Approval  provisioner.ApprovalOutcome
	Applied   bool
	Result    *provisioner.ApplyResult
}

// parseActionIntent decodes the payload; the tenant falls back to the subject
// (`mm.<business>.topology.action`). Device id + action are required.
func parseActionIntent(subject string, data []byte) (ActionIntent, bool) {
	var intent ActionIntent
	if len(data) > 0 {
		_ = json.Unmarshal(data, &intent)
	}
	if intent.BusinessID == "" {
		parts := strings.Split(subject, ".")
		if len(parts) >= 2 {
			intent.BusinessID = parts[1]
		}
	}
	ok := intent.BusinessID != "" && intent.DeviceID != "" && intent.Action != ""
	return intent, ok
}

// OnAction runs one explicit topology action: build the change -> approve-gate
// (action_requested -> await -> EvaluateApproval; high-risk => OTP) -> apply
// (fire-and-validate) -> verify -> publish results. Never applies without
// explicit approval.
func (c *TopologyActionConsumer) OnAction(ctx context.Context, intent ActionIntent) (*ActionOutcome, error) {
	if intent.DeviceID == "" {
		return nil, fmt.Errorf("provisioner: topology action without device id")
	}
	if c.svc == nil {
		return nil, fmt.Errorf("provisioner: no apply service configured")
	}

	out := &ActionOutcome{DeviceID: intent.DeviceID, Action: intent.Action}

	// 1) Map the explicit action to a change. Reboot is the only supported
	//    action today; unknown actions fail closed.
	var change provisioner.Change
	if intent.Action == provisioner.ActionReboot {
		change = provisioner.Change{
			Action:   provisioner.ActionReboot,
			Target:   intent.DeviceID,
			Proposed: "system reboot",
			Reason:   "explicit owner reboot from topology map",
			Risk:     "high",
		}
	} else {
		_ = c.publishJSON(intent.BusinessID, provisioner.SubjectApplyResult, map[string]any{
			"peer_id": intent.DeviceID, "applied": false, "detail": "unsupported topology action: " + intent.Action,
		})
		return out, nil
	}
	out.Changes = []provisioner.Change{change}

	// 2) Manual approve-gate: high-risk => OTP on top of the owner's approval.
	req := provisioner.ApprovalRequest{
		BusinessID:  intent.BusinessID,
		PeerID:      intent.DeviceID,
		Changes:     out.Changes,
		RequiresOTP: provisioner.RequiresOTP(out.Changes),
		RequestedAt: time.Now().UTC(),
	}
	out.Requested = true
	if err := c.publishJSON(intent.BusinessID, provisioner.SubjectActionRequested, req); err != nil {
		return nil, fmt.Errorf("provisioner: emit action_requested: %w", err)
	}

	if c.approver == nil {
		return nil, fmt.Errorf("provisioner: no approver configured")
	}
	dec, err := c.approver.Await(ctx, intent.BusinessID, req)
	if err != nil {
		return nil, fmt.Errorf("provisioner: await approval for device %s: %w", intent.DeviceID, err)
	}

	outcome, err := provisioner.EvaluateApproval(ctx, intent.DeviceID, out.Changes, dec, c.otp)
	if err != nil {
		return nil, err
	}
	out.Approval = outcome
	_ = c.publishJSON(intent.BusinessID, provisioner.SubjectApprovalResult, map[string]any{
		"peer_id":      intent.DeviceID,
		"allowed":      outcome.Allowed,
		"reason":       outcome.Reason,
		"otp_required": outcome.OTPRequired,
	})
	if !outcome.Allowed {
		return out, nil
	}

	// 3) Fire-and-validate: apply the command, then verify the device returns.
	res, err := c.svc.ApplyAction(ctx, provisioner.ApplyRequest{
		BusinessID: intent.BusinessID,
		PeerID:     intent.DeviceID,
		Changes:    out.Changes,
	})
	if err != nil {
		return nil, err
	}
	out.Applied = true
	out.Result = res

	_ = c.publishJSON(intent.BusinessID, provisioner.SubjectApplyResult, res)
	return out, nil
}

func (c *TopologyActionConsumer) publishJSON(businessID, subject string, payload any) error {
	if c.events == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", subject, err)
	}
	return c.events.Publish(businessID, subject, data)
}