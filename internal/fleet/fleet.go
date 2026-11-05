package fleet

import (
	"context"
	"fmt"
	"sync"
	"time"

	"mega-magic/internal/database"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
)

// Peer states the fleet understands. These mirror the probe classifier output
// so the aggregate can be built from Evaluate/StateForPeer results directly.
const (
	StateUp      = "up"
	StateStale   = "stale"
	StateOffline = "offline"
	StateUnknown = "unknown"
)

// Where a state came from. Every page shows the same state because this service is
// the only thing that decides it.
const (
	SourceHubTunnel   = "hub_tunnel"   // the Hub's live WireGuard handshake
	SourceRouterCheck = "router_check" // the monitoring service's own check of the router
	SourceRouterRead  = "router_read"  // a recent successful read of the router
	SourceNone        = "none"         // nothing has answered yet
)

const (
	freshContact     = 5 * time.Minute
	hubLookupBudget  = 4 * time.Second
	hubLookupsAtOnce = 8
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
	Source       string `json:"source"`
	// LastContactAt is when the router was last heard from, where the source knows it.
	LastContactAt *time.Time `json:"last_contact_at,omitempty"`
	// CheckedAt is when this state was read live. Absent for a cached router check.
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	// StateSince is when the recorded connection history says this state began.
	StateSince *time.Time `json:"state_since,omitempty"`
}

// AggregateHealth is a pure function: it folds a slice of peer states into a
// FleetHealth summary (counts + overall state) with no I/O. Keeping the state
// math here makes it directly unit-testable without a DB.
//
// overall_state is the worst of the reported states, best-first:
//
//	unknown (no data yet) < up < stale (data, connectivity slipping)
//	< offline (hard down / unreachable).
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

type stateReader interface {
	StateForPeer(ctx context.Context, businessID, peerID string) (string, error)
	LastRouterRead(ctx context.Context, businessID, peerID string) (time.Time, bool)
}

type connectionHistory interface {
	ListGatewaysAcrossTenants() ([]*model.Device, error)
	LatestConnectionChanges(businessID string) (map[string]database.ConnectionChange, error)
	RecordConnectionChange(businessID, deviceID, action, reason string) error
}

type routerAlerts interface {
	DueRouterAlerts(grace time.Duration) ([]*database.RouterAlert, error)
	CreateRouterAlert(*database.RouterAlert) (bool, error)
}

// routerAlertGrace is how long a connected router must stay down before the owner
// hears about it, so a restart or brief drop stays quiet.
const routerAlertGrace = 15 * time.Minute

type hubInspector interface {
	InspectRouter(context.Context, hub.Identity) (hub.Peer, error)
}

// FleetService is the single source of a router's connection state. Fleet health,
// Topology, the Router tab and Magic Hub all read it through /fleet/health.
type FleetService struct {
	devices *database.DeviceRepository
	monitor stateReader
	hub     hubInspector
	history connectionHistory
	alerts  routerAlerts
	notify  func(string) error
	now     func() time.Time
}

func NewFleetService(devices *database.DeviceRepository, monitor stateReader) *FleetService {
	return &FleetService{devices: devices, monitor: monitor, history: devices, alerts: devices, now: time.Now}
}

// ConfigureHub lets gateways use the Hub's live tunnel state. Inspection only.
func (s *FleetService) ConfigureHub(client hubInspector) { s.hub = client }

// Health enumerates the tenant's devices and attaches each one's state. A device
// that cannot be read is unknown and never stops the report.
func (s *FleetService) Health(ctx context.Context, businessID, requestedBy string) (*FleetHealth, error) {
	devices, err := s.devices.ListDevicesByBusiness(businessID)
	if err != nil {
		return nil, fmt.Errorf("fleet: list devices: %w", err)
	}
	summary := AggregateHealth(businessID, s.withStateSince(businessID, s.summaries(ctx, businessID, requestedBy, devices)))
	return &summary, nil
}

// summaries decides each device's state: the Hub tunnel when the Hub knows the
// gateway, otherwise the router check, otherwise a recent successful router read.
func (s *FleetService) summaries(ctx context.Context, businessID, requestedBy string, devices []*model.Device) []PeerSummary {
	live := s.hubSummaries(ctx, businessID, requestedBy, devices)
	peers := make([]PeerSummary, 0, len(devices))
	for _, d := range devices {
		peer, known := live[d.DeviceID]
		if !known {
			peer = s.localSummary(ctx, businessID, d.DeviceID)
		}
		peer.DeviceID, peer.HardwareName = d.DeviceID, d.HardwareName
		peers = append(peers, peer)
	}
	return peers
}

func (s *FleetService) hubSummaries(ctx context.Context, businessID, requestedBy string, devices []*model.Device) map[string]PeerSummary {
	out := map[string]PeerSummary{}
	if s.hub == nil {
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, hubLookupBudget)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, hubLookupsAtOnce)
	for _, d := range devices {
		if d.DeploymentLayer != "MAIN_GATEWAY" {
			continue
		}
		wg.Add(1)
		slots <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-slots }()
			peer, err := s.hub.InspectRouter(ctx, hub.Identity{TenantID: businessID, RouterID: id, RequestedBy: requestedBy})
			// No peer, a failed lookup or someone else's peer says nothing about this router.
			if err != nil || peer.TenantID != businessID || peer.RouterID != id {
				return
			}
			summary := hubSummary(peer, s.now())
			mu.Lock()
			out[id] = summary
			mu.Unlock()
		}(d.DeviceID)
	}
	wg.Wait()
	return out
}

