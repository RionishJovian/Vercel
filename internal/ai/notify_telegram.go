package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"mega-magic/internal/model"
	"mega-magic/internal/telegram"
)

// TelegramNotifier pushes a short human-readable AI-incident alert to an
// operator Telegram chat. It implements IncidentNotifier.
//
// It is BEST-EFFORT: a send failure is logged and swallowed — the incident is
// already persisted, so a notify failure must never block, undo, or re-raise it.
// A failed line is kept in memory and resent by Run until it is delivered or an hour old
// (shortcut: the queue is lost if the process stops, which is acceptable because the
// incident itself is stored and the usual cause is the receiver still starting).
// Nil-safe: with no messenger or no target it is a no-op. It never logs the bot
// token (the messenger owns the token; this type never sees or echoes it).
type TelegramNotifier struct {
	msg    telegram.Messenger
	target string
	logger *zap.SugaredLogger

	mu      sync.Mutex
	pending []unsentLine
}

type unsentLine struct {
	text  string
	since time.Time
}

const (
	incidentRetryEvery = time.Minute
	incidentRetryMax   = time.Hour
	incidentQueueMax   = 50
)

// NewTelegramNotifier builds the notifier. A nil messenger or empty target makes
// IncidentOpened a no-op.
func NewTelegramNotifier(msg telegram.Messenger, target string, logger *zap.SugaredLogger) *TelegramNotifier {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
	return &TelegramNotifier{msg: msg, target: strings.TrimSpace(target), logger: logger}
}

// IncidentOpened sends one plain-text line. Never returns an error; failures are
// logged only.
func (n *TelegramNotifier) IncidentOpened(_ string, inc *model.Incident) {
	if n == nil || n.msg == nil || n.target == "" || inc == nil {
		return
	}
	text := formatIncidentLine(inc)
	if err := n.msg.SendMessage(n.target, text); err != nil {
		n.logger.Warnf("ai: telegram incident notify failed, will retry: %v", err)
		n.mu.Lock()
		if len(n.pending) >= incidentQueueMax {
			n.pending = n.pending[1:]
		}
		n.pending = append(n.pending, unsentLine{text: text, since: time.Now()})
		n.mu.Unlock()
	}
}

// Flush resends queued lines; delivered ones and ones older than an hour leave the queue.
func (n *TelegramNotifier) Flush(now time.Time) {
	if n == nil || n.msg == nil || n.target == "" {
		return
	}
	n.mu.Lock()
	queue := n.pending
	n.pending = nil
	n.mu.Unlock()
	var keep []unsentLine
	for _, l := range queue {
		if now.Sub(l.since) > incidentRetryMax {
			n.logger.Warnf("ai: telegram incident alert dropped after an hour without delivery")
			continue
		}
		if err := n.msg.SendMessage(n.target, l.text); err != nil {
			keep = append(keep, l)
		}
	}
	if len(keep) > 0 {
		n.mu.Lock()
		n.pending = append(keep, n.pending...)
		n.mu.Unlock()
	}
}

// Run resends undelivered alerts every minute until ctx ends.
func (n *TelegramNotifier) Run(ctx context.Context) {
	tick := time.NewTicker(incidentRetryEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			n.Flush(now)
		}
	}
}

// formatIncidentLine renders the single-line alert:
//
//	Platform alert [critical] Router dev-9 latency_spike: latency 540ms (incident <id>) at <UTC>
func formatIncidentLine(inc *model.Incident) string {
	sev := strings.TrimSpace(inc.Severity)
	if sev == "" {
		sev = "info"
	}
	subject := strings.TrimSpace(inc.SubjectLabel)
	if subject == "" {
		subject = strings.TrimSpace(inc.SubjectID)
	}
	if subject == "" {
		subject = "unknown"
	}

	line := fmt.Sprintf("Platform alert [%s] %s %s", sev, subject, strings.TrimSpace(inc.Kind))
	if inc.Detail != nil && strings.TrimSpace(*inc.Detail) != "" {
		line += ": " + strings.TrimSpace(*inc.Detail)
	}
	return line + fmt.Sprintf(" (incident %s) at %s", inc.ID, inc.OpenedAt.UTC().Format(time.RFC3339))
}

// Fanout emits each incident to every configured notifier. This is the Note-1
// COMPOSE choice: the existing webhook notifier and the operator Telegram push
// both fire (nothing is replaced). Nil sinks are dropped at construction.
type Fanout struct {
	sinks []IncidentNotifier
}

// NewFanout composes the given notifiers; nil entries are ignored.
func NewFanout(sinks ...IncidentNotifier) *Fanout {
	kept := make([]IncidentNotifier, 0, len(sinks))
	for _, s := range sinks {
		if s != nil {
			kept = append(kept, s)
		}
	}
	return &Fanout{sinks: kept}
}

// IncidentOpened fans out to every sink.
func (f *Fanout) IncidentOpened(businessID string, inc *model.Incident) {
	if f == nil {
		return
	}
	for _, s := range f.sinks {
		s.IncidentOpened(businessID, inc)
	}
}
