package fleet

import (
	"context"
	"errors"
	"testing"
	"time"

	"mega-magic/internal/database"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
)

type fakeMonitor struct {
	state string
	err   error
	read  time.Time
}

func (f fakeMonitor) StateForPeer(context.Context, string, string) (string, error) {
	return f.state, f.err
}
func (f fakeMonitor) LastRouterRead(context.Context, string, string) (time.Time, bool) {
	return f.read, !f.read.IsZero()
}

type fakeHub func(hub.Identity) (hub.Peer, error)

func (f fakeHub) InspectRouter(_ context.Context, id hub.Identity) (hub.Peer, error) { return f(id) }

func oneSummary(t *testing.T, s *FleetService, layer string) PeerSummary {
	t.Helper()
	peers := s.summaries(context.Background(), "biz", "user", []*model.Device{{DeviceID: "d1", HardwareName: "Shop", DeploymentLayer: layer}})
	if len(peers) != 1 || peers[0].DeviceID != "d1" || peers[0].HardwareName != "Shop" {
		t.Fatalf("peers %+v", peers)
	}
	return peers[0]
}

func TestStatusSource(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	recent, old := now.Add(-time.Minute), now.Add(-time.Hour)
	active := func(at *time.Time) fakeHub {
		return func(id hub.Identity) (hub.Peer, error) {
			return hub.Peer{TenantID: id.TenantID, RouterID: id.RouterID, State: "active", LastHandshakeAt: at}, nil
		}
	}
	notFound := fakeHub(func(hub.Identity) (hub.Peer, error) { return hub.Peer{}, hub.ErrPeerNotFound })
	foreign := fakeHub(func(hub.Identity) (hub.Peer, error) {
		return hub.Peer{TenantID: "other", RouterID: "d1", State: "active", LastHandshakeAt: &recent}, nil
	})
	removed := fakeHub(func(id hub.Identity) (hub.Peer, error) {
		return hub.Peer{TenantID: id.TenantID, RouterID: id.RouterID, State: "removed", LastHandshakeAt: &recent}, nil
	})
	for _, tc := range []struct {
		name    string
		layer   string
		hub     hubInspector
		monitor fakeMonitor
		state   string
		source  string
		contact *time.Time
		checked bool
	}{
		{"recent handshake is up", "MAIN_GATEWAY", active(&recent), fakeMonitor{state: "unknown"}, StateUp, SourceHubTunnel, &recent, true},
		{"old handshake is offline", "MAIN_GATEWAY", active(&old), fakeMonitor{state: "up"}, StateOffline, SourceHubTunnel, &old, true},
		{"never connected is offline", "MAIN_GATEWAY", active(nil), fakeMonitor{}, StateOffline, SourceHubTunnel, nil, true},
		{"a removed peer is offline", "MAIN_GATEWAY", removed, fakeMonitor{}, StateOffline, SourceHubTunnel, &recent, true},
		{"no peer uses the router check", "MAIN_GATEWAY", notFound, fakeMonitor{state: "up"}, StateUp, SourceRouterCheck, nil, false},
		{"someone else's peer is ignored", "MAIN_GATEWAY", foreign, fakeMonitor{state: "unknown"}, StateUnknown, SourceNone, nil, false},
		{"a recent read counts when the check knows nothing", "MAIN_GATEWAY", nil, fakeMonitor{state: "unknown", read: recent}, StateUp, SourceRouterRead, &recent, false},
		{"an old read does not", "MAIN_GATEWAY", nil, fakeMonitor{state: "unknown", read: old}, StateUnknown, SourceNone, &old, false},
		{"a read does not overrule a known bad state", "MAIN_GATEWAY", nil, fakeMonitor{state: "offline", read: recent}, StateOffline, SourceRouterCheck, &recent, false},
		{"a failed check is unknown", "MAIN_GATEWAY", nil, fakeMonitor{err: errors.New("down")}, StateUnknown, SourceNone, nil, false},
		{"only gateways ask the Hub", "ACCESS_POINT", active(&recent), fakeMonitor{state: "unknown"}, StateUnknown, SourceNone, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &FleetService{monitor: tc.monitor, hub: tc.hub, now: func() time.Time { return now }}
			got := oneSummary(t, s, tc.layer)
			if got.State != tc.state || got.Source != tc.source {
				t.Fatalf("got %s from %s, want %s from %s", got.State, got.Source, tc.state, tc.source)
			}
			if (tc.contact == nil) != (got.LastContactAt == nil) || (tc.contact != nil && !tc.contact.Equal(*got.LastContactAt)) {
				t.Fatalf("last contact %v, want %v", got.LastContactAt, tc.contact)
			}
			if (got.CheckedAt != nil) != tc.checked {
				t.Fatalf("checked at %v, want set %v", got.CheckedAt, tc.checked)
			}
		})
	}
}

