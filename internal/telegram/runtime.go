package telegram

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"
)

// Binding is resolved server-side; a browser can never choose Platform or ActorID.
type Binding struct {
	BusinessID, Token, ActorID string
	ChatID, ApproverID         int64
	Enabled, Platform          bool
	Reserved                   bool
}
type BindingSource interface {
	List(context.Context) ([]Binding, error)
	Get(context.Context, string) (Binding, error)
}
type RuntimeStatus struct {
	Scope              string     `json:"scope"`
	Receiver           string     `json:"receiver"`
	Token              string     `json:"token"`
	ApproverConfigured bool       `json:"approver_configured"`
	LastPollAt         *time.Time `json:"last_poll_at,omitempty"`
	LastDelivery       string     `json:"last_delivery"`
	LastDeliveryAt     *time.Time `json:"last_delivery_at,omitempty"`
	Detail             string     `json:"detail,omitempty"`
}
type runtimeEntry struct {
	binding Binding
	bot     *Bot
	cancel  context.CancelFunc
	done    chan struct{}
	status  RuntimeStatus
}

// Runtime owns exactly one inbound stream per saved, enabled token. Both sends
// and decisions resolve current bindings, so disable/rebind takes effect immediately.
type Runtime struct {
	source    BindingSource
	platform  func(Binding) PurchaseDecider
	business  PurchaseDecider
	factory   func(string, ...BotOption) *Bot
	mu        sync.Mutex
	reconcile sync.Mutex
	entries   map[string]*runtimeEntry
	wake      chan struct{}
}

func NewRuntime(source BindingSource, platform func(Binding) PurchaseDecider, business PurchaseDecider) *Runtime {
	return &Runtime{source: source, platform: platform, business: business, factory: NewBot, entries: map[string]*runtimeEntry{}, wake: make(chan struct{}, 1)}
}
func (r *Runtime) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}
func (r *Runtime) Run(ctx context.Context) {
	defer r.stop()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		r.Reconcile(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-r.wake:
		}
	}
}
func (r *Runtime) stop() {
	r.mu.Lock()
	var done []chan struct{}
	for _, e := range r.entries {
		if e.cancel != nil {
			e.cancel()
			done = append(done, e.done)
		}
	}
	r.mu.Unlock()
	for _, ch := range done {
		<-ch
	}
}

func bindingScope(b Binding) string {
	if b.Platform {
		return "platform"
	}
	return "business"
}
func initialStatus(b Binding) RuntimeStatus {
	return RuntimeStatus{Scope: bindingScope(b), Receiver: "disabled", Token: "unknown", ApproverConfigured: b.ChatID != 0 && b.ApproverID > 0 && (b.ChatID < 0 || b.ChatID == b.ApproverID), LastDelivery: "not_tested"}
}
func (r *Runtime) Status(id string) RuntimeStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, err := r.source.Get(ctx, id)
	if err != nil {
		return RuntimeStatus{Scope: "business", Receiver: "unavailable", Token: "unknown", LastDelivery: "not_tested"}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.entries[id]; e != nil && e.binding == b {
		return e.status
	}
	status := initialStatus(b)
	if b.Enabled {
		status.Receiver = "starting"
	}
	return status
}

