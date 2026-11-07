// Package telegram holds the shared, injectable Telegram transport seam used by
// the provisioner approval gate and account notification/decision paths.
// Production uses the Bot API; tests use injected local transports.
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Inbound

// InboundMessage is one inbound bot message delivered by the bridge source.
type InboundMessage struct {
	UpdateID  int64
	ChatID    int64
	FromID    int64
	Text      string
	MessageID int64
	// CallbackID is set when the update was an inline-button tap (Telegram
	// callback_query); the bridge answers it so the spinner clears.
	CallbackID string
}

// InboundSource yields inbound messages (long-polling). Next blocks until a
// message is available or ctx is done.
type InboundSource interface {
	Next(ctx context.Context) (InboundMessage, error)
}

// Bot is a real Telegram Bot-API transport. It implements Messenger (send +
// wait-for-reply, used by the provisioner approve-gate) and InboundSource (a
// continuous getUpdates poller, used by the billing bridge). One Bot instance
// owns ONE update stream — use a dedicated Bot per consumer so offsets don't
// fight.
type Bot struct {
	token  string
	base   string // default https://api.telegram.org
	client *http.Client

	// long-poll window for getUpdates
	pollTimeout time.Duration
	// allowChat, when nonzero, filters received updates to one chat (the operator).
	allowChat int64

	mu         sync.Mutex
	offset     int64 // next update_id to fetch
	nextMu     sync.Mutex
	pending    []InboundMessage
	onPoll     func(error)
	onDelivery func(error)
}

// BotOption configures a Bot.
type BotOption func(*Bot)

// WithBase overrides the API base URL (testing). Must be otherwise empty.
func WithBase(base string) BotOption {
	return func(b *Bot) { b.base = strings.TrimSuffix(base, "/") }
}

// WithPollTimeout sets the getUpdates long-poll window.
func WithPollTimeout(d time.Duration) BotOption { return func(b *Bot) { b.pollTimeout = d } }

// WithAllowChat filters inbound updates to a single operator chat id.
func WithAllowChat(chatID int64) BotOption { return func(b *Bot) { b.allowChat = chatID } }

// Observers receive sanitized errors and never credentials.
func WithPollObserver(f func(error)) BotOption     { return func(b *Bot) { b.onPoll = f } }
func WithDeliveryObserver(f func(error)) BotOption { return func(b *Bot) { b.onDelivery = f } }
func (b *Bot) delivered(err error) {
	if b.onDelivery != nil {
		b.onDelivery(err)
	}
}

