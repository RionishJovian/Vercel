package lanaccess

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

type dialFunc func(context.Context, Grant) (net.Conn, error)

func (f dialFunc) Dial(ctx context.Context, g Grant) (net.Conn, error) { return f(ctx, g) }

func TestSessionsRevokeClosesConnection(t *testing.T) {
	scope, req, _ := fixture()
	client, remote := net.Pipe()
	defer remote.Close()
	s := NewSessions(dialFunc(func(_ context.Context, g Grant) (net.Conn, error) {
		if g.BusinessID != scope.BusinessID || g.GatewayID != scope.GatewayID || g.Target.String() != req.TargetIP || int(g.Port) != req.Port {
			t.Fatal("wrong tunnel destination")
		}
		return client, nil
	}))
	defer s.Close()
	id, _, err := s.Create(scope, req)
	if err != nil {
		t.Fatal(err)
	}
	foreign := scope
	foreign.UserID = "other"
	if _, err = s.Connect(context.Background(), foreign, id); err == nil {
		t.Fatal("foreign user connected")
	}
	conn, err := s.Connect(context.Background(), scope, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Revoke(foreign, id); err == nil {
		t.Fatal("foreign user revoked")
	}
	cleanupScope := scope
	cleanupScope.Verified = false
	cleanupScope.LANs = nil
	cleanupScope.Protected = nil
	if err = s.Revoke(cleanupScope, id); err != nil {
		t.Fatal(err)
	}
	remote.SetReadDeadline(time.Now().Add(time.Second))
	var b [1]byte
	if _, err = remote.Read(b[:]); !errors.Is(err, io.EOF) {
		t.Fatal("revoked stream open")
	}
	if _, err = s.Connect(context.Background(), scope, id); err == nil {
		t.Fatal("revoked grant reused")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); conn.Close() }()
	}
	wg.Wait()
}

func TestSessionsExpiryAndShutdown(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		scope, req, _ := fixture()
		req.Duration = 40 * time.Millisecond
		client, remote := net.Pipe()
		s := NewSessions(dialFunc(func(context.Context, Grant) (net.Conn, error) { return client, nil }))
		id, _, err := s.Create(scope, req)
		if err != nil {
			t.Fatal(err)
		}
		conn, err := s.Connect(context.Background(), scope, id)
		if err != nil {
			t.Fatal(err)
		}
		if shutdown {
			s.Close()
		}
		remote.SetReadDeadline(time.Now().Add(time.Second))
		var b [1]byte
		if _, err = remote.Read(b[:]); !errors.Is(err, io.EOF) {
			t.Fatal("expired/shutdown stream open")
		}
		if _, err = s.Connect(context.Background(), scope, id); err == nil {
			t.Fatal("expired session reused")
		}
		conn.Close()
		remote.Close()
		s.Close()
	}
}

func TestSessionsRevocationDuringDial(t *testing.T) {
	scope, req, _ := fixture()
	started, finish := make(chan struct{}), make(chan struct{})
	client, remote := net.Pipe()
	defer remote.Close()
	s := NewSessions(dialFunc(func(ctx context.Context, _ Grant) (net.Conn, error) { close(started); <-finish; return client, nil }))
	defer s.Close()
	id, _, err := s.Create(scope, req)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := s.Connect(context.Background(), scope, id); result <- err }()
	<-started
	if err = s.Revoke(scope, id); err != nil {
		t.Fatal(err)
	}
	close(finish)
	if err = <-result; err == nil {
		t.Fatal("late dial returned revoked connection")
	}
	remote.SetReadDeadline(time.Now().Add(time.Second))
	var b [1]byte
	if _, err = remote.Read(b[:]); !errors.Is(err, io.EOF) {
		t.Fatal("late connection not closed")
	}
}

func TestSessionsFailClosedAndBounded(t *testing.T) {
	scope, req, _ := fixture()
	if _, _, err := NewSessions(nil).Create(scope, req); err == nil {
		t.Fatal("missing tunnel transport accepted")
	}
	s := NewSessions(dialFunc(func(context.Context, Grant) (net.Conn, error) { panic("unexpected network") }))
	defer s.Close()
	for i := 0; i < 10; i++ {
		if _, _, err := s.Create(scope, req); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.Create(scope, req); err == nil {
		t.Fatal("unbounded user sessions")
	}
	s.Close()
	if _, _, err := s.Create(scope, req); err == nil {
		t.Fatal("closed service reopened")
	}
}
