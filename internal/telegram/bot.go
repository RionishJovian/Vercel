// Package telegram holds the shared, injectable Telegram transport seam used by
// BOTH the provisioner approve-gate and the billing notify path — a single seam,
// so a later real Bot-API transport is wired once. Nothing here touches a real
// bot; tests inject fakes.
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
	// allowChat, when >0, filters received updates to one chat (the operator).
	allowChat int64

	mu     sync.Mutex
	offset int64 // next update_id to fetch
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

// NewBot builds a Bot-API transport.
func NewBot(token string, opts ...BotOption) *Bot {
	b := &Bot{
		token:       token,
		base:        "https://api.telegram.org",
		client:      &http.Client{Timeout: 60 * time.Second},
		pollTimeout: 10 * time.Second,
	}
	for _, o := range opts {
		o(b)
	}
	return b
}

var _ Messenger = (*Bot)(nil)
var _ InboundSource = (*Bot)(nil)

// SendMessage implements Messenger.
func (b *Bot) SendMessage(target, text string) error {
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
		return fmt.Errorf("telegram sendMessage: %s", resp.Description)
	}
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
		return false, resp.Description
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
			if m.FromID != 0 && b.allowChat > 0 && m.ChatID != b.allowChat {
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
	for {
		msgs, err := b.nextBatch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return InboundMessage{}, ctx.Err()
			}
			// transient — long-poll short window then retry
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if len(msgs) > 0 {
			return msgs[0], nil
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
	vals.Set("allowed_updates", "[\"message\"]")
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
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := b.do(ctx, "getUpdates", vals, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("telegram getUpdates: %s", resp.Description)
	}

	var pass []InboundMessage
	for _, u := range resp.Result {
		if u.UpdateID >= b.offset {
			b.offset = u.UpdateID + 1
		}
		if u.Message == nil {
			continue
		}
		m := InboundMessage{
			UpdateID:  u.UpdateID,
			ChatID:    u.Message.Chat.ID,
			MessageID: u.Message.MessageID,
			Text:      u.Message.Text,
		}
		if u.Message.From != nil {
			m.FromID = u.Message.From.ID
		}
		if b.allowChat > 0 && (m.ChatID != b.allowChat) {
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
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram %s: http %d: %s", method, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}
