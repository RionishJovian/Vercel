package fleet

import (
	"context"
	"errors"
	"testing"
	"time"

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
