// Package ai is the first cut of the AI/scanning service: it consumes telemetry
// samples, meters an AI-token spend before acting, classifies via a pluggable
// ModelClient, emits findings, and opens incidents for them. The worker is GATED
// OFF by default (ai.enabled=false); construction is safe regardless.
package ai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

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
	Mode            string // "auto" | "fallback" | "ollama" | "openrouter"
	OllamaURL       string
	CostPerAnalysis int
	// OpenRouter fields are only used when Mode == "openrouter".
	OpenRouterURL    string
	OpenRouterAPIKey string
	OpenRouterModel  string
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

// IncidentStore opens an incident, tenant-scoped and deduped by an open
// subject+kind (the durable guard). Returns (created=true, nil) when a NEW
// incident was opened; (false, nil) when one is already open; (false, err) on
// failure with nothing written.
type IncidentStore interface {
	CreateIncident(businessID string, inc model.Incident) (bool, error)
}

// IncidentNotifier is the notify seam (satisfied by *notifier.Notifier).
type IncidentNotifier interface {
	IncidentOpened(businessID string, inc *model.Incident)
}

// Service consumes telemetry and emits findings (and opens incidents).
type Service struct {
	bus       Publisher
	sub       Subscriber
	meter     Meter
	incidents IncidentStore
	notifier  IncidentNotifier
	client    ModelClient
	cfg       Config
	logger    *zap.SugaredLogger

	mu   sync.Mutex
	seen map[string]struct{} // dedupe: business|device|rule while unresolved
}

