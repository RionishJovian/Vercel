package probe

import (
	"context"
	"time"

	"mega-magic/internal/cache"
	"mega-magic/internal/model"
)

type ProbeService struct {
	cache   *cache.Client
	tDown   time.Duration
}

func NewProbeService(cacheClient *cache.Client, tDown time.Duration) *ProbeService {
	return &ProbeService{cache: cacheClient, tDown: tDown}
}

// classifyFreshness is the pure peer-state decision table, extracted from
// ClassifyPeerState so it can be tested deterministically (no wall clock).
// - up:      handshake fresh AND data plane up
// - stale:   handshake expired BUT data plane still up (edge in reachability)
// - offline: handshake expired AND data plane down
// - unknown: any other combination
func classifyFreshness(fresh, dataPlaneUp bool) string {
	if fresh && dataPlaneUp {
		return "up"
	}
	if !fresh && dataPlaneUp {
		return "stale"
	}
	if !fresh && !dataPlaneUp {
		return "offline"
	}
	return "unknown"
}

func (s *ProbeService) ClassifyPeerState(latestHeartbeat *model.PeerHeartbeat, latestSample *model.TelemetrySample) string {
	if latestHeartbeat == nil && latestSample == nil {
		return "unknown"
	}

	handshakeAge := time.Duration(0)
	if latestHeartbeat != nil && latestHeartbeat.HandshakeAt != nil {
		now := time.Now()
		hs := *latestHeartbeat.HandshakeAt
		handshakeAge = now.Sub(hs)
	}

	fresh := handshakeAge <= s.tDown

	dataPlaneUp := false
	if latestSample != nil {
		if latestSample.TunnelState != nil && latestSample.WANState != nil {
			dataPlaneUp = *latestSample.TunnelState == "up" && *latestSample.WANState == "up"
		}
	}

	return classifyFreshness(fresh, dataPlaneUp)
}

func (s *ProbeService) GetPeerReachability(ctx context.Context, businessID, peerID string, latestHeartbeat *model.PeerHeartbeat, latestSample *model.TelemetrySample) (string, error) {
	key := "peerstate:" + businessID + ":" + peerID

	if cached, err := s.cache.Get(ctx, key); err == nil && cached != "" {
		return cached, nil
	}

	state := s.ClassifyPeerState(latestHeartbeat, latestSample)

	if err := s.cache.Set(ctx, key, state, 30*time.Second); err != nil {
		return state, nil
	}

	return state, nil
}