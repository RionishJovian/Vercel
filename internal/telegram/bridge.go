package telegram

import (
	"context"
	"strings"
	"time"
)

// PurchaseDecider is the money-path seam the bridge needs to act on a reply.
// BillingHandler implements it; tests inject a fake.
type PurchaseDecider interface {
	// ResolveRequestBusiness returns the business that owns a request id, or a
	// not-found error.
	ResolveRequestBusiness(requestID string) (businessID string, err error)
	// DecidePurchaseRequest approves (approve=true) or rejects (false) a request.
	DecidePurchaseRequest(businessID, requestID string, approve bool) error
}

// PurchaseBridge runs a continuous inbound loop: it long-polls the bot for
// operator replies ("approve <id>" / "reject <id>") and dispatches each to the
// decide-core. One instance owns the update stream — start at most one per bot
// consumer.
//
// Fail closed: malformed/unknown messages are ignored (never acted on); an
// unparseable verdict or a business-resolution error means NO decision is
// issued. Acks are best-effort via the same messenger.
type PurchaseBridge struct {
	src       InboundSource
	msg       Messenger
	target    string // chat the operator replies from (ack destination)
	decider   PurchaseDecider
	minDelay  time.Duration // optional spacing between decisions
	authorize func(InboundMessage, string) bool
}

// SetAuthorization installs a fail-closed guard for tenant-owned guest bots.
// It runs before lookup and again with the resolved business before any decision.
func (b *PurchaseBridge) SetAuthorization(guard func(InboundMessage, string) bool) {
	b.authorize = guard
}

// NewPurchaseBridge builds the bridge.
func NewPurchaseBridge(src InboundSource, msg Messenger, target string, decider PurchaseDecider) *PurchaseBridge {
	return &PurchaseBridge{src: src, msg: msg, target: target, decider: decider}
}

// Run consumes inbound messages until ctx is done.
func (b *PurchaseBridge) Run(ctx context.Context) error {
	for {
		m, err := b.src.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			// Next already retries transient errors; a hard error here means stop.
			return err
		}
		b.handle(m)
	}
}

func (b *PurchaseBridge) handle(m InboundMessage) {
	if b.authorize != nil && !b.authorize(m, "") {
		if bot, ok := b.msg.(*Bot); ok && m.CallbackID != "" {
			_ = bot.AnswerCallback(m.CallbackID, "You are not authorized to approve this payment")
		}
		return
	}
	requestID, approve, ok := ParsePurchaseReply(m.Text)
	if !ok {
		// Unparseable (or an inline tap we already consumed): ack so the tap
		// spinner clears even on an unrecognised/skipped update.
		if bot, down := b.msg.(*Bot); down && m.CallbackID != "" {
			_ = bot.AnswerCallback(m.CallbackID, "Unknown request")
		}
		return // fail closed: never act on an unparseable reply
	}
	verdict := "unreadable"
	businessID, err := b.decider.ResolveRequestBusiness(requestID)
	if err != nil {
		b.ack("We couldn't find this order. Please review it in Mega Magic.")
		if bot, down := b.msg.(*Bot); down && m.CallbackID != "" {
			_ = bot.AnswerCallback(m.CallbackID, "Request not found")
		}
		return
	}
	if b.authorize != nil && !b.authorize(m, businessID) {
		if bot, ok := b.msg.(*Bot); ok && m.CallbackID != "" {
			_ = bot.AnswerCallback(m.CallbackID, "You are not authorized to approve this payment")
		}
		return
	}
	if err := b.decider.DecidePurchaseRequest(businessID, requestID, approve); err != nil {
		b.ack("Couldn't update order " + OrderReference(requestID) + ". Please check its status in Mega Magic before trying again.")
		if bot, down := b.msg.(*Bot); down && m.CallbackID != "" {
			_ = bot.AnswerCallback(m.CallbackID, "Decision failed")
		}
		return
	}
	verdict = "approved"
	if !approve {
		verdict = "rejected"
	}
	b.ack("Mega Magic: Order " + OrderReference(requestID) + " " + verdict + ".")
	// Answer the button tap so Telegram clears the loading spinner, then drop
	// the inline keyboard so there are no buttons left to tap (idempotent UI).
	if bot, down := b.msg.(*Bot); down {
		shown := "Approved"
		if !approve {
			shown = "Rejected"
		}
		if m.CallbackID != "" {
			_ = bot.AnswerCallback(m.CallbackID, shown)
		}
		_ = bot.RemoveButtons(m.ChatID, m.MessageID)
	}
}

func (b *PurchaseBridge) ack(text string) {
	if b.msg == nil || b.target == "" {
		return
	}
	_ = b.msg.SendMessage(b.target, text)
}

// ParsePurchaseReply parses an operator Telegram reply into a purchase decision.
// Expected shape: a verdict word plus a request id, e.g. "approve a1b2c3".
// Strict and default-deny: blank, ambiguous, or unrecognised input returns
// ok=false so the caller never misacts.
func ParsePurchaseReply(text string) (requestID string, approve bool, ok bool) {
	toks := strings.Fields(strings.TrimSpace(text))
	if len(toks) != 2 {
		return "", false, false
	}
	verdict := strings.ToLower(strings.TrimLeft(toks[0], " /")) // tolerate bot-style "/approve"
	requestID = toks[1]
	switch verdict {
	case "approve", "approved", "yes", "y", "ok", "accept":
		return requestID, true, true
	case "reject", "rejected", "no", "n", "deny", "cancel":
		return requestID, false, true
	default:
		return "", false, false
	}
}