func (r *Runtime) update(e *runtimeEntry, f func(*RuntimeStatus)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries[e.binding.BusinessID] == e {
		f(&e.status)
	}
}
func (r *Runtime) Reconcile(ctx context.Context) {
	r.reconcile.Lock()
	defer r.reconcile.Unlock()
	bindings, err := r.source.List(ctx)
	if err != nil {
		r.mu.Lock()
		for _, e := range r.entries {
			e.status.Receiver = "error"
			e.status.Detail = "Settings could not be read"
			if e.cancel != nil {
				e.cancel()
			}
		}
		r.mu.Unlock()
		return
	}
	tokens := map[string]int{}
	platforms := 0
	for _, b := range bindings {
		if b.Enabled {
			tokens[b.Token]++
			if b.Platform {
				platforms++
			}
		}
	}
	wanted := map[string]bool{}
	for _, b := range bindings {
		wanted[b.BusinessID] = true
		r.mu.Lock()
		old := r.entries[b.BusinessID]
		r.mu.Unlock()
		if old != nil && old.binding == b && old.cancel != nil && tokens[b.Token] == 1 && (!b.Platform || platforms == 1) {
			select {
			case <-old.done:
			default:
				continue
			}
		}
		if old != nil && old.cancel != nil {
			old.cancel()
			select {
			case <-old.done:
			case <-ctx.Done():
				return
			}
		}
		e := &runtimeEntry{binding: b, status: initialStatus(b)}
		if old != nil && old.binding == b {
			r.mu.Lock()
			e.status.Token = old.status.Token
			e.status.LastDelivery = old.status.LastDelivery
			e.status.LastDeliveryAt = old.status.LastDeliveryAt
			r.mu.Unlock()
		}

		r.mu.Lock()
		r.entries[b.BusinessID] = e
		r.mu.Unlock()
		if !b.Enabled {
			continue
		}
		if b.Reserved {
			r.update(e, func(s *RuntimeStatus) {
				s.Receiver = "error"
				s.Detail = "This token is already reserved for router approvals; use a separate account bot"
			})
			continue
		}
		if b.ChatID > 0 && b.ChatID != b.ApproverID {
			r.update(e, func(s *RuntimeStatus) {
				s.Receiver = "error"
				s.Detail = "For a private chat, the approval chat ID must equal the approver's Telegram user ID"
			})
			continue
		}
		if b.Token == "" || b.ChatID == 0 || b.ApproverID <= 0 || (b.Platform && b.ActorID == "") {
			r.update(e, func(s *RuntimeStatus) {
				s.Receiver = "error"
				s.Detail = "Configure the bot, chat and authorized approver"
			})
			continue
		}
		if tokens[b.Token] != 1 || (b.Platform && platforms != 1) {
			r.update(e, func(s *RuntimeStatus) {
				s.Receiver = "error"
				s.Detail = "A bot must belong to one account; only one platform binding is allowed"
			})
			continue
		}
		bot := r.factory(b.Token, WithAllowChat(b.ChatID), WithPollObserver(func(err error) {
			r.update(e, func(s *RuntimeStatus) {
				if err != nil {
					s.Receiver = "error"
					s.Detail = err.Error()
					return
				}
				now := time.Now().UTC()
				s.LastPollAt = &now
				s.Receiver = "running"
				s.Detail = ""
			})
		}), WithDeliveryObserver(func(err error) {
			r.update(e, func(s *RuntimeStatus) {
				now := time.Now().UTC()
				s.LastDeliveryAt = &now
				s.LastDelivery = "delivered"
				if err != nil {
					s.LastDelivery = "failed"
				}
			})
		}))
		r.mu.Lock()
		e.bot = bot
		r.mu.Unlock()
		r.update(e, func(s *RuntimeStatus) { s.Receiver = "starting" })
		// Probe without sending or consuming updates. Webhooks must be removed by
		// the owner explicitly; never delete another consumer's webhook automatically.
		probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err = e.bot.Probe(probeCtx)
		cancel()
		if err != nil {
			r.update(e, func(s *RuntimeStatus) { s.Receiver = "error"; s.Token = "unverified"; s.Detail = err.Error() })
			continue
		}
		r.update(e, func(s *RuntimeStatus) { s.Token = "valid" })
		decider := r.business
		if b.Platform {
			decider = r.platform(b)
		}
		bridge := NewPurchaseBridge(e.bot, e.bot, strconv.FormatInt(b.ChatID, 10), decider)
		bridge.SetAuthorization(func(m InboundMessage, businessID string) bool {
			checkCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			current, err := r.source.Get(checkCtx, b.BusinessID)
			return err == nil && bindingAuthorized(b, current, m, businessID)
		})
		loopCtx, stop := context.WithCancel(ctx)
		r.mu.Lock()
		e.cancel = stop
		e.done = make(chan struct{})
		r.mu.Unlock()
		go func() { defer close(e.done); _ = bridge.Run(loopCtx) }()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, e := range r.entries {
		if !wanted[id] {
			if e.cancel != nil {
				e.cancel()
			}
			delete(r.entries, id)
		}
	}
}

func (r *Runtime) currentBot(id string) (*Bot, Binding, error) {
	readCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	b, err := r.source.Get(readCtx, id)
	if err != nil || !b.Enabled {
		return nil, b, errors.New("Telegram binding is unavailable")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[id]
	if e == nil || e.binding != b || e.bot == nil || e.status.Receiver != "running" {
		return nil, b, errors.New("Telegram receiver is not running")
	}
	return e.bot, b, nil
}
func (r *Runtime) platformBot() (*Bot, Binding, error) {
	readCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	bindings, err := r.source.List(readCtx)
	if err != nil {
		return nil, Binding{}, errors.New("Telegram settings unavailable")
	}
	var id string
	for _, b := range bindings {
		if b.Platform && b.Enabled {
			if id != "" {
				return nil, Binding{}, errors.New("Ambiguous platform Telegram binding")
			}
			id = b.BusinessID
		}
	}
	if id == "" {
		return nil, Binding{}, errors.New("Platform Telegram bot is not enabled")
	}
	return r.currentBot(id)
}

// Messenger implementation is deliberately platform-only; caller-supplied
// destinations cannot reroute a platform payment to a customer's chat.
func (r *Runtime) SendMessage(_ string, text string) error {
	bot, b, err := r.platformBot()
	if err != nil {
		return r.deliveryFailed(b, err)
	}
	return bot.SendMessage(strconv.FormatInt(b.ChatID, 10), text)
}
func (r *Runtime) SendButtons(_ string, text, id string) error {
	bot, b, err := r.platformBot()
	if err != nil {
		return r.deliveryFailed(b, err)
	}
	return bot.SendButtons(strconv.FormatInt(b.ChatID, 10), text, id)
}
func (r *Runtime) VerifyToken() (bool, string)                             { return false, "Test the account's saved bot" }
func (r *Runtime) AwaitReply(string, time.Duration) (string, string, bool) { return "", "", false }

// MessengerFor reuses the account's existing receiver for guest notifications
// and media receipts. No second getUpdates consumer is created.
func (r *Runtime) MessengerFor(token string) (Messenger, bool) {
	readCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	bindings, err := r.source.List(readCtx)
	if err != nil {
		return nil, false
	}
	for _, b := range bindings {
		if b.Token == token {
			bot, _, err := r.currentBot(b.BusinessID)
			return bot, err == nil
		}
	}
	return nil, false
}

func (r *Runtime) Probe(ctx context.Context, id string) error {
	b, err := r.source.Get(ctx, id)
	if err != nil {
		return errors.New("Telegram binding unavailable")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	err = r.factory(b.Token).Probe(probeCtx)
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.entries[id]; e != nil && e.binding == b {
		e.status.Token = "valid"
		if err != nil {
			e.status.Token = "unverified"
		}
	}
	return err
}

// Set once during server construction, before Run starts.
func (r *Runtime) SetBusinessDecider(d PurchaseDecider) { r.business = d }

// Explicit owner action only: this never runs as part of saving/probing a token.
func (r *Runtime) SendTest(ctx context.Context, id string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	bot, b, err := r.currentBot(id)
	if err != nil {
		return r.deliveryFailed(b, err)
	}
	return bot.SendMessage(strconv.FormatInt(b.ChatID, 10), "Mega Magic test notification. Your saved Telegram chat can receive messages. No payment was approved and no router settings were changed.")
}

func bindingAuthorized(started, current Binding, m InboundMessage, businessID string) bool {
	return current == started && current.Enabled && current.ApproverID > 0 && current.ChatID != 0 && m.FromID == current.ApproverID && m.ChatID == current.ChatID && (current.Platform || businessID == "" || businessID == current.BusinessID)
}

func (r *Runtime) deliveryFailed(b Binding, err error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.entries[b.BusinessID]; e != nil && e.binding == b {
		now := time.Now().UTC()
		e.status.LastDelivery = "failed"
		e.status.LastDeliveryAt = &now
	}
	return err
}
