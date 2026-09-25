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

// ApplyConsumer wires the provisioner apply path (DESIGN-provisioner.md
// sections 2-4) to the existing recovery intent. It consumes `peer.recover.cmd`
// (the hub action intent emitted by RecoveryService.emitHubAction) and
// `peer.recover` (the raw trigger), builds a plan with the committed planner,
// takes it through the manual approve-gate, and only then applies it.
//
// It uses the messaging client's existing plumbing (SubscribePattern for the
// intent, Request for the Lead's approval round-trip) and emits the four
// provisioner events. It ships no router transport: the Applier is injected.
type ApplyConsumer struct {
	svc      *provisioner.ApplyService
	probe    provisioner.Prober
	approver provisioner.Approver
	otp      provisioner.OtpVerifier
	events   provisioner.Publisher
	logger   *zap.SugaredLogger
}

// NewApplyConsumer builds the consumer. Any of approver/otp/events may be nil in
// tests; a nil events sink means events are silently dropped.
func NewApplyConsumer(
	svc *provisioner.ApplyService,
	probe provisioner.Prober,
	approver provisioner.Approver,
	otp provisioner.OtpVerifier,
	events provisioner.Publisher,
	logger *zap.SugaredLogger,
) *ApplyConsumer {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
	return &ApplyConsumer{svc: svc, probe: probe, approver: approver, otp: otp, events: events, logger: logger}
}

// Start subscribes to the recovery intent on every tenant. It is the only place
// the consumer touches the message bus for subscription; the bus also becomes
// the default event sink when none was injected.
func (c *ApplyConsumer) Start(client *messaging.Client) error {
	if client == nil {
		return fmt.Errorf("provisioner: nil messaging client")
	}
	if c.events == nil {
		c.events = client
	}
	patterns := []string{
		fmt.Sprintf("mm.*.%s", recoverSubject+".cmd"),
		fmt.Sprintf("mm.*.%s", recoverSubject),
	}
	for _, pattern := range patterns {
		if err := client.SubscribePattern(pattern, c.handle); err != nil {
			return err
		}
		c.logger.Infof("provisioner: apply consumer listening on %s", pattern)
	}
	return nil
}

