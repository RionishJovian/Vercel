package auth

import (
	"strings"
	"sync"
	"time"
)

// MinPasswordLength is the shortest password accepted when one is set or changed. Existing passwords keep working.
const MinPasswordLength = 10

// LoginLimiter slows password guessing against one account, on top of the per-address limit. It is process-local,
// which matches the single API instance. After maxFails failures inside window the email is locked for lockFor; a
// successful sign-in clears it. The same applies to emails that do not exist, so a lock never reveals whether an
// account is real.
type LoginLimiter struct {
	mu       sync.Mutex
	maxFails int
	window   time.Duration
	lockFor  time.Duration
	now      func() time.Time
	entries  map[string]*loginEntry
}

type loginEntry struct {
	fails       []time.Time
	lockedUntil time.Time
}

func NewLoginLimiter(maxFails int, window, lockFor time.Duration) *LoginLimiter {
	return &LoginLimiter{maxFails: maxFails, window: window, lockFor: lockFor, now: time.Now, entries: map[string]*loginEntry{}}
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// Locked reports whether the account is locked and for how long.
func (l *LoginLimiter) Locked(email string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[normalizeEmail(email)]
	if e == nil {
		return false, 0
	}
	if left := e.lockedUntil.Sub(l.now()); left > 0 {
		return true, left
	}
	return false, 0
}

// Fail records a failed attempt and locks the account when the limit is reached.
func (l *LoginLimiter) Fail(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key, now := normalizeEmail(email), l.now()
	e := l.entries[key]
	if e == nil {
		if len(l.entries) >= 50000 {
			l.prune(now)
		}
		e = &loginEntry{}
		l.entries[key] = e
	}
	kept := e.fails[:0]
	for _, t := range e.fails {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	e.fails = append(kept, now)
	if len(e.fails) >= l.maxFails {
		e.lockedUntil = now.Add(l.lockFor)
		e.fails = nil
	}
}

// Succeed clears the record after a correct password.
func (l *LoginLimiter) Succeed(email string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, normalizeEmail(email))
}

func (l *LoginLimiter) prune(now time.Time) {
	for k, e := range l.entries {
		if !e.lockedUntil.After(now) && (len(e.fails) == 0 || now.Sub(e.fails[len(e.fails)-1]) >= l.window) {
			delete(l.entries, k)
		}
	}
}