// New builds the service with the production tenant-scoped quota meter and
// incident store. The notifier is attached separately (WithNotifier) so tests
// stay dependency-free.
func New(bus Bus, quotaRepo *database.BusinessMeteredQuotaRepository, db *database.DB, client ModelClient, cfg Config, logger *zap.SugaredLogger) *Service {
	s := newService(bus, &quotaMeter{db: db, repo: quotaRepo}, client, cfg, logger)
	if db != nil {
		s.incidents = &databaseIncidentStore{db: db, repo: database.NewIncidentRepository(db)}
	}
	return s
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

// WithIncidentStore attaches the incident store (used by tests / DI).
func (s *Service) WithIncidentStore(store IncidentStore) *Service {
	s.incidents = store
	return s
}

// WithNotifier attaches the incident notifier.
func (s *Service) WithNotifier(n IncidentNotifier) *Service {
	s.notifier = n
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
// meter (spend) -> publish -> open incident. Never panics; failures are logged.
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

	// Publish a self-describing envelope (finding + device/subject context) so the
	// ai.finding subject is meaningful to any consumer.
	data, err := json.Marshal(findingEnvelope{
		Finding:      *finding,
		BusinessID:   sample.BusinessID,
		DeviceID:     sample.DeviceID,
		SubjectID:    subjectIDFor(sample, finding),
		SubjectLabel: subjectLabelFor(sample, finding),
	})
	if err != nil {
		s.logger.Warnf("ai: marshal finding: %v", err)
		return
	}
	if err := s.bus.Publish(sample.BusinessID, findingSubject, data); err != nil {
		s.logger.Warnf("ai: publish finding for %s: %v", sample.BusinessID, err)
		return
	}
	s.markSeen(key)

	// Couple incident opening to a SUCCESSFUL publish.
	s.consumeFinding(sample, finding)
}

// consumeFinding opens an incident for a finding (mirrors evaluator.raiseIncident:
// open only, never resolve/ack). Fail-closed: a store error logs and does not
// crash or double-spend.
func (s *Service) consumeFinding(sample model.TelemetrySample, finding *Finding) {
	if s.incidents == nil {
		return
	}
	inc := findingToIncident(sample, finding, time.Now().UTC())
	created, err := s.incidents.CreateIncident(sample.BusinessID, inc)
	if err != nil {
		s.logger.Warnf("ai: open incident for %s/%s: %v", sample.BusinessID, inc.SubjectID, err)
		return
	}
	if !created {
		return // already open for this subject+kind — durable dedupe
	}
	s.logger.Infof("ai: opened %s incident %s for subject %s (business %s)", inc.Kind, inc.ID, inc.SubjectID, sample.BusinessID)
	if s.notifier != nil {
		s.notifier.IncidentOpened(sample.BusinessID, &inc)
	}
}

// findingEnvelope is the ai.finding payload: the finding plus the context needed
// to open a meaningful incident.
type findingEnvelope struct {
	Finding      Finding `json:"finding"`
	BusinessID   string  `json:"business_id"`
	DeviceID     *string `json:"device_id,omitempty"`
	SubjectID    string  `json:"subject_id"`
	SubjectLabel string  `json:"subject_label"`
}

// findingToIncident maps a finding to an Incident. PolicyID stays nil (an AI
// finding has no alert-policy row). Timestamps are UTC now.
func findingToIncident(sample model.TelemetrySample, finding *Finding, now time.Time) model.Incident {
	summary := finding.Summary
	if strings.TrimSpace(summary) == "" {
		summary = "AI finding: " + finding.Rule
	}
	return model.Incident{
		ID:              newID(),
		BusinessID:      sample.BusinessID,
		DeviceID:        sample.DeviceID,
		PolicyID:        nil,
		Kind:            "ai_" + sanitizeRule(finding.Rule),
		Severity:        normalizeSeverity(finding.Severity),
		SubjectID:       subjectIDFor(sample, finding),
		SubjectLabel:    subjectLabelFor(sample, finding),
		Detail:          &summary,
		SuggestedAction: finding.SuggestedAction,
		OpenedAt:        now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// subjectIDFor prefers the device id, else a synthetic per-rule subject.
func subjectIDFor(sample model.TelemetrySample, finding *Finding) string {
	if sample.DeviceID != nil && strings.TrimSpace(*sample.DeviceID) != "" {
		return *sample.DeviceID
	}
	if strings.TrimSpace(sample.SubjectID) != "" {
		return sample.SubjectID
	}
	return sanitizeRule(finding.Rule)
}

// subjectLabelFor is a human-readable label for the incident.
func subjectLabelFor(sample model.TelemetrySample, finding *Finding) string {
	if sample.DeviceID != nil && strings.TrimSpace(*sample.DeviceID) != "" {
		return fmt.Sprintf("device %s %s", *sample.DeviceID, finding.Rule)
	}
	return finding.Rule
}

// sanitizeRule keeps alphanumerics (lowercased) and underscores only.
func sanitizeRule(rule string) string {
	rule = strings.ToLower(strings.TrimSpace(rule))
	var b strings.Builder
	for _, r := range rule {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = "unknown"
	}
	return out
}

// normalizeSeverity maps a finding severity to the incidents.severity CHECK
// values ('info','warning','critical'). Empty/unknown -> "info"; "warn" ->
// "warning" (the DB's spelling — the finding vocabulary says "warn").
func normalizeSeverity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return "critical"
	case "warn", "warning":
		return "warning"
	case "info":
		return "info"
	default:
		return "info"
	}
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
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

// ---------------------------------------------------------------------------
// databaseIncidentStore — production IncidentStore: dedupe-then-create in one
// tenant-scoped tx (mirrors evaluator.raiseIncident's guard).
// ---------------------------------------------------------------------------

type databaseIncidentStore struct {
	db   *database.DB
	repo *database.IncidentRepository
}

func (m *databaseIncidentStore) CreateIncident(businessID string, inc model.Incident) (bool, error) {
	if m.db == nil || m.repo == nil {
		return false, fmt.Errorf("ai: incident store not configured")
	}
	tx, err := m.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	created := false
	err = database.WithTenant(tx, businessID, func() error {
		open, e := m.repo.HasOpenIncidentBySubjectTXN(tx, businessID, inc.SubjectID, inc.Kind)
		if e != nil {
			return e
		}
		if open {
			return nil // already open — durable dedupe, do not stack
		}
		if _, e := m.repo.CreateTXN(tx, &inc); e != nil {
			return e
		}
		created = true
		return nil
	})
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return created, nil
}
