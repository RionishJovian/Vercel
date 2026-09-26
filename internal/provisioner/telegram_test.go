package provisioner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// --- fake transport seam ----------------------------------------------------

type sentMsg struct{ target, text string }

type fakeMessenger struct {
	sent      []sentMsg
	sendErr   error
	reply     string
	replyID   string
	replyOK   bool
	bindID    bool // when true, echo the awaited request id as the reply id
	awaitedID string
}

func (f *fakeMessenger) SendMessage(target, text string) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.sent = append(f.sent, sentMsg{target, text})
	return nil
}

func (f *fakeMessenger) AwaitReply(requestID string, _ time.Duration) (string, string, bool) {
	f.awaitedID = requestID
	if f.bindID {
		return f.reply, requestID, f.replyOK
	}
	return f.reply, f.replyID, f.replyOK
}

func reqFixtures() ApprovalRequest {
	return ApprovalRequest{
		BusinessID:  "biz-1",
		PeerID:      "peer2",
		Changes:     highRiskPlan(),
		RequiresOTP: true,
		RequestedAt: time.Unix(59, 0).UTC(),
	}
}

// --- formatter --------------------------------------------------------------

func TestFormatApprovalMessage(t *testing.T) {
	msg := FormatApprovalMessage(reqFixtures(), "abcd1234")
	for _, want := range []string{"biz-1", "peer2", "abcd1234", "rotate_listen_port", "escalate", "HIGH RISK", "approve", "reject"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("formatted message missing %q:\n%s", want, msg)
		}
	}
	// Low-risk rendering must not claim OTP.
	low := reqFixtures()
	low.RequiresOTP = false
	low.Changes = lowMedPlan()
	if m := FormatApprovalMessage(low, "id"); strings.Contains(m, "HIGH RISK") || !strings.Contains(m, "no OTP needed") {
		t.Fatalf("low-risk message wrong:\n%s", m)
	}
}

// --- parser decision table --------------------------------------------------

func TestParseApprovalReply_table(t *testing.T) {
	cases := []struct {
		name        string
		text        string
		wantOK      bool
		wantApprove bool
		wantOTP     string
	}{
		{"approve", "approve", true, true, ""},
		{"yes", "yes", true, true, ""},
		{"uppercase", "APPROVE", true, true, ""},
		{"approve with otp", "approve 94287082", true, true, "94287082"},
		{"approve 6-digit otp", "approve 123456", true, true, "123456"},
		{"reject", "reject", true, false, ""},
		{"no", "no", true, false, ""},
		{"deny", "deny", true, false, ""},
		{"reject with code", "reject 123456", true, false, ""},
		{"empty", "", false, false, ""},
		{"garbage", "maybe later", false, false, ""},
		{"ambiguous approve+reject", "approve reject", false, false, ""},
		{"ambiguous two codes", "approve 111111 222222", false, false, ""},
		{"unrecognised word", "please approve", false, false, ""},
		{"verdict missing", "123456", false, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec, ok := ParseApprovalReply(tc.text)
			if ok != tc.wantOK {
				t.Fatalf("recognized = %v, want %v (%q)", ok, tc.wantOK, tc.text)
			}
			if ok && dec.Approved != tc.wantApprove {
				t.Fatalf("approved = %v, want %v", dec.Approved, tc.wantApprove)
			}
			if ok && dec.OTP != tc.wantOTP {
				t.Fatalf("otp = %q, want %q", dec.OTP, tc.wantOTP)
			}
			if !ok && dec.Approved {
				t.Fatal("unrecognised reply must never approve")
			}
		})
	}
}

// --- approver ---------------------------------------------------------------

