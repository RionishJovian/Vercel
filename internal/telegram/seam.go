// Package telegram holds the shared, injectable Telegram transport seam used by
// BOTH the provisioner approve-gate and the billing notify path — a single seam,
// so a later real Bot-API transport is wired once. Nothing here touches a real
// bot; tests inject fakes.
package telegram

import "time"

// Messenger is the injectable transport seam for the bot.
type Messenger interface {
	// SendMessage delivers text to target (a chat/channel id).
	SendMessage(target, text string) error
	// AwaitReply waits for the inbound reply correlated with requestID and
	// returns the reply text, the request id it was actually correlated with,
	// and ok=false on timeout/no reply.
	AwaitReply(requestID string, timeout time.Duration) (text, replyID string, ok bool)
}

// ConfigMessenger is the documented seam for the REAL bot transport. It
// deliberately returns ok=false (no token, no inbound routing) so callers stay
// inert until a later explicit step injects a real Messenger. Fail closed.
func ConfigMessenger(chatID string) (Messenger, bool) {
	return nil, false
}