func hubSummary(peer hub.Peer, now time.Time) PeerSummary {
	checked := now.UTC()
	summary := PeerSummary{State: StateOffline, Source: SourceHubTunnel, LastContactAt: peer.LastHandshakeAt, CheckedAt: &checked}
	if peer.State == "active" && peer.LastHandshakeAt != nil {
		if age := now.Sub(*peer.LastHandshakeAt); age >= 0 && age <= freshContact {
			summary.State = StateUp
		}
	}
	return summary
}

func (s *FleetService) localSummary(ctx context.Context, businessID, id string) PeerSummary {
	state := StateUnknown
	if got, err := s.monitor.StateForPeer(ctx, businessID, id); err == nil && got != "" {
		state = got
	}
	summary := PeerSummary{State: state, Source: SourceRouterCheck}
	read, ok := s.monitor.LastRouterRead(ctx, businessID, id)
	if ok {
		summary.LastContactAt = &read
	}
	if state != StateUnknown {
		return summary
	}
	if age := s.now().Sub(read); ok && age >= 0 && age <= freshContact {
		summary.State, summary.Source = StateUp, SourceRouterRead
		return summary
	}
	summary.Source = SourceNone
	return summary
}

// connectionAction is the change-log action for a Hub-reported state.
func connectionAction(state string) string {
	if state == StateUp {
		return database.ActionConnectionUp
	}
	return database.ActionConnectionOffline
}

// withStateSince adds when the current Hub state began, where history records it.
// History is extra detail, so failing to read it never fails the report.
func (s *FleetService) withStateSince(businessID string, peers []PeerSummary) []PeerSummary {
	changes, err := s.history.LatestConnectionChanges(businessID)
	if err != nil {
		return peers
	}
	for i := range peers {
		change, ok := changes[peers[i].DeviceID]
		if ok && peers[i].Source == SourceHubTunnel && change.Action == connectionAction(peers[i].State) {
			at := change.At
			peers[i].StateSince = &at
		}
	}
	return peers
}

// RecordConnectionChanges logs each gateway whose Hub state differs from its last
// recorded one. Only the Hub's own answers count: when the Hub cannot answer, nothing
// is known and nothing is written, so an outage of the lookup is not a router drop.
func (s *FleetService) RecordConnectionChanges(ctx context.Context) error {
	if s.hub == nil {
		return nil
	}
	gateways, err := s.history.ListGatewaysAcrossTenants()
	if err != nil {
		return err
	}
	byBusiness := map[string][]*model.Device{}
	for _, d := range gateways {
		byBusiness[d.BusinessID] = append(byBusiness[d.BusinessID], d)
	}
	var firstErr error
	note := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	for businessID, devices := range byBusiness {
		changes, err := s.history.LatestConnectionChanges(businessID)
		if err != nil {
			note(err)
			continue
		}
		for id, peer := range s.hubSummaries(ctx, businessID, connectionHistoryRequester, devices) {
			action := connectionAction(peer.State)
			if changes[id].Action == action {
				continue
			}
			if err := s.history.RecordConnectionChange(businessID, id, action, connectionReason(peer)); err != nil {
				note(err)
			}
		}
	}
	return firstErr
}

const connectionHistoryRequester = "connection-history"

func connectionReason(peer PeerSummary) string {
	if peer.State == StateUp {
		return "Connected to Magic Hub."
	}
	if peer.LastContactAt == nil {
		return "No Magic Hub connection. No contact on record."
	}
	return "No Magic Hub connection. Last contact " + peer.LastContactAt.UTC().Format(time.RFC3339) + "."
}

// ConfigureAlertMessages sends each new router alert to the platform owner as well
// as showing it in the console. Without this the console is the only place.
func (s *FleetService) ConfigureAlertMessages(notify func(string) error) { s.notify = notify }

// RaiseRouterAlerts stores every due alert and sends each new one. A failed send
// doesn't undo the alert: it stays in the console, and it isn't sent twice.
func (s *FleetService) RaiseRouterAlerts() error {
	due, err := s.alerts.DueRouterAlerts(routerAlertGrace)
	if err != nil {
		return err
	}
	var firstErr error
	for _, alert := range due {
		created, err := s.alerts.CreateRouterAlert(alert)
		if err == nil && created && s.notify != nil {
			err = s.notify(routerAlertMessage(alert))
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func routerAlertMessage(a *database.RouterAlert) string {
	if a.Kind == database.RouterAlertRecovered {
		return "Router back online: " + a.HardwareName + " (" + a.CompanyName + ")."
	}
	return "Router offline: " + a.HardwareName + " (" + a.CompanyName + "). " + a.Detail
}

// StartConnectionHistory runs RecordConnectionChanges until the process exits.
func (s *FleetService) StartConnectionHistory(interval time.Duration, logf func(string, ...interface{})) {
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		for {
			if err := s.RecordConnectionChanges(context.Background()); err != nil {
				logf("connection history: %v", err)
			}
			if err := s.RaiseRouterAlerts(); err != nil {
				logf("router alerts: %v", err)
			}
			time.Sleep(interval)
		}
	}()
}
