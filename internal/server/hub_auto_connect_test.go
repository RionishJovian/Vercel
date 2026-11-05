package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"mega-magic/internal/database"
)

type fakeWaiting struct {
	routers []database.PendingEnrollmentRef
	err     error
}

func (f fakeWaiting) Waiting(context.Context) ([]database.PendingEnrollmentRef, error) {
	return f.routers, f.err
}

type fakeCompleter struct {
	calls  []string
	result map[string]error
	done   map[string]bool
}

func (f *fakeCompleter) CompletePendingConnection(_ context.Context, biz, id string) (bool, error) {
	f.calls = append(f.calls, biz+"/"+id)
	return f.done[id], f.result[id]
}

func TestAutoConnectChecksEveryWaitingRouter(t *testing.T) {
	store := fakeWaiting{routers: []database.PendingEnrollmentRef{{BusinessID: "b1", DeviceID: "r1"}, {BusinessID: "b2", DeviceID: "r2"}}}
	completer := &fakeCompleter{done: map[string]bool{"r1": true}}
	newHubAutoConnect(store, completer).tick(context.Background())
	if len(completer.calls) != 2 || completer.calls[0] != "b1/r1" || completer.calls[1] != "b2/r2" {
		t.Fatalf("calls = %v", completer.calls)
	}
}

// A router that reached the Hub but failed its login is not retried on every tick,
// and one failing router does not hold up the others.
func TestAutoConnectBacksOffAFailedLogin(t *testing.T) {
	store := fakeWaiting{routers: []database.PendingEnrollmentRef{{BusinessID: "b", DeviceID: "bad"}, {BusinessID: "b", DeviceID: "good"}}}
	completer := &fakeCompleter{result: map[string]error{"bad": errors.New("login rejected")}}
	now := time.Unix(1000, 0)
	a := newHubAutoConnect(store, completer)
	a.now = func() time.Time { return now }
	a.tick(context.Background())
	a.tick(context.Background())
	if want := []string{"b/bad", "b/good", "b/good"}; len(completer.calls) != 3 || completer.calls[2] != want[2] {
		t.Fatalf("failed login retried too soon: %v", completer.calls)
	}
	now = now.Add(hubAutoConnectBackoff + time.Second)
	a.tick(context.Background())
	if completer.calls[3] != "b/bad" {
		t.Fatalf("failed login never retried: %v", completer.calls)
	}
}

// Waiting for a paste is normal and must not trigger the backoff.
func TestAutoConnectKeepsCheckingWhileWaiting(t *testing.T) {
	store := fakeWaiting{routers: []database.PendingEnrollmentRef{{BusinessID: "b", DeviceID: "r"}}}
	completer := &fakeCompleter{}
	a := newHubAutoConnect(store, completer)
	a.tick(context.Background())
	a.tick(context.Background())
	if len(completer.calls) != 2 {
		t.Fatalf("calls = %v", completer.calls)
	}
}

func TestAutoConnectSurvivesAnUnreadableList(t *testing.T) {
	completer := &fakeCompleter{}
	newHubAutoConnect(fakeWaiting{err: errors.New("db down")}, completer).tick(context.Background())
	if len(completer.calls) != 0 {
		t.Fatal("checked routers without a list")
	}
}
