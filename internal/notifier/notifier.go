package notifier

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/security"
)

const (
	// MaxAttempts is how many times a delivery is tried before giving up.
	MaxAttempts = 3
	// BackoffBetweenAttempts spaces out retries.
	BackoffBetweenAttempts = 5 * time.Second
)

// Notifier fans incident events out to the tenant's webhook endpoints.
//
// It holds the DB handle so every webhook-repo call can run inside a
// tenant-bound transaction (WithTenant). Without that, enabling RLS would
// deny-all the worker's reads/writes and outbound webhooks would silently stop.
type Notifier struct {
	webhookRepo *database.WebhookRepository
	db          *database.DB
	client      *http.Client
	logger      *zap.SugaredLogger
}

func New(repo *database.WebhookRepository, db *database.DB, logger *zap.SugaredLogger) *Notifier {
	return &Notifier{
		webhookRepo: repo,
		db:          db,
		// Tenant-supplied URLs: never connect to internal addresses or follow redirects.
		client: security.SafeHTTPClient(10 * time.Second),
		logger: logger,
	}
}

// withTenant runs fn in a transaction RLS-bound to businessID. When no DB handle
// is configured (legacy construction), it falls back to the caller's bare path
// so behaviour is unchanged.
func (n *Notifier) withTenant(businessID string, fn func(tx *sql.Tx) error) error {
	if n.db == nil {
		return nil // caller handles the bare fallback
	}
	tx, err := n.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := database.WithTenant(tx, businessID, func() error { return fn(tx) }); err != nil {
		return err
	}
	return tx.Commit()
}

func (n *Notifier) listEnabledForEvent(businessID, eventType string) ([]*model.WebhookEndpoint, error) {
	if n.db == nil {
		return n.webhookRepo.ListEnabledForEvent(businessID, eventType)
	}
	var out []*model.WebhookEndpoint
	err := n.withTenant(businessID, func(tx *sql.Tx) error {
		eps, e := n.webhookRepo.ListEnabledForEventTXN(tx, businessID, eventType)
		if e != nil {
			return e
		}
		out = eps
		return nil
	})
	return out, err
}

func (n *Notifier) createDelivery(d *model.WebhookDelivery, businessID string) (*model.WebhookDelivery, error) {
	if n.db == nil {
		return n.webhookRepo.CreateDelivery(d)
	}
	var out *model.WebhookDelivery
	err := n.withTenant(businessID, func(tx *sql.Tx) error {
		dd, e := n.webhookRepo.CreateDeliveryTXN(tx, d)
		if e != nil {
			return e
		}
		out = dd
		return nil
	})
	return out, err
}

func (n *Notifier) markDeliveryResult(businessID, id, status string, statusCode int, errMsg *string) error {
	if n.db == nil {
		return n.webhookRepo.MarkDeliveryResult(id, status, statusCode, errMsg)
	}
	return n.withTenant(businessID, func(tx *sql.Tx) error {
		return n.webhookRepo.MarkDeliveryResultTXN(tx, id, status, statusCode, errMsg)
	})
}

// IncidentOpened delivers an incident.opened event to every subscribed endpoint.
func (n *Notifier) IncidentOpened(businessID string, inc *model.Incident) {
	payload, _ := json.Marshal(map[string]any{
		"event":         "incident.opened",
		"id":            inc.ID,
		"business_id":   businessID,
		"kind":          inc.Kind,
		"severity":      inc.Severity,
		"subject_id":    inc.SubjectID,
		"subject_label": inc.SubjectLabel,
		"detail":        inc.Detail,
		"opened_at":     inc.OpenedAt,
		"timestamp":     time.Now().UTC(),
	})
	n.dispatch(businessID, "incident.opened", payload)
}

// IncidentResolved delivers an incident.resolved event.
func (n *Notifier) IncidentResolved(businessID string, inc *model.Incident) {
	payload, _ := json.Marshal(map[string]any{
		"event":       "incident.resolved",
		"id":          inc.ID,
		"business_id": businessID,
		"kind":        inc.Kind,
		"subject_id":  inc.SubjectID,
		"resolved_at": inc.ResolvedAt,
		"timestamp":   time.Now().UTC(),
	})
	n.dispatch(businessID, "incident.resolved", payload)
}

// dispatch finds subscribed endpoints, records a delivery row, and POSTs.
// Its repo calls are tenant-bound (the worker has no owner session context).
func (n *Notifier) dispatch(businessID, eventType string, payload []byte) {
	endpoints, err := n.listEnabledForEvent(businessID, eventType)
	if err != nil {
		n.logger.Warnf("notifier: list endpoints for %s: %v", businessID, err)
		return
	}
	if len(endpoints) == 0 {
		return
	}

	for _, ep := range endpoints {
		delivery := &model.WebhookDelivery{
			WebhookID:  &ep.ID,
			BusinessID: businessID,
			EventType:  eventType,
			Payload:    payload,
			Status:     "pending",
		}
		created, err := n.createDelivery(delivery, businessID)
		if err != nil {
			n.logger.Warnf("notifier: record delivery for %s: %v", ep.ID, err)
			continue
		}

		// Async per endpoint: slow targets must not block other webhooks.
		// businessID is carried in so the async write is tenant-bound too.
		go n.attempt(ep, created, payload, businessID)
	}
}

func (n *Notifier) attempt(ep *model.WebhookEndpoint, delivery *model.WebhookDelivery, payload []byte, businessID string) {
	lastErr := ""
	statusCode := 0

	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(BackoffBetweenAttempts)
		}

		req, err := http.NewRequest(http.MethodPost, ep.URL, bytes.NewReader(payload))
		if err != nil {
			lastErr = err.Error()
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "MegaMagic-Notifier/1.0")
		req.Header.Set("X-MegaMagic-Event", delivery.EventType)

		if ep.Secret != nil && *ep.Secret != "" {
			mac := hmac.New(sha256.New, []byte(*ep.Secret))
			mac.Write(payload)
			req.Header.Set("X-MegaMagic-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		}

		resp, err := n.client.Do(req)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		statusCode = resp.StatusCode
		_ = resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			msg := ""
			if err := n.markDeliveryResult(businessID, delivery.ID, "delivered", resp.StatusCode, &msg); err != nil {
				n.logger.Warnf("notifier: mark delivered %s: %v", delivery.ID, err)
			}
			n.logger.Infof("notifier: delivered %s to %s (%d)", delivery.EventType, ep.URL, resp.StatusCode)
			return
		}

		lastErr = fmt.Sprintf("http %d", resp.StatusCode)
	}

	msg := lastErr
	if msg == "" {
		msg = fmt.Sprintf("http %d", statusCode)
	}
	if err := n.markDeliveryResult(businessID, delivery.ID, "failed", statusCode, &msg); err != nil {
		n.logger.Warnf("notifier: mark failed %s: %v", delivery.ID, err)
	}
	n.logger.Warnf("notifier: delivery %s to %s failed after %d attempts (last: %s)",
		delivery.EventType, ep.URL, MaxAttempts, lastErr)
}
