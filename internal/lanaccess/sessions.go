package lanaccess

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"sync"
	"time"
)

// Transport must route through this grant's verified gateway, not dial Target
// from the app host's general network. A nil transport always denies access.
type Transport interface {
	Dial(context.Context, Grant) (net.Conn, error)
}

type session struct {
	grant       Grant
	connections map[*sessionConn]struct{}
	timer       *time.Timer
	ctx         context.Context
	cancel      context.CancelFunc
	pending     int
}

// Sessions are intentionally ephemeral: restart invalidates all IDs and closes
// process-owned streams. They cannot re-open an old grant after a process crash.
// Scope is freshly server-resolved for each operation, never guest request JSON.
type Sessions struct {
	mu        sync.Mutex
	entries   map[string]*session
	transport Transport
	closed    bool
}

func NewSessions(transport Transport) *Sessions {
	return &Sessions{entries: make(map[string]*session), transport: transport}
}

func (s *Sessions) Create(scope Scope, request Request) (string, Grant, error) {
	if s.transport == nil {
		return "", Grant{}, ErrDenied
	}
	g, err := Create(scope, request, time.Now())
	if err != nil {
		return "", Grant{}, err
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return "", Grant{}, ErrDenied
	}
	id := hex.EncodeToString(nonce[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || len(s.entries) >= 1000 {
		return "", Grant{}, ErrDenied
	}
	count := 0
	for _, entry := range s.entries {
		if entry.grant.BusinessID == scope.BusinessID && entry.grant.UserID == scope.UserID {
			count++
		}
	}
	if count >= 10 {
		return "", Grant{}, ErrDenied
	}
	entry := &session{grant: g, connections: make(map[*sessionConn]struct{})}
	entry.ctx, entry.cancel = context.WithDeadline(context.Background(), g.ExpiresAt)
	s.entries[id] = entry
	entry.timer = time.AfterFunc(time.Until(g.ExpiresAt), func() { s.closeSession(id) })
	return id, g, nil
}

func (s *Sessions) Connect(ctx context.Context, scope Scope, id string) (net.Conn, error) {
	s.mu.Lock()
	entry := s.entries[id]
	if s.closed || entry == nil || entry.grant.Authorize(scope, entry.grant.Target, entry.grant.Port, time.Now()) != nil || len(entry.connections)+entry.pending >= 8 {
		s.mu.Unlock()
		return nil, ErrDenied
	}
	entry.pending++
	g := entry.grant
	s.mu.Unlock()
	defer func() { s.mu.Lock(); entry.pending--; s.mu.Unlock() }()
	work, cancel := context.WithDeadline(ctx, g.ExpiresAt)
	defer cancel()
	stop := context.AfterFunc(entry.ctx, cancel)
	defer stop()
	conn, err := s.transport.Dial(work, g)
	if err != nil || conn == nil {
		if conn != nil {
			conn.Close()
		}
		return nil, ErrDenied
	}
	s.mu.Lock()
	// Revocation/expiry can race the dial. Recheck the same live entry before
	// handing a connection to the caller; a completed dial is not authority.
	if ctx.Err() != nil || s.closed || s.entries[id] != entry || g.Authorize(scope, g.Target, g.Port, time.Now()) != nil || len(entry.connections) >= 8 {
		s.mu.Unlock()
		conn.Close()
		return nil, ErrDenied
	}
	wrapped := &sessionConn{Conn: conn, release: func(c *sessionConn) { s.mu.Lock(); delete(entry.connections, c); s.mu.Unlock() }}
	entry.connections[wrapped] = struct{}{}
	s.mu.Unlock()
	return wrapped, nil
}

func (s *Sessions) Revoke(scope Scope, id string) error {
	s.mu.Lock()
	entry := s.entries[id]
	// Cleanup must remain possible after inventory/plan/reachability changes.
	// Revocation grants no connectivity: require only the current authenticated
	// managing role and exact owner identity, not an otherwise-live LAN scope.
	if entry == nil || !canManage(scope.Role) || scope.BusinessID == "" || scope.UserID == "" ||
		entry.grant.BusinessID != scope.BusinessID || entry.grant.GatewayID != scope.GatewayID || entry.grant.UserID != scope.UserID {
		s.mu.Unlock()
		return ErrDenied
	}
	// Remove authority before closing sockets. New connects fail immediately.
	delete(s.entries, id)
	entry.timer.Stop()
	entry.cancel()
	connections := snapshotConnections(entry)
	s.mu.Unlock()
	closeConnections(connections)
	return nil
}

func (s *Sessions) closeSession(id string) {
	s.mu.Lock()
	entry := s.entries[id]
	delete(s.entries, id)
	var connections []*sessionConn
	if entry != nil {
		entry.timer.Stop()
		entry.cancel()
		connections = snapshotConnections(entry)
	}
	s.mu.Unlock()
	closeConnections(connections)
}

func snapshotConnections(entry *session) []*sessionConn {
	var connections []*sessionConn
	for conn := range entry.connections {
		connections = append(connections, conn)
	}
	return connections
}
func closeConnections(connections []*sessionConn) {
	for _, conn := range connections {
		conn.Close()
	}
}

func (s *Sessions) Close() {
	s.mu.Lock()
	s.closed = true
	entries := s.entries
	s.entries = make(map[string]*session)
	var connections []*sessionConn
	for _, entry := range entries {
		entry.timer.Stop()
		entry.cancel()
		connections = append(connections, snapshotConnections(entry)...)
	}
	s.mu.Unlock()
	closeConnections(connections)
}

type sessionConn struct {
	net.Conn
	once    sync.Once
	release func(*sessionConn)
	err     error
}

func (c *sessionConn) Close() error {
	c.once.Do(func() { c.err = c.Conn.Close(); c.release(c) })
	return c.err
}