func TestStatusNamesTheAccountAndRequester(t *testing.T) {
	var seen hub.Identity
	s := &FleetService{monitor: fakeMonitor{}, now: time.Now, hub: fakeHub(func(id hub.Identity) (hub.Peer, error) {
		seen = id
		return hub.Peer{}, hub.ErrPeerNotFound
	})}
	oneSummary(t, s, "MAIN_GATEWAY")
	if seen != (hub.Identity{TenantID: "biz", RouterID: "d1", RequestedBy: "user"}) {
		t.Fatalf("identity %+v", seen)
	}
}

type fakeHistory struct {
	gateways []*model.Device
	latest   map[string]database.ConnectionChange
	recorded []string
	err      error
}

func (f *fakeHistory) ListGatewaysAcrossTenants() ([]*model.Device, error) { return f.gateways, nil }
func (f *fakeHistory) LatestConnectionChanges(string) (map[string]database.ConnectionChange, error) {
	return f.latest, f.err
}
func (f *fakeHistory) RecordConnectionChange(business, device, action, reason string) error {
	f.recorded = append(f.recorded, business+"/"+device+"/"+action+"/"+reason)
	f.latest[device] = database.ConnectionChange{Action: action}
	return nil
}

func TestConnectionHistoryRecordsOnlyChanges(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	handshake := now.Add(-time.Minute)
	var peerErr error
	inspect := fakeHub(func(id hub.Identity) (hub.Peer, error) {
		if id.RequestedBy != connectionHistoryRequester {
			t.Errorf("requester %q", id.RequestedBy)
		}
		return hub.Peer{TenantID: id.TenantID, RouterID: id.RouterID, State: "active", LastHandshakeAt: &handshake}, peerErr
	})
	history := &fakeHistory{gateways: []*model.Device{{BusinessID: "biz", DeviceID: "d1", DeploymentLayer: "MAIN_GATEWAY"}}, latest: map[string]database.ConnectionChange{}}
	s := &FleetService{monitor: fakeMonitor{}, hub: inspect, history: history, now: func() time.Time { return now }}
	pass := func() {
		t.Helper()
		if err := s.RecordConnectionChanges(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	pass()
	pass()
	if len(history.recorded) != 1 || history.recorded[0] != "biz/d1/router.connection.up/Connected to Magic Hub." {
		t.Fatalf("first passes recorded %v", history.recorded)
	}

	now = now.Add(time.Hour)
	pass()
	pass()
	want := "biz/d1/router.connection.offline/No Magic Hub connection. Last contact " + handshake.Format(time.RFC3339) + "."
	if len(history.recorded) != 2 || history.recorded[1] != want {
		t.Fatalf("drop recorded %v", history.recorded)
	}

	// A lookup failure says nothing about the router.
	handshake = now
	peerErr = errors.New("hub down")
	pass()
	if len(history.recorded) != 2 {
		t.Fatalf("a failed lookup recorded %v", history.recorded)
	}
	peerErr = nil
	pass()
	if len(history.recorded) != 3 {
		t.Fatalf("reconnect recorded %v", history.recorded)
	}
}

func TestConnectionHistoryOffWithoutHub(t *testing.T) {
	history := &fakeHistory{gateways: []*model.Device{{BusinessID: "biz", DeviceID: "d1"}}, latest: map[string]database.ConnectionChange{}}
	s := &FleetService{monitor: fakeMonitor{}, history: history, now: time.Now}
	if err := s.RecordConnectionChanges(context.Background()); err != nil || len(history.recorded) != 0 {
		t.Fatalf("err %v, recorded %v", err, history.recorded)
	}
}

func TestStateSince(t *testing.T) {
	at := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	history := &fakeHistory{latest: map[string]database.ConnectionChange{
		"same":  {Action: database.ActionConnectionUp, At: at},
		"stale": {Action: database.ActionConnectionOffline, At: at},
		"local": {Action: database.ActionConnectionUp, At: at},
	}}
	s := &FleetService{history: history}
	peers := s.withStateSince("biz", []PeerSummary{
		{DeviceID: "same", State: StateUp, Source: SourceHubTunnel},
		{DeviceID: "stale", State: StateUp, Source: SourceHubTunnel},
		{DeviceID: "local", State: StateUp, Source: SourceRouterRead},
		{DeviceID: "none", State: StateUp, Source: SourceHubTunnel},
	})
	if peers[0].StateSince == nil || !peers[0].StateSince.Equal(at) {
		t.Fatalf("matching state: %v", peers[0].StateSince)
	}
	for _, p := range peers[1:] {
		if p.StateSince != nil {
			t.Fatalf("%s should have no start time: %v", p.DeviceID, p.StateSince)
		}
	}
	history.err = errors.New("db down")
	if got := s.withStateSince("biz", []PeerSummary{{DeviceID: "same", State: StateUp, Source: SourceHubTunnel}}); len(got) != 1 || got[0].StateSince != nil {
		t.Fatalf("history failure: %+v", got)
	}
}
