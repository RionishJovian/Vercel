package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"mega-magic/internal/provisioner"
)

// BridgeBus is the small messaging surface the approval bridge needs: subscribe
// to the approval subject and reply to a request's inbox. *messaging.Client
// satisfies it; tests inject a fake.
type BridgeBus interface {
	SubscribePattern(pattern string, handler func(msg *nats.Msg)) error
	Reply(msg *nats.Msg, data []byte) error
}

// ApprovalBridge answers the NATS `provision.approval` request on behalf of the
// human Lead: it hands the request to a provisioner.Approver (the Telegram
// bridge) and replies with the resulting ApprovalDecision. It never fabricates
// an approval — if the approver fails or times out, the reply is a DENY.
type ApprovalBridge struct {
	approver provisioner.Approver
	bus      BridgeBus
	logger   *zap.SugaredLogger
	timeout  time.Duration
}

// NewApprovalBridge builds the bridge over an approver + bus.
func NewApprovalBridge(approver provisioner.Approver, bus BridgeBus, logger *zap.SugaredLogger) *ApprovalBridge {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
	return &ApprovalBridge{approver: approver, bus: bus, logger: logger, timeout: 10 * time.Minute}
}

// Start subscribes to the approval request on every tenant.
func (b *ApprovalBridge) Start() error {
	if b.bus == nil {
		return fmt.Errorf("provisioner: approval bridge has no bus")
	}
	pattern := fmt.Sprintf("mm.*.%s", provisioner.SubjectApprovalRequest)
	if err := b.bus.SubscribePattern(pattern, b.handle); err != nil {
		return fmt.Errorf("provisioner: approval bridge subscribe %s: %w", pattern, err)
	}
	b.logger.Infof("provisioner: approval bridge listening on %s", pattern)
	return nil
}

// handle answers one approval request. Any error from the approver becomes a
// DENY reply so the consumer always sees a decision (never a silent hang, never
// an accidental approve).
func (b *ApprovalBridge) handle(m *nats.Msg) {
	var req provisioner.ApprovalRequest
	if err := json.Unmarshal(m.Data, &req); err != nil {
		b.logger.Warnf("provisioner: bad approval request on %s: %v", m.Subject, err)
		b.reply(m, provisioner.ApprovalDecision{Approved: false, Approver: "bridge"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	dec, err := b.approver.Await(ctx, req.BusinessID, req)
	if err != nil {
		b.logger.Warnf("provisioner: approver failed for peer %s: %v (replying DENY)", req.PeerID, err)
		dec = provisioner.ApprovalDecision{Approved: false, Approver: "bridge"}
	}
	if !dec.Approved {
		b.logger.Infof("provisioner: approval DENIED for peer %s (%s)", req.PeerID, dec.Approver)
	} else {
		b.logger.Infof("provisioner: approval GRANTED for peer %s", req.PeerID)
	}
	b.reply(m, dec)
}

func (b *ApprovalBridge) reply(m *nats.Msg, dec provisioner.ApprovalDecision) {
	data, err := json.Marshal(dec)
	if err != nil {
		b.logger.Warnf("provisioner: marshal approval decision: %v", err)
		return
	}
	if err := b.bus.Reply(m, data); err != nil {
		b.logger.Warnf("provisioner: reply approval decision: %v", err)
	}
}