// NewBot builds a Bot-API transport.
func NewBot(token string, opts ...BotOption) *Bot {
	b := &Bot{
		token:       token,
		base:        "https://api.telegram.org",
		client:      &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		pollTimeout: 10 * time.Second,
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

var _ Messenger = (*Bot)(nil)
var _ InboundSource = (*Bot)(nil)
var _ ButtonMessenger = (*Bot)(nil)

// SendMessage implements Messenger.
func (b *Bot) SendMessage(target, text string) (err error) {
	defer func() { b.delivered(err) }()
	// target may be a numeric chat id (string) or a @username.
	vals := url.Values{}
	vals.Set("chat_id", target)
	vals.Set("text", text)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var resp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := b.do(ctx, "sendMessage", vals, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("telegram sendMessage: %s", strings.ReplaceAll(resp.Description, b.token, "[redacted]"))
	}
	return nil
}

// SendButtons implements ButtonMessenger: posts text with an inline
// Approve/Reject keyboard. callback_data carries the request id so the bridge's
// existing ParsePurchaseReply path acts on a press. The callback is answered by
// the bridge (Bot.AnswerCallback) after it acts, clearing the spinner.
func (b *Bot) SendButtons(target, text, requestID string) (err error) {
	defer func() { b.delivered(err) }()
	markup := fmt.Sprintf(
		`{"inline_keyboard":[[{"text":"Approve","callback_data":"approve %s"},{"text":"Reject","callback_data":"reject %s"}]]}`,
		requestID, requestID)
	vals := url.Values{}
	vals.Set("chat_id", target)
	vals.Set("text", text)
	vals.Set("reply_markup", markup)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var resp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      *struct {
			Message *struct {
				MessageID int64 `json:"message_id"`
			} `json:"message"`
		} `json:"result"`
	}
	if err := b.do(ctx, "sendMessage", vals, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("telegram sendMessage: %s", strings.ReplaceAll(resp.Description, b.token, "[redacted]"))
	}
	return nil
}

// AnswerCallback tells Telegram a button tap was handled, clearing the loading
// spinner. Best-effort: an already-answered or expired callback is a no-op.
func (b *Bot) AnswerCallback(callbackID, text string) error {
	if strings.TrimSpace(callbackID) == "" {
		return nil
	}
	vals := url.Values{}
	vals.Set("callback_query_id", callbackID)
	if text != "" {
		vals.Set("text", text)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var resp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := b.do(ctx, "answerCallbackQuery", vals, &resp); err != nil {
		return err
	}
	return nil
}

// RemoveButtons replaces the inline keyboard on a notify message with an empty
// one (editMessageReplyMarkup + empty inline_keyboard), so there are no buttons
// left to tap after the owner has decided. Best-effort no-op on failure.
func (b *Bot) RemoveButtons(chatID, messageID int64) error {
	if chatID == 0 || messageID == 0 {
		return nil
	}
	vals := url.Values{}
	vals.Set("chat_id", strconv.FormatInt(chatID, 10))
	vals.Set("message_id", strconv.FormatInt(messageID, 10))
	vals.Set("reply_markup", `{"inline_keyboard":[]}`)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// editMessageReplyMarkup drops the buttons and keeps the original text.
	var resp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = b.do(ctx, "editMessageReplyMarkup", vals, &resp)
	return nil
}

// VerifyToken implements a token-sanity probe used by the owner bot-binding
// "test" endpoint: GET me returns ok when the token is live, 401 otherwise.
func (b *Bot) VerifyToken() (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var resp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := b.do(ctx, "getMe", url.Values{}, &resp); err != nil {
		return false, err.Error()
	}
	if !resp.OK {
		return false, strings.ReplaceAll(resp.Description, b.token, "[redacted]")
	}
	return true, ""
}

// AwaitReply implements Messenger: long-polls getUpdates until a message whose
// text mentions requestID arrives, or the timeout elapses.
func (b *Bot) AwaitReply(requestID string, timeout time.Duration) (string, string, bool) {
	deadline := time.Now().Add(timeout)
	ctx := context.Background()
	for time.Now().Before(deadline) {
		msgs, err := b.nextBatch(ctx)
		if err != nil {
			// transient API error — keep polling until deadline
			time.Sleep(1 * time.Second)
			continue
		}
		for _, m := range msgs {
			if m.FromID != 0 && b.allowChat != 0 && m.ChatID != b.allowChat {
				continue
			}
			if strings.Contains(m.Text, requestID) {
				return m.Text, requestID, true
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return "", "", false
}

// Next implements InboundSource.
func (b *Bot) Next(ctx context.Context) (InboundMessage, error) {
	b.nextMu.Lock()
	defer b.nextMu.Unlock()
	for {
		if ctx.Err() != nil {
			return InboundMessage{}, ctx.Err()
		}
		if len(b.pending) > 0 {
			m := b.pending[0]
			b.pending = b.pending[1:]
			return m, nil
		}
		msgs, err := b.nextBatch(ctx)
		if b.onPoll != nil && ctx.Err() == nil {
			b.onPoll(err)
		}
		if err != nil {
			if ctx.Err() != nil {
				return InboundMessage{}, ctx.Err()
			}
			// transient — long-poll short window then retry
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if len(msgs) > 0 {
			b.pending = msgs
			continue
		}
	}
}

// nextBatch performs one getUpdates call and returns only messages that pass
// the allowChat filter, advancing the internal offset.
func (b *Bot) nextBatch(ctx context.Context) ([]InboundMessage, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	vals := url.Values{}
	vals.Set("timeout", strconv.Itoa(int(b.pollTimeout.Seconds())))
	vals.Set("allowed_updates", `["message","callback_query"]`)
	if b.offset > 0 {
		vals.Set("offset", strconv.FormatInt(b.offset, 10))
	}

	var resp struct {
		OK     bool `json:"ok"`
		Result []struct {
			UpdateID int64 `json:"update_id"`
			Message  *struct {
				MessageID int64 `json:"message_id"`
				Chat      *struct {
					ID int64 `json:"id"`
				} `json:"chat"`
				From *struct {
					ID int64 `json:"id"`
				} `json:"from"`
				Text string `json:"text"`
			} `json:"message"`
			CallbackQuery *struct {
				ID   string `json:"id"`
				From *struct {
					ID int64 `json:"id"`
				} `json:"from"`
				Message *struct {
					MessageID int64 `json:"message_id"`
					Chat      *struct {
						ID int64 `json:"id"`
					} `json:"chat"`
				} `json:"message"`
				Data string `json:"data"`
			} `json:"callback_query"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := b.do(ctx, "getUpdates", vals, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("telegram getUpdates: %s", strings.ReplaceAll(resp.Description, b.token, "[redacted]"))
	}

	var pass []InboundMessage
	for _, u := range resp.Result {
		if u.UpdateID >= b.offset {
			b.offset = u.UpdateID + 1
		}
		m := InboundMessage{
			UpdateID:  u.UpdateID,
			ChatID:    -1,
			MessageID: 0,
		}
		if u.Message != nil {
			m.ChatID = u.Message.Chat.ID
			m.MessageID = u.Message.MessageID
			m.Text = u.Message.Text
			if u.Message.From != nil {
				m.FromID = u.Message.From.ID
			}
		} else if u.CallbackQuery != nil {
			// A button tap: surface callback_data as the "text" so the bridge's
			// ParsePurchaseReply ("approve <id>") handles it identically to typed
			// text, record the message id, and keep the callback id so the bridge
			// can answer it (clearing Telegram's loading spinner).
			m.ChatID = -1
			if u.CallbackQuery.Message != nil {
				m.ChatID = u.CallbackQuery.Message.Chat.ID
				m.MessageID = u.CallbackQuery.Message.MessageID
			}
			if u.CallbackQuery.From != nil {
				m.FromID = u.CallbackQuery.From.ID
			}
			m.Text = u.CallbackQuery.Data
			m.CallbackID = u.CallbackQuery.ID
		} else {
			continue
		}
		if b.allowChat != 0 && (m.ChatID != b.allowChat) {
			continue
		}
		pass = append(pass, m)
	}
	return pass, nil
}

func (b *Bot) do(ctx context.Context, method string, vals url.Values, out any) error {
	u := fmt.Sprintf("%s/bot%s/%s", b.base, b.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(vals.Encode()))
	if err != nil {
		return fmt.Errorf("telegram %s: invalid request", method)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: connection failed", method)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("telegram %s: response could not be read", method)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram %s: http %d", method, resp.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// Probe checks credentials and receiver compatibility without consuming updates.
func (b *Bot) Probe(ctx context.Context) error {
	var me struct {
		OK bool `json:"ok"`
	}
	if err := b.do(ctx, "getMe", url.Values{}, &me); err != nil {
		return err
	}
	if !me.OK {
		return fmt.Errorf("Telegram token could not be verified")
	}
	var webhook struct {
		OK     bool `json:"ok"`
		Result struct {
			URL string `json:"url"`
		} `json:"result"`
	}
	if err := b.do(ctx, "getWebhookInfo", url.Values{}, &webhook); err != nil {
		return err
	}
	if !webhook.OK {
		return fmt.Errorf("Telegram webhook status could not be verified")
	}
	if webhook.Result.URL != "" {
		return fmt.Errorf("This bot already has a webhook; disconnect its other integration first")
	}
	return nil
}
