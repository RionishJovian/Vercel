package recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"mega-magic/internal/cache"
	"mega-magic/internal/config"
	"mega-magic/internal/messaging"
	"mega-magic/internal/monitoring"
)

const recoverSubject      = "peer.recover"
const recoveryResultSubject = "peer.recovery.result"
const escalateSubject     = "peer.escalate"

// RecoveryService drives the WireGuard peer self-healing loop (spec 4.2). It is
// triggered on `mm.<biz>.peer.recover` (published by the evaluator when a peer
// drifts stale/offline) and attempts the recovery ladder in order, emitting a
// peer.recovery.result per action and escalating via peer.escalate when all
// in-band recovery is exhausted. A RateLimit backoff guard (spec 4.2.6) stops a
// genuinely dead peer from thrashing the hub.
type RecoveryService struct {
	msg    *messaging.Client
	cache  *cache.Client
	monitor *monitoring.MonitoringService
	cfg    config.RecoveryConfig
	logger *zap.SugaredLogger
}

func New(msg *messaging.Client, c *cache.Client, monitor *monitoring.MonitoringService, cfg config.RecoveryConfig, logger *zap.SugaredLogger) *RecoveryService {
	return &RecoveryService{msg: msg, cache: c, monitor: monitor, cfg: cfg, logger: logger}
}

func (s *RecoveryService) Start() error {
	pattern := fmt.Sprintf("mm.*.%s", recoverSubject)
	if err := s.msg.SubscribePattern(pattern, s.handleRecoveryRequest); err != nil {
		return fmt.Errorf("recovery subscribe %s: %w", pattern, err)
	}
	s.logger.Infof("recovery: listening for %s on every tenant", pattern)
	return nil
}

// recoveryRequest is the trigger payload published by the evaluator on stale/offline.
type recoveryRequest struct {
	BusinessID string `json:"business_id"`
	PeerID     string `json:"peer_id"`
	State      string `json:"state"`
}

func (s *RecoveryService) handleRecoveryRequest(m *nats.Msg) {
	if !s.cfg.Enabled {
		return
	}
	parts := strings.Split(m.Subject, ".")
	if len(parts) < 3 {
		return
	}

	var req recoveryRequest
	if err := json.Unmarshal(m.Data, &req); err != nil {
		s.logger.Warnf("recovery: bad recover payload from %s: %v", m.Subject, err)
		return
	}
	if req.PeerID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s.run(ctx, &req)
}

// run executes one recovery pass against a peer: backoff gate, then the
// a/b/c ladder with a peer.recovery.result emission per action, then escalate.
func (s *RecoveryService) run(ctx context.Context, req *recoveryRequest) {
	// Backoff guard (spec 4.2.6, AC#2): at most cfg.MaxAttempts auto-recovery
	// cycles per peer within cfg.BackoffWindow.
	key := s.cache.Key(req.BusinessID, "recovery:att:"+req.PeerID)
	remaining, _, err := s.cache.RateLimit(ctx, key, s.cfg.MaxAttempts, s.cfg.BackoffWindow)
	if err != nil {
		s.logger.Warnf("recovery: backoff check failed for peer %s: %v", req.PeerID, err)
		return
	}
	if remaining <= 0 {
		s.logger.Warnf("recovery: backoff engaged, suppressing auto-recovery for peer %s (attempts maxed)", req.PeerID)
		return
	}

	start := time.Now().UTC()

	// a) Re-probe / fresh evaluation — cheap, clears most transient states.
	state, _ := s.monitor.EvaluatePeer(ctx, req.BusinessID, req.PeerID)
	ttr := time.Now().UTC().Sub(start)
	recovered := state == "up"
	s.emitResult(req, "reprobe", recovered, ttr)
	s.logger.Infof("recovery: reprobe peer %s -> %s (recovered=%v) in %v", req.PeerID, state, recovered, ttr)
	if recovered {
		return
	}

	// b) Disable peer > UDP timeout, then re-enable — busts a stale conntrack
	//    pinhole. Executed hub-side (provisioner); emitted here as the intent,
	//    verify=true only if a reprobe confirms recovery.
	stateB, _ := s.emitHubAction(req, "reset_peer", ttr)
	if stateB == "up" {
		return
	}

	// c) Rotate the WG listen-port — busts a wedged listener. Last in-band action.
	stateC, _ := s.emitHubAction(req, "rotate_listen_port", ttr)
	if stateC == "up" {
		return
	}

	// All in-band recovery exhausted: escalate (AC#5).
	s.emitEscalate(req, state)
	s.logger.Warnf("recovery: escalated peer %s (state %s) after exhausting auto-recovery", req.PeerID, state)
}

// emitHubAction publishes a hub-plane command intent and a peer.recovery.result.
// The provisioner/hub executor performs the actual RouterOS action; verify is an
// immediate re-evaluation (`up` = recovered). Returns the post-action state.
func (s *RecoveryService) emitHubAction(req *recoveryRequest, action string, ttr time.Duration) (string, error) {
	payload := map[string]string{
		"business_id": req.BusinessID,
		"peer_id":     req.PeerID,
		"action":      action,
	}
	if data, err := json.Marshal(payload); err == nil && s.msg != nil {
		_ = s.msg.Publish(req.BusinessID, recoverSubject+".cmd", data)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	state, _ := s.monitor.EvaluatePeer(ctx, req.BusinessID, req.PeerID)
	s.emitResult(req, action, state == "up", ttr)
	return state, nil
}

func (s *RecoveryService) emitResult(req *recoveryRequest, action string, ok bool, ttr time.Duration) {
	if s.msg == nil {
		return
	}
	payload := map[string]string{
		"business_id": req.BusinessID,
		"peer_id":     req.PeerID,
		"action":      action,
		"ok":          fmt.Sprintf("%v", ok),
		"time_to_recover_ms": fmt.Sprintf("%d", ttr.Nanoseconds()/1000000),
		"observed_at": time.Now().UTC().Format(time.RFC3339),
	}
	if data, err := json.Marshal(payload); err == nil {
		_ = s.msg.Publish(req.BusinessID, recoveryResultSubject, data)
	}
}

func (s *RecoveryService) emitEscalate(req *recoveryRequest, state string) {
	if s.msg == nil {
		return
	}
	payload := map[string]string{
		"business_id": req.BusinessID,
		"peer_id":     req.PeerID,
		"state":       state,
		"reason":      "all in-band recovery exhausted",
		"observed_at": time.Now().UTC().Format(time.RFC3339),
	}
	if data, err := json.Marshal(payload); err == nil {
		_ = s.msg.Publish(req.BusinessID, escalateSubject, data)
	}
}