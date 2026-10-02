package server

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/telegram"
)

// GuestWifiBridgeSupervisor owns the multi-tenant OWNER bot loops. For every
// business with an enabled owner bot it spawns ONE PurchaseBridge (which owns
// one getUpdates stream) wired to the shared guest decide-core. It re-scans on
// a schedule so newly-enabled/disabled owners are picked up without a restart.
// Fail closed: a bridge only runs for a business that has configured an enabled
// token; a bot with no usable transport is skipped, never acted on.
type GuestWifiBridgeSupervisor struct {
	db      *database.DB
	botRepo *database.BusinessTelegramBotRepository
	decider telegram.PurchaseDecider
	svcFmt  func(token string) (telegram.Messenger, bool)
	mu      sync.Mutex
	running map[string]guestBotLoop
}

type guestBotLoop struct {
	binding model.BusinessTelegramBot
	cancel  context.CancelFunc
}

func NewGuestWifiBridgeSupervisor(
	db *database.DB,
	botRepo *database.BusinessTelegramBotRepository,
	decider telegram.PurchaseDecider,
	svcFmt func(token string) (telegram.Messenger, bool),
) *GuestWifiBridgeSupervisor {
	return &GuestWifiBridgeSupervisor{
		db:      db,
		botRepo: botRepo,
		decider: decider,
		svcFmt:  svcFmt,
		running: map[string]guestBotLoop{},
	}
}

// isRunning reports whether this business already has a tracked loop.
func (g *GuestWifiBridgeSupervisor) isRunning(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.running[id]
	return ok
}

// Bump reconciles running owner loops with the enabled set. Callers drive it on
// their own schedule (typically a goroutine sleeping the scan interval).
func (g *GuestWifiBridgeSupervisor) Bump() {
	list, err := g.botRepo.ListEnabled()
	if err != nil {
		return // transient — retry next cycle
	}
	seen := map[string]bool{}
	for _, bot := range list {
		if bot.ApproverUserID <= 0 || bot.TargetChatID == 0 {
			continue
		}
		seen[bot.BusinessID] = true
		g.mu.Lock()
		old, running := g.running[bot.BusinessID]
		unchanged := running && old.binding.BotToken == bot.BotToken && old.binding.TargetChatID == bot.TargetChatID && old.binding.ApproverUserID == bot.ApproverUserID
		if running && !unchanged {
			old.cancel()
			delete(g.running, bot.BusinessID)
		}
		g.mu.Unlock()
		if unchanged {
			continue
		}
		g.start(bot)
	}
	// Cancel disabled bindings instead of leaving an untracked poller alive.
	g.mu.Lock()
	for id, loop := range g.running {
		if !seen[id] {
			loop.cancel()
			delete(g.running, id)
		}
	}
	g.mu.Unlock()
}

// start builds a messenger from the owner token and runs ONE bridge for that
// business. One Bot instance is both the long-poll source and the ack messenger
// (a single getUpdates stream per owner). The loop runs in the background.
func (g *GuestWifiBridgeSupervisor) start(b *model.BusinessTelegramBot) {
	token := strings.TrimSpace(b.BotToken)
	if token == "" {
		return
	}
	msg, ok := g.svcFmt(token)
	if !ok || msg == nil {
		return // fail closed: no transport => no bridge
	}
	target := "0"
	if b.TargetChatID != 0 {
		target = strconv.FormatInt(b.TargetChatID, 10)
	}
	if bot, down := msg.(*telegram.Bot); down {
		bridge := telegram.NewPurchaseBridge(bot, msg, target, g.decider)
		// Re-read the saved binding for every decision so disable/rebind/user-ID
		// changes take effect before the supervisor's next scheduled scan.
		bridge.SetAuthorization(func(m telegram.InboundMessage, businessID string) bool {
			current, err := g.botRepo.GetByBusiness(b.BusinessID)
			return err == nil && guestApprovalAuthorized(*b, current, m, businessID)
		})
		ctx, cancel := context.WithCancel(context.Background())
		g.mu.Lock()
		g.running[b.BusinessID] = guestBotLoop{binding: *b, cancel: cancel}
		g.mu.Unlock()
		go func() { _ = bridge.Run(ctx) }()
	}
}

func guestApprovalAuthorized(started model.BusinessTelegramBot, current *model.BusinessTelegramBot, m telegram.InboundMessage, businessID string) bool {
	return current != nil && current.Enabled && current.BotToken == started.BotToken &&
		current.BusinessID == started.BusinessID && current.TargetChatID == started.TargetChatID &&
		current.ApproverUserID > 0 && current.ApproverUserID == m.FromID && current.TargetChatID == m.ChatID &&
		(businessID == "" || businessID == current.BusinessID)
}

// Active reports how many owner loops are currently tracked (observability).
func (g *GuestWifiBridgeSupervisor) Active() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.running)
}
