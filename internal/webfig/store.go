package webfig

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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
}

type session struct {
	Grant
	expires time.Time
	checked time.Time
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

// Issue creates a one-use ticket.
func (s *Store) Issue(g Grant) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
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
			delete(s.sessions, k)
		}
	}
	s.tickets[sha256.Sum256([]byte(token))] = ticket{Grant: g, expires: now.Add(TicketLifetime)}
	return token, nil
}

// Redeem spends a ticket and returns the new session's cookie value.
func (s *Store) Redeem(token string) (string, time.Time, bool) {
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
	if !ok || !now.Before(t.expires) {
		return "", time.Time{}, false
	}
	expires := now.Add(SessionLifetime)
	s.sessions[sha256.Sum256([]byte(value))] = &session{Grant: t.Grant, expires: expires, checked: now}
	return value, expires, true
}

// Lookup returns a live session and whether it is due for a fresh check.
func (s *Store) Lookup(value string, recheck time.Duration) (Grant, time.Time, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sha256.Sum256([]byte(value))
	v, ok := s.sessions[key]
	if !ok {
		return Grant{}, time.Time{}, false, false
	}
	now := s.now()
	if !now.Before(v.expires) {
		delete(s.sessions, key)
		return Grant{}, time.Time{}, false, false
	}
	due := now.Sub(v.checked) >= recheck
	if due {
		v.checked = now
	}
	return v.Grant, v.expires, due, true
}

// Revoke ends one session.
func (s *Store) Revoke(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sha256.Sum256([]byte(value)))
}
