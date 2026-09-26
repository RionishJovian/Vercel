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
	"mega-magic/internal/security"
)

type MonitoringService struct {
	heartbeats *database.PeerHeartbeatRepository
	telemetry  *database.TelemetrySampleRepository
	cache      *cache.Client
	msg        *messaging.Client
	probe      *probe.ProbeService

	// Optional security-posture capture (C1): when wired, each peer evaluation
	// best-effort reads the RouterOS version and records its posture. Off by
	// default (nil) — nothing changes until wired.
	versionObserver VersionObserver
	devices         *database.DeviceRepository
	db              *database.DB
}

// VersionObserver reads a peer's RouterOS version + channel (satisfied by
// *routeros.Prober). Implemented as a narrow seam so monitoring stays decoupled.
type VersionObserver interface {
	ObserveVersion(ctx context.Context, peerID string) (version, channel string, err error)
}

// WithVersionCapture enables best-effort RouterOS version capture on the
// peer-evaluation path. Nil-safe: unwired, evaluation is unchanged.
func (s *MonitoringService) WithVersionCapture(db *database.DB, devices *database.DeviceRepository, obs VersionObserver) {
	s.db = db
	s.devices = devices
	s.versionObserver = obs
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

	// Best-effort security-posture capture (C1). Never affects the state result.
	s.captureVersion(ctx, businessID, peerID)

	return state, nil
}

// captureVersion best-effort records a device's RouterOS version + posture,
// tenant-scoped. Fail-soft: any failure (unreachable device, nil deps) is
// ignored and never fabricates a value.
func (s *MonitoringService) captureVersion(ctx context.Context, businessID, peerID string) {
	if s.versionObserver == nil || s.devices == nil || s.db == nil {
		return
	}
	version, channel, err := s.versionObserver.ObserveVersion(ctx, peerID)
	if err != nil || version == "" {
		return
	}
	posture := security.ClassifyPosture(version, channel)
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	if err := database.WithTenant(tx, businessID, func() error {
		return s.devices.SavePostureTXN(tx, businessID, peerID, version, channel, posture, time.Now().UTC())
	}); err != nil {
		return
	}
	_ = tx.Commit()
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
