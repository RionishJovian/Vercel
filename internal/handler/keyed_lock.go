package handler

import (
	"context"
	"sync"
)

// keyedLock serialises work per key (here: per router) without holding a global
// lock. Waiting honours the caller's context, so a stuck holder cannot pin HTTP
// workers indefinitely. It is in-process: it matches the single API instance the
// service runs as, and the database unique indexes on hub_peer_state remain the
// backstop if that ever changes.
type keyedLock struct {
	mu   sync.Mutex
	held map[string]*keyedEntry
}

type keyedEntry struct {
	slot chan struct{}
	refs int
}

// Lock waits for the key, returning a release function, or the context's error.
func (k *keyedLock) Lock(ctx context.Context, key string) (func(), error) {
	k.mu.Lock()
	if k.held == nil {
		k.held = make(map[string]*keyedEntry)
	}
	e := k.held[key]
	if e == nil {
		e = &keyedEntry{slot: make(chan struct{}, 1)}
		k.held[key] = e
	}
	e.refs++
	k.mu.Unlock()

	release := func() {
		k.mu.Lock()
		defer k.mu.Unlock()
		if e.refs--; e.refs == 0 {
			delete(k.held, key)
		}
	}
	select {
	case e.slot <- struct{}{}:
		return func() {
			<-e.slot
			release()
		}, nil
	case <-ctx.Done():
		release()
		return nil, ctx.Err()
	}
}
