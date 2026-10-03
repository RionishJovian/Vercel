// Package access enforces timed playback on the owner's media computer.
// Expiry is enforced during an open stream, not only when a player first loads.
package access

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrDenied = errors.New("media access ended")

// Check must validate the same tenant, pass and guest identity against the cloud
// ledger. No successful cached authorization may extend the absolute deadline.
// A failed check (including cloud outage) closes the stream, never fails open.
type Check func(context.Context) error
type Open func(context.Context) (io.ReadCloser, error)

// Copy streams from a fixed authorized local source. Open must honor ctx and
// Close must unblock reads (as net/http response bodies do). It never accepts a
// browser-supplied URL or exposes the local Jellyfin credential to the browser.
func Copy(ctx context.Context, destination io.Writer, expires time.Time, check Check, open Open) (int64, error) {
	return copyChecked(ctx, destination, expires, 5*time.Second, check, open)
}

func copyChecked(ctx context.Context, destination io.Writer, expires time.Time, interval time.Duration, check Check, open Open) (int64, error) {
	now := time.Now()
	if destination == nil || check == nil || open == nil || !expires.After(now) || expires.After(now.Add(30*24*time.Hour)) || interval <= 0 || interval > 5*time.Second {
		return 0, ErrDenied
	}
	deadline, cancel := context.WithDeadline(ctx, expires)
	defer cancel()
	checkCtx, checkCancel := context.WithTimeout(deadline, 3*time.Second)
	err := check(checkCtx)
	checkCancel()
	if err != nil || deadline.Err() != nil {
		return 0, ErrDenied
	}
	source, err := open(deadline)
	if err != nil {
		return 0, ErrDenied
	}
	if source == nil {
		return 0, ErrDenied
	}
	defer source.Close()
	// A blocked read must be interrupted at expiry even if no further bytes arrive.
	stop := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-deadline.Done():
				source.Close()
				return
			case <-ticker.C:
				probe, done := context.WithTimeout(deadline, 3*time.Second)
				e := check(probe)
				done()
				if e != nil {
					cancel()
					source.Close()
					return
				}
			}
		}
	}()
	n, err := io.Copy(destination, source)
	close(stop)
	<-finished
	if deadline.Err() != nil || !time.Now().Before(expires) {
		return n, ErrDenied
	}
	return n, err
}
