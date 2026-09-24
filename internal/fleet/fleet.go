package fleet

import (
	"context"
	"fmt"

	"mega-magic/internal/database"
	"mega-magic/internal/monitoring"
)

// Peer states the fleet understands. These mirror the probe classifier output
// so the aggregate can be built from Evaluate/StateForPeer results directly.
const (
	StateUp      = "up"
	StateStale   = "stale"
	StateOffline = "offline"
	StateUnknown = "unknown"
)

// FleetHealth is the per-tenant, read-only picture of every managed peer.
type FleetHealth struct {
	BusinessID string        `json:"business_id"`
	Total      int           `json:"total_peers"`
	Up         int           `json:"up"`
	Stale      int           `json:"stale"`
	Offline    int           `json:"offline"`
	Unknown    int           `json:"unknown"`
	Overall    string        `json:"overall_state"`
	Peers      []PeerSummary `json:"peers"`
}

// PeerSummary is one router's fleet row: identity + current reachability state.
type PeerSummary struct {
	DeviceID     string `json:"device_id"`
	HardwareName string `json:"hardware_name"`
	State        string `json:"state"`
}

// AggregateHealth is a pure function: it folds a slice of peer states into a
// FleetHealth summary (counts + overall state) with no I/O. Keeping the state
// math here makes it directly unit-testable without a DB.
//
// overall_state is the worst of the reported states, best-first:
//   unknown (no data yet) < up < stale (data, connectivity slipping)
//   < offline (hard down / unreachable).
func AggregateHealth(businessID string, peers []PeerSummary) FleetHealth {
	summary := FleetHealth{
		BusinessID: businessID,
		Total:      len(peers),
		Peers:      peers,
	}
	overall := StateUnknown
	for _, p := range peers {
		switch p.State {
		case StateUp:
			summary.Up++
		case StateStale:
			summary.Stale++
		case StateOffline:
			summary.Offline++
		default:
			summary.Unknown++
		}
		overall = worst(overall, p.State)
	}
	summary.Overall = overall
	return summary
}

// worst returns the more severe of two peer states using the severity ladder
// unknown(0) < up(1) < stale(2) < offline(3).
func worst(a, b string) string {
	if severity(b) > severity(a) {
		return b
	}
	return a
}

func severity(s string) int {
	switch s {
	case StateOffline:
		return 3
	case StateStale:
		return 2
	case StateUp:
		return 1
	default:
		return 0
	}
}

// FleetService computes the read-only fleet picture by wiring the proven
// monitoring service (StateForPeer) over every device a tenant owns.
type FleetService struct {
	devices  *database.DeviceRepository
	monitor  *monitoring.MonitoringService
}

func NewFleetService(devices *database.DeviceRepository, monitor *monitoring.MonitoringService) *FleetService {
	return &FleetService{devices: devices, monitor: monitor}
}

// Health enumerates the tenant's devices and attaches each peer's current
// reachability state, then aggregates into a FleetHealth. A single unhealthy
// device must not abort the whole fleet report, so a peer that fails to
// evaluate is recorded as unknown and the loop continues.
func (s *FleetService) Health(ctx context.Context, businessID string) (*FleetHealth, error) {
	devices, err := s.devices.ListDevicesByBusiness(businessID)
	if err != nil {
		return nil, fmt.Errorf("fleet: list devices: %w", err)
	}

	peers := make([]PeerSummary, 0, len(devices))
	for _, d := range devices {
		state := StateUnknown
		if got, err := s.monitor.StateForPeer(ctx, businessID, d.DeviceID); err == nil && got != "" {
			state = got
		}
		peers = append(peers, PeerSummary{
			DeviceID:     d.DeviceID,
			HardwareName: d.HardwareName,
			State:        state,
		})
	}

	summary := AggregateHealth(businessID, peers)
	return &summary, nil
}