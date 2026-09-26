// Package telegram holds the shared Telegram transport seam used by BOTH the
// provisioner approve-gate and the billing notify/bridge path — a single seam,
// so the real Bot-API transport (Bot) is wired in one place and both callers
// depend only on the interface. Tests inject fakes.
package telegram

import (
	"strconv"
	"strings"
	"time"
)

// Messenger is the injectable transport seam for the bot.
type Messenger interface {
	// SendMessage delivers text to target (a chat/channel id).
	SendMessage(target, text string) error
	// VerifyToken probes the token (Telegram getMe): ok=true when live.
	VerifyToken() (ok bool, detail string)
	// AwaitReply waits for the inbound reply correlated with requestID and
	// returns the reply text, the request id it was actually correlated with,
	// and ok=false on timeout/no reply.
	AwaitReply(requestID string, timeout time.Duration) (text, replyID string, ok bool)
}

// ConfigMessenger builds the real Bot transport when a token is configured,
// returning ok=false (inert) when the feature is disabled. Fail closed: no
// token => no bot => callers stay inert.
func ConfigMessenger(token string, chatID string, poolTimeout time.Duration) (Messenger, bool) {
	if strings.TrimSpace(token) == "" {
		return nil, false
	}
	opts := []BotOption{WithPollTimeout(poolTimeout)}
	if id, err := strconv.ParseInt(strings.TrimSpace(chatID), 10, 64); err == nil && id > 0 {
		opts = append(opts, WithAllowChat(id))
	}
	return NewBot(strings.TrimSpace(token), opts...), true
}
