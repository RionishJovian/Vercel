package evaluator

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"mega-magic/internal/database"
	"mega-magic/internal/messaging"
	"mega-magic/internal/model"
	"mega-magic/internal/notifier"
)

const telemetrySubject = "telemetry.ingest"
const monitorStateSubject = "monitor.peer.state"

// Evaluator watches the NATS telemetry stream and auto-opens incidents when a
// sample breaches a tenant's alert policy threshold. It dedupes per
// business+subject+kind so the same condition doesn't re-fire while open.
type Evaluator struct {
	msg          *messaging.Client
	policyRepo   *database.AlertPolicyRepository
	incidentRepo *database.IncidentRepository
	notifier     *notifier.Notifier
	logger       *zap.SugaredLogger
}

func New(
	msg *messaging.Client,
	policyRepo *database.AlertPolicyRepository,
	incidentRepo *database.IncidentRepository,
	notifier *notifier.Notifier,
	logger *zap.SugaredLogger,
) *Evaluator {
	return &Evaluator{msg: msg, policyRepo: policyRepo, incidentRepo: incidentRepo, notifier: notifier, logger: logger}
}

// Start subscribes to the cross-tenant telemetry pattern `mm.*.telemetry.ingest`
// and the peer state pattern `mm.*.monitor.peer.state`.
func (e *Evaluator) Start() error {
	pattern := fmt.Sprintf("mm.*.%s", telemetrySubject)
	if err := e.msg.SubscribePattern(pattern, e.handleSample); err != nil {
		return fmt.Errorf("evaluator subscribe %s: %w", pattern, err)
	}
	e.logger.Infof("evaluator: watching %s for every tenant", pattern)

	statePattern := fmt.Sprintf("mm.*.%s", monitorStateSubject)
	if err := e.msg.SubscribePattern(statePattern, e.handlePeerState); err != nil {
		return fmt.Errorf("evaluator subscribe %s: %w", statePattern, err)
	}
	e.logger.Infof("evaluator: watching %s for every tenant", statePattern)
	return nil
}

// sampleEnvelope is the minimal payload the ingest handler publishes to NATS.
type sampleEnvelope struct {
	BusinessID    string   `json:"business_id"`
	DeviceID      *string  `json:"device_id,omitempty"`
	SubjectKind   string   `json:"subject_kind"`
	SubjectID     string   `json:"subject_id"`
	SubjectLabel  string   `json:"subject_label,omitempty"`
	CPUPercent    *float64 `json:"cpu_pct,omitempty"`
	MemoryPercent *float64 `json:"memory_pct,omitempty"`
	LatencyMs     *int     `json:"latency_ms,omitempty"`
	WANState      *string  `json:"wan_state,omitempty"`
	ConnectorState *string `json:"connector_state,omitempty"`
	TunnelState   *string  `json:"tunnel_state,omitempty"`
	ObservedAt    time.Time `json:"observed_at"`
}

func (e *Evaluator) handleSample(m *nats.Msg) {
	// Subject shape: mm.<business_id>.telemetry.ingest
	parts := strings.Split(m.Subject, ".")
	if len(parts) < 3 {
		return
	}
	businessID := parts[1]

	var env sampleEnvelope
	if err := json.Unmarshal(m.Data, &env); err != nil {
		e.logger.Warnf("evaluator: bad sample payload from %s: %v", m.Subject, err)
		return
	}
	if env.SubjectID == "" {
		return
	}

	policies, err := e.policyRepo.ListEnabled(businessID)
	if err != nil {
		e.logger.Warnf("evaluator: list policies for %s: %v", businessID, err)
		return
	}

	for _, p := range policies {
		if !e.breached(p, &env) {
			continue
		}
		if err := e.raiseIncident(businessID, p, &env); err != nil {
			e.logger.Warnf("evaluator: raise incident (kind=%s subject=%s): %v", p.Kind, env.SubjectID, err)
		}
	}
}

// breached evaluates one policy against the sample.
func (e *Evaluator) breached(p *model.AlertPolicy, s *sampleEnvelope) bool {
	switch p.Kind {
	case "cpu_high":
		return s.CPUPercent != nil && *s.CPUPercent > p.Threshold
	case "memory_high":
		return s.MemoryPercent != nil && *s.MemoryPercent > p.Threshold
	case "latency_high":
		return s.LatencyMs != nil && float64(*s.LatencyMs) > p.Threshold
	case "wan_down":
		return s.WANState != nil && *s.WANState == "down"
	case "tunnel_down":
		return s.TunnelState != nil && *s.TunnelState == "down"
	default:
		return false
	}
}