// handle decodes a recovery intent and drives one recovery pass.
func (c *ApplyConsumer) handle(m *nats.Msg) {
	businessID, peerID, ok := parseRecover(m.Subject, m.Data)
	if !ok {
		c.logger.Warnf("provisioner: ignoring malformed recover intent on %s", m.Subject)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	out, err := c.OnRecover(ctx, businessID, peerID)
	if err != nil {
		c.logger.Warnf("provisioner: recover pass for peer %s failed: %v", peerID, err)
		return
	}
	c.logger.Infof("provisioner: peer %s plan=%d approved=%v applied=%v rolled_back=%v",
		peerID, len(out.Plan), out.Approval.Allowed, out.Applied, out.Result != nil && out.Result.RolledBack)
}

// Outcome is the result of one recovery pass through the approve/apply path.
type Outcome struct {
	PeerID    string
	Plan      []provisioner.Change
	Requested bool
	Approval  provisioner.ApprovalOutcome
	Applied   bool
	Result    *provisioner.ApplyResult
}

// recoverIntent is the payload RecoveryService emits on peer.recover.cmd
// (plus the optional state carried by the peer.recover trigger).
type recoverIntent struct {
	BusinessID string `json:"business_id"`
	PeerID     string `json:"peer_id"`
	Action     string `json:"action,omitempty"`
	State      string `json:"state,omitempty"`
}

// parseRecover extracts the tenant + peer from a recover payload/subject. The
// payload wins; the subject (`mm.<business>.peer.recover[.cmd]`) is the
// fallback, so a bare trigger with no body still resolves.
func parseRecover(subject string, data []byte) (businessID, peerID string, ok bool) {
	var intent recoverIntent
	if len(data) > 0 {
		_ = json.Unmarshal(data, &intent)
	}
	businessID = intent.BusinessID
	if businessID == "" {
		parts := strings.Split(subject, ".")
		if len(parts) >= 2 {
			businessID = parts[1]
		}
	}
	peerID = intent.PeerID
	return businessID, peerID, peerID != "" && businessID != ""
}

// OnRecover runs one recovery pass: observe -> plan -> action_requested ->
// await approval (+ OTP for high-risk) -> apply -> verify -> emit results. It
// never applies without an explicit approval.
func (c *ApplyConsumer) OnRecover(ctx context.Context, businessID, peerID string) (*Outcome, error) {
	if peerID == "" {
		return nil, fmt.Errorf("provisioner: recover intent without peer id")
	}
	if c.svc == nil {
		return nil, fmt.Errorf("provisioner: no apply service configured")
	}

	out := &Outcome{PeerID: peerID}

	// 1) Build the plan from current evidence (reuses the committed planner).
	if c.probe == nil {
		return nil, fmt.Errorf("provisioner: no prober configured")
	}
	obs, _, err := c.probe.Observe(ctx, peerID)
	if err != nil {
		return nil, fmt.Errorf("provisioner: observe peer %s: %w", peerID, err)
	}
	obs.PeerID = peerID
	out.Plan = provisioner.PlanRecovery(obs)

	if len(out.Plan) == 0 {
		_ = c.publishJSON(businessID, provisioner.SubjectApplyResult, map[string]any{
			"peer_id": peerID, "applied": false, "detail": "no recovery changes proposed",
		})
		return out, nil
	}

	// 2) Manual approve-gate: ask before touching the router.
	req := provisioner.ApprovalRequest{
		BusinessID:  businessID,
		PeerID:      peerID,
		Changes:     out.Plan,
		RequiresOTP: provisioner.RequiresOTP(out.Plan),
		RequestedAt: time.Now().UTC(),
	}
	out.Requested = true
	if err := c.publishJSON(businessID, provisioner.SubjectActionRequested, req); err != nil {
		return nil, fmt.Errorf("provisioner: emit action_requested: %w", err)
	}

	if c.approver == nil {
		return nil, fmt.Errorf("provisioner: no approver configured")
	}
	dec, err := c.approver.Await(ctx, businessID, req)
	if err != nil {
		return nil, fmt.Errorf("provisioner: await approval for peer %s: %w", peerID, err)
	}

	outcome, err := provisioner.EvaluateApproval(ctx, peerID, out.Plan, dec, c.otp)
	if err != nil {
		return nil, err
	}
	out.Approval = outcome
	_ = c.publishJSON(businessID, provisioner.SubjectApprovalResult, map[string]any{
		"peer_id":      peerID,
		"allowed":      outcome.Allowed,
		"reason":       outcome.Reason,
		"otp_required": outcome.OTPRequired,
	})
	if !outcome.Allowed {
		return out, nil
	}

	// 3) Apply only on approval; verify-before-apply + rollback live in ApplyFixes.
	res, err := c.svc.ApplyFixes(ctx, provisioner.ApplyRequest{
		BusinessID: businessID,
		PeerID:     peerID,
		Changes:    out.Plan,
	})
	if err != nil {
		return nil, err
	}
	out.Applied = true
	out.Result = res

	_ = c.publishJSON(businessID, provisioner.SubjectApplyResult, res)
	if res.RolledBack {
		_ = c.publishJSON(businessID, provisioner.SubjectRollbackResult, map[string]any{
			"peer_id":           peerID,
			"detail":            res.RollbackDetail,
			"deadline_exceeded": res.DeadlineExceeded,
			"rollback_ms":       res.RollbackMs,
		})
	}
	return out, nil
}

func (c *ApplyConsumer) publishJSON(businessID, subject string, payload any) error {
	if c.events == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", subject, err)
	}
	return c.events.Publish(businessID, subject, data)
}

// ---------------------------------------------------------------------------
// Real approve-gate transport
// ---------------------------------------------------------------------------

// NatsApprover obtains the Lead's manual decision by issuing a request on the
// tenant's approval subject and waiting for the Telegram bridge to reply with an
// ApprovalDecision. It uses the existing messaging client Request plumbing.
type NatsApprover struct {
	client  *messaging.Client
	timeout time.Duration
}

func NewNatsApprover(client *messaging.Client, timeout time.Duration) *NatsApprover {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &NatsApprover{client: client, timeout: timeout}
}

// Await implements provisioner.Approver.
func (a *NatsApprover) Await(ctx context.Context, businessID string, req provisioner.ApprovalRequest) (provisioner.ApprovalDecision, error) {
	if a.client == nil {
		return provisioner.ApprovalDecision{}, fmt.Errorf("provisioner: nil messaging client for approval")
	}
	data, err := json.Marshal(req)
	if err != nil {
		return provisioner.ApprovalDecision{}, fmt.Errorf("marshal approval request: %w", err)
	}
	msg, err := a.client.Request(ctx, businessID, provisioner.SubjectApprovalRequest, data, a.timeout)
	if err != nil {
		return provisioner.ApprovalDecision{}, err
	}
	var dec provisioner.ApprovalDecision
	if err := json.Unmarshal(msg.Data, &dec); err != nil {
		return provisioner.ApprovalDecision{}, fmt.Errorf("decode approval decision: %w", err)
	}
	return dec, nil
}
