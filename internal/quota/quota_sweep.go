// Package quota holds the quota-ENFORCEMENT sweep: a periodic background worker
// that detects LAPSED Sovereign passes (a business's webfig_pass_expires_at in
// the past) and emits a real downstream signal. It mirrors the monitoring state
// sweep's shape (goroutine started once, interval-paced, zero-guard).
//
// The router-side revoke is deliberately GATED elsewhere (provisioner.applier
// enabled=false); this worker only DETECTS + SIGNALS.
package quota

import (
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"mega-magic/internal/database"
)

// Lister enumerates every tenant's pass expiry (system-level, cross-tenant).
// *database.BusinessMeteredQuotaRepository satisfies it.
type Lister interface {
	ListAllQuotas() ([]database.PassExpiry, error)
}

// Publisher emits the pass-expired event. *messaging.Client satisfies it.
type Publisher interface {
	Publish(businessID, subject string, data []byte) error
}

const passExpiredSubject = "quota.pass.expired"

// Sweep periodically detects expired Sovereign passes, independent of any HTTP
// request. The push side of quota enforcement: a lapsed pass produces a
// quota.pass.expired event without anyone asking.
type Sweep struct {
	quotas   Lister
	msg      Publisher
	logger   *zap.SugaredLogger
	interval time.Duration
	now      func() time.Time
}

func NewSweep(quotas Lister, msg Publisher, logger *zap.SugaredLogger, interval time.Duration) *Sweep {
	return &Sweep{
		quotas:   quotas,
		msg:      msg,
		logger:   logger,
		interval: interval,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// Start launches the sweep loop on a goroutine. It never returns; the loop runs
// until the process exits.
func (s *Sweep) Start() {
	go func() {
		if s.interval.Nanoseconds() == 0 {
			s.logger.Warnf("quota enforcement sweep: zero interval, using 60s")
			s.interval = 60 * time.Second
		}
		for {
			s.pass()
			time.Sleep(s.interval)
		}
	}()
}

// pass detects expired passes and emits ONE event per expired tenant. A nil or
// future expiry is NOT emitted. Errors are logged and swallowed per tenant so one
// bad row cannot abort the sweep.
//
// DEDUPE: there is no "already-signalled" marker on the quota row yet, so an
// expired pass re-emits once per sweep pass. The consumer MUST dedupe by
// (business_id, expired_at) — flagged to the Ledger; a persisted marker is a
// clean follow-up. Publishing happens only AFTER the DB enumeration succeeds
// (publish-after-storage rule).
func (s *Sweep) pass() {
	rows, err := s.quotas.ListAllQuotas()
	if err != nil {
		s.logger.Warnf("quota enforcement sweep: list all quotas: %v", err)
		return
	}

	now := s.now()
	for _, q := range rows {
		if q.ExpiresAt == nil || q.ExpiresAt.After(now) {
			continue // no pass, or still live — never emit
		}

		payload := map[string]string{
			"business_id": q.BusinessID,
			"expired_at":  q.ExpiresAt.UTC().Format(time.RFC3339),
		}
		data, err := json.Marshal(payload)
		if err != nil {
			s.logger.Warnf("quota enforcement sweep: marshal payload for %s: %v", q.BusinessID, err)
			continue
		}
		if err := s.msg.Publish(q.BusinessID, passExpiredSubject, data); err != nil {
			s.logger.Warnf("quota enforcement sweep: publish for %s: %v", q.BusinessID, err)
			continue
		}
		s.logger.Infof("quota enforcement sweep: pass expired for %s at %s", q.BusinessID, q.ExpiresAt.UTC().Format(time.RFC3339))
	}
}
