package recovery

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"

	"mega-magic/internal/provisioner"
)

type fakeBus struct {
	pattern  string
	handler  func(*nats.Msg)
	replies  [][]byte
	subErr   error
	replyErr error
}

func (b *fakeBus) SubscribePattern(pattern string, h func(*nats.Msg)) error {
	if b.subErr != nil {
		return b.subErr
	}
	b.pattern = pattern
	b.handler = h
	return nil
}

func (b *fakeBus) Reply(_ *nats.Msg, data []byte) error {
	if b.replyErr != nil {
		return b.replyErr
	}
	b.replies = append(b.replies, append([]byte(nil), data...))
	return nil
}

func bridgeReqMsg(t *testing.T) *nats.Msg {
	t.Helper()
	req := provisioner.ApprovalRequest{
		BusinessID:  "biz-1",
		PeerID:      "peer2",
		Changes:     []provisioner.Change{{Action: provisioner.ActionRotateListenPort, Risk: "medium"}},
		RequiresOTP: true,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal req: %v", err)
	}
	return &nats.Msg{Subject: "mm.biz-1.provision.approval", Reply: "_INBOX.abc", Data: data}
}

func TestApprovalBridge_startSubscribes(t *testing.T) {
	bus := &fakeBus{}
	if err := NewApprovalBridge(stubApprover{}, bus, nil).Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if bus.pattern != "mm.*.provision.approval" {
		t.Fatalf("pattern = %q", bus.pattern)
	}
	if bus.handler == nil {
		t.Fatal("handler not registered")
	}
}

func TestApprovalBridge_repliesDecision(t *testing.T) {
	bus := &fakeBus{}
	approver := stubApprover{dec: provisioner.ApprovalDecision{Approved: true, OTP: "94287082"}}
	bridge := NewApprovalBridge(approver, bus, nil)
	if err := bridge.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	bus.handler(bridgeReqMsg(t))

	if len(bus.replies) != 1 {
		t.Fatalf("expected exactly one reply, got %d", len(bus.replies))
	}
	var dec provisioner.ApprovalDecision
	if err := json.Unmarshal(bus.replies[0], &dec); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if !dec.Approved || dec.OTP != "94287082" {
		t.Fatalf("reply decision = %+v, want approved+otp", dec)
	}
}

func TestApprovalBridge_deniesOnApproverError(t *testing.T) {
	bus := &fakeBus{}
	bridge := NewApprovalBridge(stubApprover{err: errors.New("telegram unreachable")}, bus, nil)
	if err := bridge.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	bus.handler(bridgeReqMsg(t))

	var dec provisioner.ApprovalDecision
	_ = json.Unmarshal(bus.replies[0], &dec)
	if dec.Approved {
		t.Fatal("approver error must produce a DENY reply")
	}
}

func TestApprovalBridge_badPayloadRepliesDeny(t *testing.T) {
	bus := &fakeBus{}
	bridge := NewApprovalBridge(stubApprover{dec: provisioner.ApprovalDecision{Approved: true}}, bus, nil)
	if err := bridge.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	bus.handler(&nats.Msg{Subject: "mm.biz-1.provision.approval", Reply: "_INBOX.x", Data: []byte("{not json")})

	if len(bus.replies) != 1 {
		t.Fatalf("bad payload must still yield a DENY reply, got %d replies", len(bus.replies))
	}
	var dec provisioner.ApprovalDecision
	_ = json.Unmarshal(bus.replies[0], &dec)
	if dec.Approved {
		t.Fatal("malformed request must not approve")
	}
}

func TestApprovalBridge_noBus(t *testing.T) {
	if err := NewApprovalBridge(stubApprover{}, nil, nil).Start(); err == nil {
		t.Fatal("nil bus must error")
	}
}

func TestApprovalBridge_subscribeError(t *testing.T) {
	bus := &fakeBus{subErr: errors.New("nats gone")}
	if err := NewApprovalBridge(stubApprover{}, bus, nil).Start(); err == nil {
		t.Fatal("subscribe error must propagate")
	}
}
