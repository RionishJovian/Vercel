package webfig

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"strings"
	"sync"
	"time"
)

const (
	// TicketLifetime covers the hand-off from the console to the new tab.
	TicketLifetime = time.Minute
	// SessionLifetime is a hard limit. WebFig polls constantly, so an open tab is
	// never idle and an idle limit would mean nothing.
	SessionLifetime = time.Hour
)

// Grant is who may reach which router, fixed when the console asks to open it.
type Grant struct {
	UserID, BusinessID, DeviceID, Role, AuthSessionID string
	// Scheme, Host and Port are the router's tunnel address, resolved from the
	// tenant's own device record. Nothing from the browser reaches them.
	Scheme, Host string
	Port         int
	// Label is this session's own subdomain. Each session is its own web origin,
	// so what one router serves cannot touch another session in the same browser.
	Label string
}

type session struct {
	Grant
	expires time.Time
	checked time.Time
	done    chan struct{}
}

// Live is a session that may still be used.
type Live struct {
	Grant
	Expires time.Time
	// Due says this request should run the fresh check.
	Due bool
	// Done closes when the session is revoked, which ends requests still held open.
	Done <-chan struct{}
}

// Store keeps tickets and sessions in memory, hashed. A restart of the API ends
// every session and the user opens WebFig again, which is the safe direction.
type Store struct {
	mu       sync.Mutex
	tickets  map[[32]byte]ticket
	sessions map[[32]byte]*session
	now      func() time.Time
}

type ticket struct {
	Grant
	expires time.Time
}

func NewStore() *Store {
	return &Store{tickets: map[[32]byte]ticket{}, sessions: map[[32]byte]*session{}, now: time.Now}
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// newLabel is a DNS label: lowercase, and random enough that it can't be guessed.
func newLabel() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)), nil
}

// Issue creates a one-use ticket and the subdomain it works on.
func (s *Store) Issue(g Grant) (string, string, error) {
	token, err := newToken()
	if err != nil {
		return "", "", err
	}
	label, err := newLabel()
	if err != nil {
		return "", "", err
	}
	g.Label = label
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, t := range s.tickets {
		if !now.Before(t.expires) {
			delete(s.tickets, k)
		}
	}
	for k, v := range s.sessions {
		if !now.Before(v.expires) {
			close(v.done)
			delete(s.sessions, k)
		}
	}
	s.tickets[sha256.Sum256([]byte(token))] = ticket{Grant: g, expires: now.Add(TicketLifetime)}
	return token, label, nil
}

// Redeem spends a ticket, only on its own subdomain, and returns the new session's
// cookie value. A ticket tried anywhere else is spent too.
func (s *Store) Redeem(token, label string) (string, time.Time, bool) {
	value, err := newToken()
	if err != nil {
		return "", time.Time{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sha256.Sum256([]byte(token))
	t, ok := s.tickets[key]
	delete(s.tickets, key)
	now := s.now()
	if !ok || !now.Before(t.expires) || t.Label != label {
		return "", time.Time{}, false
	}
	expires := now.Add(SessionLifetime)
	s.sessions[sha256.Sum256([]byte(value))] = &session{Grant: t.Grant, expires: expires, checked: now, done: make(chan struct{})}
	return value, expires, true
}

// Lookup returns a live session on its own subdomain and whether it is due for a
// fresh check. A session's cookie is never accepted on another subdomain.
func (s *Store) Lookup(value, label string, recheck time.Duration) (Live, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sha256.Sum256([]byte(value))
	v, ok := s.sessions[key]
	if !ok || v.Label != label {
		return Live{}, false
	}
	now := s.now()
	if !now.Before(v.expires) {
		close(v.done)
		delete(s.sessions, key)
		return Live{}, false
	}
	due := now.Sub(v.checked) >= recheck
	if due {
		v.checked = now
	}
	return Live{Grant: v.Grant, Expires: v.expires, Due: due, Done: v.done}, true
}

// Revoke ends one session.
func (s *Store) Revoke(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sha256.Sum256([]byte(value))
	if v, ok := s.sessions[key]; ok {
		close(v.done)
		delete(s.sessions, key)
	}
}
