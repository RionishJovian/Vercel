package server

import (
	"context"
	"log"
	"time"

	"mega-magic/internal/database"
)

// How often to look for routers whose pasted script has reached the Hub, and how long to
// wait before trying a router again after a failed login.
const (
	hubAutoConnectInterval = 15 * time.Second
	hubAutoConnectBackoff  = time.Minute
)

type pendingConnections interface {
	Waiting(context.Context) ([]database.PendingEnrollmentRef, error)
}

type connectionCompleter interface {
	CompletePendingConnection(ctx context.Context, biz, id string) (bool, error)
}

// hubAutoConnect finishes a connection once the router reaches the Hub, so the person
// who pasted the script has nothing more to do and can close the page.
type hubAutoConnect struct {
	store     pendingConnections
	completer connectionCompleter
	now       func() time.Time
	retryAt   map[string]time.Time
}

func newHubAutoConnect(store pendingConnections, completer connectionCompleter) *hubAutoConnect {
	return &hubAutoConnect{store: store, completer: completer, now: time.Now, retryAt: map[string]time.Time{}}
}

func (a *hubAutoConnect) tick(ctx context.Context) {
	waiting, err := a.store.Waiting(ctx)
	if err != nil {
		log.Printf("hub auto-connect: could not read pending routers")
		return
	}
	for _, router := range waiting {
		if ctx.Err() != nil {
			return
		}
		if next, ok := a.retryAt[router.DeviceID]; ok && a.now().Before(next) {
			continue
		}
		connected, err := a.completer.CompletePendingConnection(ctx, router.BusinessID, router.DeviceID)
		switch {
		case err != nil:
			a.retryAt[router.DeviceID] = a.now().Add(hubAutoConnectBackoff)
			log.Printf("hub auto-connect: router %s reached the Hub but login did not verify yet", router.DeviceID)
		case connected:
			delete(a.retryAt, router.DeviceID)
			log.Printf("hub auto-connect: router %s connected", router.DeviceID)
		}
	}
}

func (a *hubAutoConnect) run(ctx context.Context) {
	ticker := time.NewTicker(hubAutoConnectInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.tick(ctx)
		}
	}
}
