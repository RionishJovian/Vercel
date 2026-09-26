// Package ai is the first cut of the AI/scanning service: it consumes telemetry
// samples, meters an AI-token spend before acting, classifies via a pluggable
// ModelClient, and emits findings. The worker is GATED OFF by default
// (ai.enabled=false); construction is safe regardless.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

const (
	telemetryIngestSubject = "telemetry.ingest"
	findingSubject         = "ai.finding"
)

// Config controls the service. Built from config.AIConfig at the wiring site.
type Config struct {
	Enabled         bool
	Mode            string // "auto" | "fallback" | "ollama"
	OllamaURL       string
	CostPerAnalysis int
}

// Publisher / Subscriber are the messaging seams (satisfied by
// *messaging.Client) so the worker is unit-testable without NATS.
type Publisher interface {
	Publish(businessID, subject string, data []byte) error
}

type Subscriber interface {
	SubscribePattern(pattern string, handler func(msg *nats.Msg)) error
}

type Bus interface {
	Publisher
	Subscriber
}

// Meter spends AI tokens for a business, tenant-scoped. Returns spent=false when
// the balance is exhausted (or absent), without spending.
type Meter interface {
	Spend(ctx context.Context, businessID string, cost int) (bool, error)
}

// Service consumes telemetry and emits findings.
type Service struct {
	bus    Publisher
	sub    Subscriber
	meter  Meter
	client ModelClient
	cfg    Config
	logger *zap.SugaredLogger

	mu   sync.Mutex
	seen map[string]struct{} // dedupe: business|device|rule while unresolved
}

// New builds the service with the production tenant-scoped quota meter.
func New(bus Bus, quotaRepo *database.BusinessMeteredQuotaRepository, db *database.DB, client ModelClient, cfg Config, logger *zap.SugaredLogger) *Service {
	return newService(bus, &quotaMeter{db: db, repo: quotaRepo}, client, cfg, logger)
}

// NewWithMeter builds the service with an injected Meter (used by unit tests).
func NewWithMeter(bus Bus, meter Meter, client ModelClient, cfg Config, logger *zap.SugaredLogger) *Service {
	return newService(bus, meter, client, cfg, logger)
}

func newService(bus Bus, meter Meter, client ModelClient, cfg Config, logger *zap.SugaredLogger) *Service {
	if logger == nil {
		logger = zap.NewNop().Sugar()
	}
	s := &Service{
		meter:  meter,
		client: client,
		cfg:    cfg,
		logger: logger,
		seen:   make(map[string]struct{}),
	}
	if bus != nil {
		s.bus = bus
		s.sub = bus
	}
	return s
}

// Start subscribes the telemetry handler. Gated: when ai.enabled is false it does
// nothing and returns nil (the service is constructed but inert).
func (s *Service) Start() error {
	if !s.cfg.Enabled {
		s.logger.Infow("ai service disabled (ai.enabled=false)")
		return nil
	}
	if s.sub == nil {
		return fmt.Errorf("ai: no subscriber configured")
	}
	if err := s.sub.SubscribePattern(telemetryIngestSubject, s.handle); err != nil {
		return fmt.Errorf("ai: subscribe %s: %w", telemetryIngestSubject, err)
	}
	s.logger.Infow("ai service started", "subject", telemetryIngestSubject, "mode", s.cfg.Mode)
	return nil
}

func (s *Service) handle(msg *nats.Msg) {
	var sample model.TelemetrySample
	if err := json.Unmarshal(msg.Data, &sample); err != nil {
		s.logger.Warnf("ai: decode telemetry sample: %v", err)
		return
	}
	s.process(context.Background(), sample)
}

// process runs one sample through: analyze -> (healthy? clear) -> dedupe ->
// meter (spend) -> publish. Never panics; every failure path is logged.
func (s *Service) process(ctx context.Context, sample model.TelemetrySample) {
	finding, err := s.client.Analyze(ctx, sample)
	if err != nil {
		// Fail closed: an analysis error is not a finding and costs nothing.
		s.logger.Warnf("ai: analyze sample for %s: %v", sample.BusinessID, err)
		return
	}
	if finding == nil {
		s.clearSeen(sample) // a healthy sample clears the prior unresolved finding
		return
	}

	key := dedupeKey(sample, finding)
	if s.isSeen(key) {
		return // already fired and unresolved — drop the repeat
	}

	// Meter first, spend last: the token is spent only once we have a finding.
	spent, err := s.meter.Spend(ctx, sample.BusinessID, s.cfg.CostPerAnalysis)
	if err != nil {
		s.logger.Warnf("ai: meter spend for %s: %v", sample.BusinessID, err)
		return
	}
	if !spent {
		s.logger.Infof("ai quota exhausted for business %s, skipping analysis", sample.BusinessID)
		return
	}

	data, err := json.Marshal(finding)
	if err != nil {
		s.logger.Warnf("ai: marshal finding: %v", err)
		return
	}
	if err := s.bus.Publish(sample.BusinessID, findingSubject, data); err != nil {
		s.logger.Warnf("ai: publish finding for %s: %v", sample.BusinessID, err)
		return
	}
	s.markSeen(key)
}

func dedupeKey(sample model.TelemetrySample, f *Finding) string {
	dev := ""
	if sample.DeviceID != nil {
		dev = *sample.DeviceID
	}
	return sample.BusinessID + "|" + dev + "|" + f.Rule
}

func (s *Service) devicePrefix(sample model.TelemetrySample) string {
	dev := ""
	if sample.DeviceID != nil {
		dev = *sample.DeviceID
	}
	return sample.BusinessID + "|" + dev + "|"
}

func (s *Service) isSeen(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.seen[key]
	return ok
}

func (s *Service) markSeen(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[key] = struct{}{}
}

func (s *Service) clearSeen(sample model.TelemetrySample) {
	prefix := s.devicePrefix(sample)
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.seen {
		if strings.HasPrefix(k, prefix) {
			delete(s.seen, k)
		}
	}
}

// nextBalance is the pure metering math; never returns below zero.
func nextBalance(balance, cost int) int {
	if cost < 0 {
		cost = 0
	}
	if balance-cost < 0 {
		return 0
	}
	return balance - cost
}

// ---------------------------------------------------------------------------
// quotaMeter — production Meter: tenant-scoped read+write in ONE tx.
// ---------------------------------------------------------------------------

type quotaMeter struct {
	db   *database.DB
	repo *database.BusinessMeteredQuotaRepository
}

func (m *quotaMeter) Spend(ctx context.Context, businessID string, cost int) (bool, error) {
	if m.db == nil || m.repo == nil {
		return false, fmt.Errorf("ai: quota meter not configured")
	}
	tx, err := m.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	spent := false
	err = database.WithTenant(tx, businessID, func() error {
		q, e := m.repo.GetByBusinessTXN(tx, businessID)
		if e != nil {
			return e
		}
		if q == nil || q.AITokenBalance <= 0 {
			return nil // exhausted / absent -> no spend
		}
		next := nextBalance(q.AITokenBalance, cost)
		if next == q.AITokenBalance {
			return nil // zero-cost analysis: nothing to write
		}
		if e := m.repo.UpdateAITokenBalanceTXN(tx, businessID, next); e != nil {
			return e
		}
		spent = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return spent, nil
}
