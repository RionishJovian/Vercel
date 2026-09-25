package provisioner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Telegram approve bridge (Slice 3)
// ---------------------------------------------------------------------------
//
// This is the human-in-the-loop side of the manual approve-gate. It RENDERS a
// plan for the Lead, AWAITS their reply, and PARSES it into an ApprovalDecision.
// It NEVER auto-approves: anything empty, ambiguous, malformed, unrecognised, or
// timed out DENIES.
//
// The real Telegram bot send/receive is a documented INJECTED seam
// (TelegramMessenger) — exactly like the SSH/Applier transport seam. Nothing
// here touches a real bot; tests use a fake.

// TelegramMessenger is the injectable transport seam for the bot. A later,
// explicit step supplies a real Bot-API implementation.
type TelegramMessenger interface {
	// SendMessage delivers text to target (a chat/channel id).
	SendMessage(target, text string) error
	// AwaitReply waits for the inbound reply correlated with requestID and
	// returns the reply text, the request id it was actually correlated with,
	// and ok=false on timeout/no reply.
	AwaitReply(requestID string, timeout time.Duration) (text, replyID string, ok bool)
}

// ConfigTelegramMessenger is the documented seam for the REAL bot transport.
//
// Slice 3 deliberately does NOT implement a live telegram bot (no token, no
// inbound routing): this returns ok=false so the bridge stays inert until a
// later explicit step injects a real TelegramMessenger. Fail closed.
func ConfigTelegramMessenger(chatID string) (TelegramMessenger, bool) {
	return nil, false
}

// TelegramApprover implements provisioner.Approver over a TelegramMessenger.
type TelegramApprover struct {
	messenger TelegramMessenger
	target    string // chat id; falls back to businessID when empty
	timeout   time.Duration
	now       func() time.Time
	newID     func() string
}

// NewTelegramApprover builds the approver. A zero timeout defaults to 5 minutes.
func NewTelegramApprover(messenger TelegramMessenger, target string, timeout time.Duration) *TelegramApprover {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &TelegramApprover{
		messenger: messenger,
		target:    target,
		timeout:   timeout,
		now:       func() time.Time { return time.Now().UTC() },
		newID:     newRequestID,
	}
}

var _ Approver = (*TelegramApprover)(nil)

// Await sends the formatted request and returns the human's decision. Every
// failure/ambiguity path denies (Approved=false).
func (a *TelegramApprover) Await(ctx context.Context, businessID string, req ApprovalRequest) (ApprovalDecision, error) {
	deny := ApprovalDecision{Approved: false, Approver: "telegram", At: a.now()}
	if a == nil || a.messenger == nil {
		return deny, nil
	}

	requestID := a.newID()
	target := a.target
	if target == "" {
		target = businessID
	}
	if err := a.messenger.SendMessage(target, FormatApprovalMessage(req, requestID)); err != nil {
		return deny, nil // cannot reach the Lead => stays denied (fail closed)
	}

	reply, replyID, ok := a.messenger.AwaitReply(requestID, a.timeout)
	if !ok {
		return deny, nil // timeout => deny
	}
	if replyID != requestID {
		return deny, nil // stale/other request's reply must not approve this plan
	}

	dec, recognized := ParseApprovalReply(reply)
	if !recognized {
		return deny, nil // ambiguous/unrecognised => deny
	}
	dec.Approver = "telegram"
	dec.At = a.now()
	return dec, nil
}

// FormatApprovalMessage renders an ApprovalRequest into terse, structured text.
func FormatApprovalMessage(req ApprovalRequest, requestID string) string {
	var b strings.Builder
	b.WriteString("MEGA MAGIC - recovery approval needed\n")
	fmt.Fprintf(&b, "request: %s\n", requestID)
	fmt.Fprintf(&b, "business: %s\n", req.BusinessID)
	fmt.Fprintf(&b, "peer: %s\n", req.PeerID)
	b.WriteString("proposed changes:\n")
	if len(req.Changes) == 0 {
		b.WriteString("  (none)\n")
	}
	for i, c := range req.Changes {
		fmt.Fprintf(&b, "  %d. %s [risk=%s] %s -> %s\n", i+1, c.Action, c.Risk, c.Current, c.Proposed)
	}
	if req.RequiresOTP {
		b.WriteString("HIGH RISK: a valid OTP is required with your approval.\n")
	} else {
		b.WriteString("risk: low/medium (no OTP needed)\n")
	}
	b.WriteString("reply: \"approve <OTP>\" or \"reject\"\n")
	return b.String()
}

var (
	approveWords = map[string]bool{"approve": true, "approved": true, "yes": true, "y": true, "ok": true, "accept": true}
	rejectWords  = map[string]bool{"reject": true, "rejected": true, "no": true, "n": true, "deny": true, "cancel": true}
)

// ParseApprovalReply parses a human reply into an ApprovalDecision. It is strict
// and default-deny: unknown tokens, ambiguity (approve AND reject, or multiple
// codes), or empty input yields recognized=false.
func ParseApprovalReply(text string) (ApprovalDecision, bool) {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(tokens) == 0 {
		return ApprovalDecision{}, false
	}

	hasApprove := false
	hasReject := false
	var codes []string

	for _, tok := range tokens {
		switch {
		case approveWords[tok]:
			hasApprove = true
		case rejectWords[tok]:
			hasReject = true
		case isNumericCode(tok):
			codes = append(codes, tok)
		default:
			return ApprovalDecision{}, false // unrecognised token => deny
		}
	}

	if hasApprove && hasReject {
		return ApprovalDecision{}, false // ambiguous => deny
	}
	if !hasApprove && !hasReject {
		return ApprovalDecision{}, false // no verdict => deny
	}
	if len(codes) > 1 {
		return ApprovalDecision{}, false // ambiguous code => deny
	}

	dec := ApprovalDecision{Approved: hasApprove}
	if hasApprove && len(codes) == 1 {
		dec.OTP = codes[0]
	}
	return dec, true
}

func isNumericCode(s string) bool {
	if len(s) < 6 || len(s) > 8 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// newRequestID returns a short-lived request id for reply correlation.
func newRequestID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano()%1000000)
	}
	return hex.EncodeToString(b[:])
}
