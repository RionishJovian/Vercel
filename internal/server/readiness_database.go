package server

import (
	"context"
	"sync"
	"time"
)

// lib/pq cancellation can itself wait on a stalled server. Bound the HTTP probe
// independently, sharing one in-flight Ping across polls so a driver that never
// returns cannot accumulate goroutines/connections on every readiness request.
func boundedDatabaseProbe(ping func(context.Context) error) func() error {
	type result struct {
		ctx  context.Context
		done chan struct{}
		err  error
	}
	var mu sync.Mutex
	var active *result
	return func() error {
		mu.Lock()
		if active == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			p := &result{ctx: ctx, done: make(chan struct{})}
			active = p
			go func() {
				p.err = ping(ctx)
				if ctx.Err() != nil {
					p.err = ctx.Err()
				}
				close(p.done)
				mu.Lock()
				active = nil
				mu.Unlock()
				cancel()
			}()
		}
		p := active
		mu.Unlock()
		select {
		case <-p.done:
			return p.err
		case <-p.ctx.Done():
			// A completed healthy probe can cancel its context to release the timer.
			select {
			case <-p.done:
				return p.err
			default:
				return p.ctx.Err()
			}
		}
	}
}
