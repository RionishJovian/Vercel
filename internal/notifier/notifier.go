package notifier

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

const (
	// MaxAttempts is how many times a delivery is tried before giving up.
	MaxAttempts = 3
	// BackoffBetweenAttempts spaces out retries.
	BackoffBetweenAttempts = 5 * time.Second
)

// Notifier fans incident events out to the tenant's webhook endpoints.
// It is synchronous for now (evaluator's caller is a NATS handler, so a
// brief delay is acceptable); a queued worker can replace it later.
type Notifier struct {
	webhookRepo *database.WebhookRepository
	client      *http.Client
	logger      *zap.SugaredLogger
}

func New(repo *database.WebhookRepository, logger *zap.SugaredLogger) *Notifier {
	return &Notifier{
		webhookRepo: repo,
		client:      &http.Client{Timeout: 10 * time.Second},
		logger:      logger,
	}
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
		"event":         "incident.resolved",
		"id":            inc.ID,
		"business_id":   businessID,
		"kind":          inc.Kind,
		"subject_id":    inc.SubjectID,
		"resolved_at":   inc.ResolvedAt,
		"timestamp":     time.Now().UTC(),
	})
	n.dispatch(businessID, "incident.resolved", payload)
}

// dispatch finds subscribed endpoints, records a delivery row, and POSTs.
func (n *Notifier) dispatch(businessID, eventType string, payload []byte) {
	endpoints, err := n.webhookRepo.ListEnabledForEvent(businessID, eventType)
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
		created, err := n.webhookRepo.CreateDelivery(delivery)
		if err != nil {
			n.logger.Warnf("notifier: record delivery for %s: %v", ep.ID, err)
			continue
		}

		// Async per endpoint: slow targets must not block other webhooks.
		go n.attempt(ep, created, payload)
	}
}

func (n *Notifier) attempt(ep *model.WebhookEndpoint, delivery *model.WebhookDelivery, payload []byte) {
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
			if err := n.webhookRepo.MarkDeliveryResult(delivery.ID, "delivered", resp.StatusCode, &msg); err != nil {
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
	if err := n.webhookRepo.MarkDeliveryResult(delivery.ID, "failed", statusCode, &msg); err != nil {
		n.logger.Warnf("notifier: mark failed %s: %v", delivery.ID, err)
	}
	n.logger.Warnf("notifier: delivery %s to %s failed after %d attempts (last: %s)",
		delivery.EventType, ep.URL, MaxAttempts, lastErr)
}