func TestTelegramApprover_approveAndReject(t *testing.T) {
	ctx := context.Background()
	req := reqFixtures()

	m := &fakeMessenger{reply: "approve 94287082", bindID: true, replyOK: true}
	dec, err := NewTelegramApprover(m, "chat-1", time.Second).Await(ctx, "biz-1", req)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}
	if !dec.Approved || dec.OTP != "94287082" || dec.Approver != "telegram" {
		t.Fatalf("decision = %+v, want approved with otp", dec)
	}
	if len(m.sent) != 1 || m.sent[0].target != "chat-1" {
		t.Fatalf("one message to chat-1 expected, got %+v", m.sent)
	}
	if m.awaitedID == "" {
		t.Fatal("approver must await a reply bound to the request id")
	}

	rm := &fakeMessenger{reply: "reject", bindID: true, replyOK: true}
	dec, _ = NewTelegramApprover(rm, "chat-1", time.Second).Await(ctx, "biz-1", req)
	if dec.Approved {
		t.Fatal("reject must deny")
	}
}

func TestTelegramApprover_defaultsDeny(t *testing.T) {
	ctx := context.Background()
	req := reqFixtures()

	cases := []struct {
		name string
		m    *fakeMessenger
	}{
		{"timeout", &fakeMessenger{bindID: true, replyOK: false}},
		{"empty reply", &fakeMessenger{reply: "", bindID: true, replyOK: true}},
		{"ambiguous", &fakeMessenger{reply: "approve reject", bindID: true, replyOK: true}},
		{"garbage", &fakeMessenger{reply: "huh?", bindID: true, replyOK: true}},
		{"send failure", &fakeMessenger{sendErr: errors.New("telegram down"), bindID: true, replyOK: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec, err := NewTelegramApprover(tc.m, "chat-1", time.Second).Await(ctx, "biz-1", req)
			if err != nil {
				t.Fatalf("must deny without error, got %v", err)
			}
			if dec.Approved {
				t.Fatalf("must DENY: %+v", dec)
			}
		})
	}
}

func TestTelegramApprover_staleReplyRejected(t *testing.T) {
	// A reply correlated with a different request id must not approve.
	m := &fakeMessenger{reply: "approve", replyID: "some-other-request", replyOK: true}
	dec, _ := NewTelegramApprover(m, "chat-1", time.Second).Await(context.Background(), "biz-1", reqFixtures())
	if dec.Approved {
		t.Fatal("stale/other request's reply must not approve this plan")
	}
}

func TestTelegramApprover_missingMessengerDenies(t *testing.T) {
	dec, err := NewTelegramApprover(nil, "chat", time.Second).Await(context.Background(), "biz-1", reqFixtures())
	if err != nil || dec.Approved {
		t.Fatalf("nil messenger = (%+v,%v), want deny", dec, err)
	}
}

func TestConfigTelegramMessenger_noLiveBot(t *testing.T) {
	// No token => inert (fail closed). Live-bot mounting is gated on a token.
	if _, ok := ConfigTelegramMessenger("", "12345", time.Minute); ok {
		t.Fatal("empty token must not wire a live bot messenger")
	}
}

// TestTelegramApprover_highRiskEndToEnd proves the bridge decision satisfies the
// Slice 2 TOTP gate for a high-risk plan.
func TestTelegramApprover_highRiskEndToEnd(t *testing.T) {
	ctx := context.Background()
	req := reqFixtures() // high risk, RequiresOTP true

	dec, err := NewTelegramApprover(&fakeMessenger{reply: "approve 94287082", bindID: true, replyOK: true}, "chat-1", time.Second).Await(ctx, "biz-1", req)
	if err != nil {
		t.Fatalf("Await: %v", err)
	}

	verifier := NewTOTPVerifier(TOTPConfig{Secret: rfcSecretB32, Digits: 8, Now: fixedClock(59)})
	out, err := EvaluateApproval(ctx, req.PeerID, req.Changes, dec, verifier)
	if err != nil {
		t.Fatalf("EvaluateApproval: %v", err)
	}
	if !out.Allowed {
		t.Fatalf("approve + valid OTP must be allowed, got %+v", out)
	}

	// Without the OTP, the same human approval must be denied.
	dec2, _ := NewTelegramApprover(&fakeMessenger{reply: "approve", bindID: true, replyOK: true}, "chat-1", time.Second).Await(ctx, "biz-1", req)
	if out2, _ := EvaluateApproval(ctx, req.PeerID, req.Changes, dec2, verifier); out2.Allowed {
		t.Fatal("high-risk approval without OTP must be denied")
	}
}
