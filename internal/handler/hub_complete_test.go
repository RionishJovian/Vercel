package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"mega-magic/internal/hub"
)

type stubLiveHub struct {
	peer hub.Peer
	err  error
	who  string
}

func (s *stubLiveHub) InspectRouter(_ context.Context, id hub.Identity) (hub.Peer, error) {
	s.who = id.RequestedBy
	return s.peer, s.err
}
func (s *stubLiveHub) InspectPeer(_ context.Context, _ hub.Identity, _ string) (hub.Peer, error) {
	return s.peer, s.err
}

func TestCompletePendingConnection(t *testing.T) {
	recent := time.Now().Add(-time.Minute)
	old := time.Now().Add(-time.Hour)
	active := func(at *time.Time) hub.Peer {
		return hub.Peer{TenantID: "b", RouterID: "r", State: "active", TunnelAddress: "10.77.0.20/32", RouterPublicKey: "key", LastHandshakeAt: at}
	}
	for name, tc := range map[string]struct {
		peer      hub.Peer
		plan      bool
		verifyErr error
		connected bool
		verifies  int
		wantErr   bool
	}{
		"not pasted yet":         {peer: active(nil), plan: true},
		"router went quiet":      {peer: active(&old), plan: true},
		"router reached the Hub": {peer: active(&recent), plan: true, connected: true, verifies: 1},
		"login did not verify":   {peer: active(&recent), plan: true, verifyErr: errors.New("rejected"), verifies: 1, wantErr: true},
		"pass ended: no attempt": {peer: active(&recent), plan: false},
	} {
		t.Run(name, func(t *testing.T) {
			live := &stubLiveHub{peer: tc.peer}
			verifies := 0
			h := &VPSHandler{liveHub: live, livePlan: func(string) bool { return tc.plan }, automaticVerify: func(_ context.Context, biz, id, address, key string) error {
				verifies++
				if biz != "b" || id != "r" || address != "10.77.0.20" || key != "key" {
					t.Errorf("verified the wrong target: %s %s %s %s", biz, id, address, key)
				}
				return tc.verifyErr
			}}
			connected, err := h.CompletePendingConnection(context.Background(), "b", "r")
			if connected != tc.connected || verifies != tc.verifies || (err != nil) != tc.wantErr {
				t.Fatalf("connected=%v verifies=%d err=%v", connected, verifies, err)
			}
			if tc.plan && live.who == "" {
				t.Error("Hub request had no requester")
			}
		})
	}
}