// raiseIncident creates the incident unless one is already open for this
// business+subject+kind (cooldown/dedupe).
func (e *Evaluator) raiseIncident(businessID string, p *model.AlertPolicy, s *sampleEnvelope) error {
	open, err := e.incidentRepo.HasOpenIncidentBySubject(businessID, s.SubjectID, p.Kind)
	if err != nil {
		return err
	}
	if open {
		return nil // already firing, don't stack
	}

	now := time.Now().UTC()
	detail := fmt.Sprintf("%s threshold breached (policy %s, value above %.2f)", p.Kind, p.ID[:8], p.Threshold)
	inc := &model.Incident{
		ID:           newID(),
		BusinessID:   businessID,
		DeviceID:     s.DeviceID,
		PolicyID:     &p.ID,
		Kind:         p.Kind,
		Severity:     p.Severity,
		SubjectID:    s.SubjectID,
		SubjectLabel: s.SubjectLabel,
		Detail:       &detail,
		OpenedAt:     now,
	}

	created, err := e.incidentRepo.Create(inc)
	if err != nil {
		return err
	}
	e.logger.Infof("evaluator: opened %s incident %s for subject %s (business %s)",
		p.Kind, created.ID, s.SubjectID, businessID)

	if e.notifier != nil {
		e.notifier.IncidentOpened(businessID, created)
	}
	return nil
}

// peerStateEnvelope is the payload the monitoring service publishes to
// `mm.<business_id>.monitor.peer.state` after a peer reachability evaluation.
type peerStateEnvelope struct {
	BusinessID string   `json:"business_id"`
	PeerID     string   `json:"peer_id"`
	State      string   `json:"state"`
	ObservedAt time.Time `json:"observed_at"`
}

// handlePeerState escalates a peer that has drifted into `stale` (warning) or
// `offline` (critical) to an incident. It dedupes per business+peer+kind so the
// same condition doesn't stack while an incident is already open.
func (e *Evaluator) handlePeerState(m *nats.Msg) {
	// Subject shape: mm.<business_id>.monitor.peer.state
	parts := strings.Split(m.Subject, ".")
	if len(parts) < 3 {
		return
	}

	var env peerStateEnvelope
	if err := json.Unmarshal(m.Data, &env); err != nil {
		e.logger.Warnf("evaluator: bad peer-state payload from %s: %v", m.Subject, err)
		return
	}
	if env.PeerID == "" {
		return
	}

	if env.State != "stale" && env.State != "offline" {
		return
	}
	if err := e.raisePeerIncident(env.BusinessID, &env); err != nil {
		e.logger.Warnf("evaluator: raise peer incident (peer=%s state=%s): %v", env.PeerID, env.State, err)
	}
}

// raisePeerIncident creates a peer_offline incident unless one is already open
// for this business+peer+kind.
func (e *Evaluator) raisePeerIncident(businessID string, env *peerStateEnvelope) error {
	open, err := e.incidentRepo.HasOpenIncidentBySubject(businessID, env.PeerID, "peer_offline")
	if err != nil {
		return err
	}
	if open {
		return nil // already firing, don't stack
	}

	severity := "warning"
	detail := fmt.Sprintf("peer reachability state %q drifted past threshold", env.State)
	if env.State == "offline" {
		severity = "critical"
		detail = "peer offline — all in-band recovery exhausted, site/escalation required"
	}

	now := time.Now().UTC()
	inc := &model.Incident{
		ID:         newID(),
		BusinessID: businessID,
		DeviceID:   &env.PeerID,
		Kind:       "peer_offline",
		Severity:   severity,
		SubjectID:  env.PeerID,
		Detail:     &detail,
		OpenedAt:   now,
	}

	created, err := e.incidentRepo.Create(inc)
	if err != nil {
		return err
	}
	e.logger.Infof("evaluator: opened %s peer incident %s for peer %s (business %s)",
		severity, created.ID, env.PeerID, businessID)

	// Kick the self-healing loop: the recovery worker consumes this and runs the
	// a/b/c recovery ladder (spec 4.2). Fire-and-forget.
	if e.msg != nil {
		trig := map[string]string{
			"business_id": businessID,
			"peer_id":     env.PeerID,
			"state":       env.State,
		}
		if data, err := json.Marshal(trig); err == nil {
			_ = e.msg.Publish(businessID, "peer.recover", data)
		}
	}

	if e.notifier != nil {
		e.notifier.IncidentOpened(businessID, created)
	}
	return nil
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}