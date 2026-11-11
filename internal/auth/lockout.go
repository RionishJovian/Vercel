package auth

import (
	"strings"
	"sync"
	"time"
)

// MinPasswordLength is the shortest password accepted when one is set or changed. Existing passwords keep working.
const MinPasswordLength = 10

// loginEntryCapacity bounds process memory when an attacker submits endless fake
// emails. Locked accounts are never evicted to make room.
const loginEntryCapacity = 50000

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
	pruned   time.Time // last full prune; a full scan per insert would be quadratic under flood
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
		if len(l.entries) >= loginEntryCapacity && now.Sub(l.pruned) >= time.Minute {
			l.prune(now)
			l.pruned = now
		}
		if len(l.entries) >= loginEntryCapacity && !l.evictOldestUnlocked(now) {
			return // every slot holds an active lock: keep them, track nothing new
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

// evictOldestUnlocked frees one slot in amortized constant time: Go randomizes map
// iteration, so it examines a small sample and drops the unlocked entry whose most
// recent failure is oldest. If the sample holds only locked accounts it falls back
// to a full scan. It reports false when every entry is actively locked.
func (l *LoginLimiter) evictOldestUnlocked(now time.Time) bool {
	pick := func(limit int) (string, bool) {
		var victim string
		var oldest time.Time
		found, seen := false, 0
		for k, e := range l.entries {
			if seen++; limit > 0 && seen > limit {
				break
			}
			if e.lockedUntil.After(now) {
				continue
			}
			last := time.Time{}
			if len(e.fails) > 0 {
				last = e.fails[len(e.fails)-1]
			}
			if !found || last.Before(oldest) {
				victim, oldest, found = k, last, true
			}
		}
		return victim, found
	}
	victim, found := pick(16)
	if !found {
		victim, found = pick(0)
	}
	if found {
		delete(l.entries, victim)
	}
	return found
}
