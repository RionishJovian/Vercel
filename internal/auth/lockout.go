package auth

import (
	"strings"
	"sync"
	"time"
)

// MinPasswordLength is the shortest password accepted when one is set or changed. Existing passwords keep working.
const MinPasswordLength = 6

// PasswordRule is the one sentence every screen and API error uses for the password format.
const PasswordRule = "at least 6 characters with an uppercase letter, a lowercase letter and a digit"

// ValidPassword is the single password format check: length 6 to 128 with at least one
// uppercase letter, one lowercase letter and one digit. Existing passwords keep working.
func ValidPassword(p string) bool {
	if len(p) < MinPasswordLength || len(p) > 128 {
		return false
	}
	var up, low, dig bool
	for _, r := range p {
		switch {
		case r >= 'A' && r <= 'Z':
			up = true
		case r >= 'a' && r <= 'z':
			low = true
		case r >= '0' && r <= '9':
			dig = true
		}
	}
	return up && low && dig
}

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
