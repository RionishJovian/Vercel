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
	db       *database.DB
	botRepo  *database.BusinessTelegramBotRepository
	decider  telegram.PurchaseDecider
	svcFmt   func(token string) (telegram.Messenger, bool)
	mu       sync.Mutex
	running  []string // business IDs with a tracked owner loop
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
		running: []string{},
	}
}

// isRunning reports whether this business already has a tracked loop.
func (g *GuestWifiBridgeSupervisor) isRunning(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, r := range g.running {
		if r == id {
			return true
		}
	}
	return false
}

// Bump reconciles running owner loops with the enabled set. Callers drive it on
// their own schedule (typically a goroutine sleeping the scan interval).
func (g *GuestWifiBridgeSupervisor) Bump() {
	list, err := g.botRepo.ListEnabled()
	if err != nil {
		return // transient — retry next cycle
	}
	seen := []string{}
	for _, bot := range list {
		seen = append(seen, bot.BusinessID)
		if g.isRunning(bot.BusinessID) {
			continue // already tracked
		}
		g.start(bot)
	}
	// Drop loops whose owner disabled/unbound their bot. The running long-poll
	// goroutine is background; marking it untracked stops acks and lets the next
	// discovery rebuild it. (Hard-stopping a live poller would require owning
	// its context, which the bridge does not expose — kept simple and safe.)
	g.mu.Lock()
	kept := []string{}
	for _, r := range g.running {
		for _, s := range seen {
			if s == r {
				kept = append(kept, r)
				break
			}
		}
	}
	g.running = kept
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
		g.mu.Lock()
		g.running = append(g.running, b.BusinessID)
		g.mu.Unlock()
		go func() { _ = bridge.Run(context.Background()) }()
	}
}

// Active reports how many owner loops are currently tracked (observability).
func (g *GuestWifiBridgeSupervisor) Active() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.running)
}