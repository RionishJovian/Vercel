package monitoring

import (
	"context"
	"encoding/json"
	"time"

	"mega-magic/internal/cache"
	"mega-magic/internal/database"
	"mega-magic/internal/messaging"
	"mega-magic/internal/model"
	"mega-magic/internal/probe"
)

type MonitoringService struct {
	heartbeats *database.PeerHeartbeatRepository
	telemetry  *database.TelemetrySampleRepository
	cache      *cache.Client
	msg        *messaging.Client
	probe      *probe.ProbeService
}

func NewMonitoringService(heartbeats *database.PeerHeartbeatRepository, telemetry *database.TelemetrySampleRepository, c *cache.Client, msg *messaging.Client, p *probe.ProbeService) *MonitoringService {
	return &MonitoringService{
		heartbeats: heartbeats,
		telemetry:  telemetry,
		cache:      c,
		msg:        msg,
		probe:      p,
	}
}

func (s *MonitoringService) EvaluatePeer(ctx context.Context, businessID, peerID string) (string, error) {
	hb, _ := s.heartbeats.GetLatestByPeer(businessID, peerID)

	since := time.Now().Add(-1 * time.Hour)
	samples, _ := s.telemetry.ListByDevice(peerID, &since, nil)
	var sample *model.TelemetrySample
	if len(samples) > 0 {
		sample = samples[0]
	}

	state := s.probe.ClassifyPeerState(hb, sample)

	key := s.cache.Key(businessID, "monitor:state:"+peerID)
	if err := s.cache.SetJSON(ctx, key, state, 30*time.Second); err != nil {
		return state, nil
	}

	if s.msg != nil {
		payload := map[string]string{
			"business_id": businessID,
			"peer_id":     peerID,
			"state":       state,
			"observed_at": time.Now().UTC().Format(time.RFC3339),
		}
		if data, err := json.Marshal(payload); err == nil {
			_ = s.msg.Publish(businessID, "monitor.peer.state", data)
		}
	}

	return state, nil
}

func (s *MonitoringService) StateForPeer(ctx context.Context, businessID, peerID string) (string, error) {
	var out string
	key := s.cache.Key(businessID, "monitor:state:"+peerID)
	if err := s.cache.GetJSON(ctx, key, &out); err == nil && out != "" {
		return out, nil
	}
	return s.EvaluatePeer(ctx, businessID, peerID)
}

func (s *MonitoringService) ListPeerStates(ctx context.Context, businessID string) (map[string]string, error) {
	return map[string]string{}, nil
}