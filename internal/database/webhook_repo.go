package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"

	"mega-magic/internal/model"
)

type WebhookRepository struct {
	db *DB
}

func NewWebhookRepository(db *DB) *WebhookRepository {
	return &WebhookRepository{db: db}
}

// --- Endpoints ---

func (r *WebhookRepository) CreateEndpoint(ep *model.WebhookEndpoint) (*model.WebhookEndpoint, error) {
	query := `
		INSERT INTO webhook_endpoints (business_id, url, secret, event_types, enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, business_id, url, secret, event_types, enabled, created_at, updated_at`
	e := &model.WebhookEndpoint{}
	var secret sql.NullString
	var events pq.StringArray
	err := r.db.QueryRow(query,
		ep.BusinessID, ep.URL, ep.Secret, pq.Array(ep.EventTypes), ep.Enabled,
	).Scan(&e.ID, &e.BusinessID, &e.URL, &secret, &events, &e.Enabled, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create webhook endpoint: %w", err)
	}
	if secret.Valid {
		s := secret.String
		e.Secret = &s
	}
	e.EventTypes = events
	return e, nil
}

func (r *WebhookRepository) ListEndpoints(businessID string) ([]*model.WebhookEndpoint, error) {
	rows, err := r.db.Query(
		`SELECT id, business_id, url, secret, event_types, enabled, created_at, updated_at
		 FROM webhook_endpoints WHERE business_id = $1 ORDER BY created_at DESC`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list webhook endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []*model.WebhookEndpoint
	for rows.Next() {
		e := &model.WebhookEndpoint{}
		var secret sql.NullString
		var events pq.StringArray
		if err := rows.Scan(&e.ID, &e.BusinessID, &e.URL, &secret, &events, &e.Enabled, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan webhook endpoint: %w", err)
		}
		if secret.Valid {
			s := secret.String
			e.Secret = &s
		}
		e.EventTypes = events
		endpoints = append(endpoints, e)
	}
	return endpoints, rows.Err()
}

func (r *WebhookRepository) GetEndpointByID(id string) (*model.WebhookEndpoint, error) {
	e := &model.WebhookEndpoint{}
	var secret sql.NullString
	var events pq.StringArray
	err := r.db.QueryRow(
		`SELECT id, business_id, url, secret, event_types, enabled, created_at, updated_at
		 FROM webhook_endpoints WHERE id = $1`, id,
	).Scan(&e.ID, &e.BusinessID, &e.URL, &secret, &events, &e.Enabled, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get webhook endpoint: %w", err)
	}
	if secret.Valid {
		s := secret.String
		e.Secret = &s
	}
	e.EventTypes = events
	return e, nil
}

func (r *WebhookRepository) UpdateEndpoint(ep *model.WebhookEndpoint) error {
	result, err := r.db.Exec(
		`UPDATE webhook_endpoints SET url=$1, secret=$2, event_types=$3, enabled=$4, updated_at=$5 WHERE id=$6`,
		ep.URL, ep.Secret, pq.Array(ep.EventTypes), ep.Enabled, time.Now().UTC(), ep.ID,
	)
	if err != nil {
		return fmt.Errorf("update webhook endpoint: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("webhook endpoint not found: %s", ep.ID)
	}
	return nil
}

func (r *WebhookRepository) DeleteEndpoint(id string) error {
	result, err := r.db.Exec(`DELETE FROM webhook_endpoints WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete webhook endpoint: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("webhook endpoint not found: %s", id)
	}
	return nil
}

// ListEnabledForEvent returns enabled endpoints for a business subscribed to an event type.
func (r *WebhookRepository) ListEnabledForEvent(businessID, eventType string) ([]*model.WebhookEndpoint, error) {
	rows, err := r.db.Query(
		`SELECT id, business_id, url, secret, event_types, enabled, created_at, updated_at
		 FROM webhook_endpoints
		 WHERE business_id = $1 AND enabled = true AND $2 = ANY(event_types)
		 ORDER BY created_at ASC`, businessID, eventType)
	if err != nil {
		return nil, fmt.Errorf("list enabled webhook endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []*model.WebhookEndpoint
	for rows.Next() {
		e := &model.WebhookEndpoint{}
		var secret sql.NullString
		var events pq.StringArray
		if err := rows.Scan(&e.ID, &e.BusinessID, &e.URL, &secret, &events, &e.Enabled, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan webhook endpoint: %w", err)
		}
		if secret.Valid {
			s := secret.String
			e.Secret = &s
		}
		e.EventTypes = events
		endpoints = append(endpoints, e)
	}
	return endpoints, rows.Err()
}

// --- Deliveries ---

func (r *WebhookRepository) CreateDelivery(d *model.WebhookDelivery) (*model.WebhookDelivery, error) {
	payload := string(d.Payload)
	query := `
		INSERT INTO webhook_deliveries (webhook_id, business_id, event_type, payload, status, attempts)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6)
		RETURNING id, webhook_id, business_id, event_type, payload, status, attempts, last_status_code, last_error, created_at, delivered_at`
	dd := &model.WebhookDelivery{}
	var webhookID sql.NullString
	var statusCode sql.NullInt64
	var lastErr sql.NullString
	var deliveredAt sql.NullTime
	err := r.db.QueryRow(query,
		d.WebhookID, d.BusinessID, d.EventType, payload, d.Status, d.Attempts,
	).Scan(&dd.ID, &webhookID, &dd.BusinessID, &dd.EventType, &payload, &dd.Status, &dd.Attempts,
		&statusCode, &lastErr, &dd.CreatedAt, &deliveredAt)
	if err != nil {
		return nil, fmt.Errorf("create webhook delivery: %w", err)
	}
	dd.Payload = []byte(payload)
	if webhookID.Valid {
		s := webhookID.String
		dd.WebhookID = &s
	}
	if statusCode.Valid {
		n := int(statusCode.Int64)
		dd.LastStatusCode = &n
	}
	if lastErr.Valid {
		s := lastErr.String
		dd.LastError = &s
	}
	if deliveredAt.Valid {
		t := deliveredAt.Time
		dd.DeliveredAt = &t
	}
	return dd, nil
}

func (r *WebhookRepository) MarkDeliveryResult(id string, status string, statusCode int, errMsg *string) error {
	_, err := r.db.Exec(
		`UPDATE webhook_deliveries
		 SET status=$2, attempts=attempts+1, last_status_code=$3, last_error=$4,
		     delivered_at=CASE WHEN $2='delivered' THEN now() ELSE delivered_at END
		 WHERE id=$1`,
		id, status, statusCode, errMsg,
	)
	if err != nil {
		return fmt.Errorf("update webhook delivery: %w", err)
	}
	return nil
}

func (r *WebhookRepository) ListDeliveries(businessID string, limit int) ([]*model.WebhookDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.Query(
		`SELECT id, webhook_id, business_id, event_type, payload, status, attempts,
		        last_status_code, last_error, created_at, delivered_at
		 FROM webhook_deliveries WHERE business_id = $1
		 ORDER BY created_at DESC LIMIT $2`, businessID, limit)
	if err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []*model.WebhookDelivery
	for rows.Next() {
		d := &model.WebhookDelivery{}
		var webhookID sql.NullString
		var payload []byte
		var statusCode sql.NullInt64
		var lastErr sql.NullString
		var deliveredAt sql.NullTime
		if err := rows.Scan(&d.ID, &webhookID, &d.BusinessID, &d.EventType, &payload, &d.Status, &d.Attempts,
			&statusCode, &lastErr, &d.CreatedAt, &deliveredAt); err != nil {
			return nil, fmt.Errorf("scan webhook delivery: %w", err)
		}
		d.Payload = payload
		if webhookID.Valid {
			s := webhookID.String
			d.WebhookID = &s
		}
		if statusCode.Valid {
			n := int(statusCode.Int64)
			d.LastStatusCode = &n
		}
		if lastErr.Valid {
			s := lastErr.String
			d.LastError = &s
		}
		if deliveredAt.Valid {
			t := deliveredAt.Time
			d.DeliveredAt = &t
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}

// ---------------------------------------------------------------------------
// Transaction-scoped variants (RLS enforcement rollout: webhooks). Same SQL on
// the caller's *sql.Tx so they can run under WithTenant (owner handlers AND the
// notifier worker). Bare versions stay for the later full rollout.
// ---------------------------------------------------------------------------

// CreateEndpointTXN is the transaction-scoped variant of CreateEndpoint.
func (r *WebhookRepository) CreateEndpointTXN(tx *sql.Tx, ep *model.WebhookEndpoint) (*model.WebhookEndpoint, error) {
	query := `
		INSERT INTO webhook_endpoints (business_id, url, secret, event_types, enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, business_id, url, secret, event_types, enabled, created_at, updated_at`
	e := &model.WebhookEndpoint{}
	var secret sql.NullString
	var events pq.StringArray
	err := tx.QueryRow(query,
		ep.BusinessID, ep.URL, ep.Secret, pq.Array(ep.EventTypes), ep.Enabled,
	).Scan(&e.ID, &e.BusinessID, &e.URL, &secret, &events, &e.Enabled, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create webhook endpoint: %w", err)
	}
	if secret.Valid {
		s := secret.String
		e.Secret = &s
	}
	e.EventTypes = events
	return e, nil
}

// ListEndpointsTXN is the transaction-scoped variant of ListEndpoints.
func (r *WebhookRepository) ListEndpointsTXN(tx *sql.Tx, businessID string) ([]*model.WebhookEndpoint, error) {
	rows, err := tx.Query(
		`SELECT id, business_id, url, secret, event_types, enabled, created_at, updated_at
		 FROM webhook_endpoints WHERE business_id = $1 ORDER BY created_at DESC`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list webhook endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []*model.WebhookEndpoint
	for rows.Next() {
		e := &model.WebhookEndpoint{}
		var secret sql.NullString
		var events pq.StringArray
		if err := rows.Scan(&e.ID, &e.BusinessID, &e.URL, &secret, &events, &e.Enabled, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan webhook endpoint: %w", err)
		}
		if secret.Valid {
			s := secret.String
			e.Secret = &s
		}
		e.EventTypes = events
		endpoints = append(endpoints, e)
	}
	return endpoints, rows.Err()
}

// GetEndpointByIDTXN is the transaction-scoped variant of GetEndpointByID.
func (r *WebhookRepository) GetEndpointByIDTXN(tx *sql.Tx, id string) (*model.WebhookEndpoint, error) {
	e := &model.WebhookEndpoint{}
	var secret sql.NullString
	var events pq.StringArray
	err := tx.QueryRow(
		`SELECT id, business_id, url, secret, event_types, enabled, created_at, updated_at
		 FROM webhook_endpoints WHERE id = $1`, id,
	).Scan(&e.ID, &e.BusinessID, &e.URL, &secret, &events, &e.Enabled, &e.CreatedAt, &e.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get webhook endpoint: %w", err)
	}
	if secret.Valid {
		s := secret.String
		e.Secret = &s
	}
	e.EventTypes = events
	return e, nil
}

// UpdateEndpointTXN is the transaction-scoped variant of UpdateEndpoint.
func (r *WebhookRepository) UpdateEndpointTXN(tx *sql.Tx, ep *model.WebhookEndpoint) error {
	result, err := tx.Exec(
		`UPDATE webhook_endpoints SET url=$1, secret=$2, event_types=$3, enabled=$4, updated_at=$5 WHERE id=$6`,
		ep.URL, ep.Secret, pq.Array(ep.EventTypes), ep.Enabled, time.Now().UTC(), ep.ID,
	)
	if err != nil {
		return fmt.Errorf("update webhook endpoint: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("webhook endpoint not found: %s", ep.ID)
	}
	return nil
}

// DeleteEndpointTXN is the transaction-scoped variant of DeleteEndpoint.
func (r *WebhookRepository) DeleteEndpointTXN(tx *sql.Tx, id string) error {
	result, err := tx.Exec(`DELETE FROM webhook_endpoints WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete webhook endpoint: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return fmt.Errorf("webhook endpoint not found: %s", id)
	}
	return nil
}

// ListEnabledForEventTXN is the transaction-scoped variant of
// ListEnabledForEvent (used by the notifier worker inside a tenant tx).
func (r *WebhookRepository) ListEnabledForEventTXN(tx *sql.Tx, businessID, eventType string) ([]*model.WebhookEndpoint, error) {
	rows, err := tx.Query(
		`SELECT id, business_id, url, secret, event_types, enabled, created_at, updated_at
		 FROM webhook_endpoints
		 WHERE business_id = $1 AND enabled = true AND $2 = ANY(event_types)
		 ORDER BY created_at ASC`, businessID, eventType)
	if err != nil {
		return nil, fmt.Errorf("list enabled webhook endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []*model.WebhookEndpoint
	for rows.Next() {
		e := &model.WebhookEndpoint{}
		var secret sql.NullString
		var events pq.StringArray
		if err := rows.Scan(&e.ID, &e.BusinessID, &e.URL, &secret, &events, &e.Enabled, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan webhook endpoint: %w", err)
		}
		if secret.Valid {
			s := secret.String
			e.Secret = &s
		}
		e.EventTypes = events
		endpoints = append(endpoints, e)
	}
	return endpoints, rows.Err()
}

// CreateDeliveryTXN is the transaction-scoped variant of CreateDelivery.
func (r *WebhookRepository) CreateDeliveryTXN(tx *sql.Tx, d *model.WebhookDelivery) (*model.WebhookDelivery, error) {
	payload := string(d.Payload)
	query := `
		INSERT INTO webhook_deliveries (webhook_id, business_id, event_type, payload, status, attempts)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6)
		RETURNING id, webhook_id, business_id, event_type, payload, status, attempts, last_status_code, last_error, created_at, delivered_at`
	dd := &model.WebhookDelivery{}
	var webhookID sql.NullString
	var statusCode sql.NullInt64
	var lastErr sql.NullString
	var deliveredAt sql.NullTime
	err := tx.QueryRow(query,
		d.WebhookID, d.BusinessID, d.EventType, payload, d.Status, d.Attempts,
	).Scan(&dd.ID, &webhookID, &dd.BusinessID, &dd.EventType, &payload, &dd.Status, &dd.Attempts,
		&statusCode, &lastErr, &dd.CreatedAt, &deliveredAt)
	if err != nil {
		return nil, fmt.Errorf("create webhook delivery: %w", err)
	}
	dd.Payload = []byte(payload)
	if webhookID.Valid {
		s := webhookID.String
		dd.WebhookID = &s
	}
	if statusCode.Valid {
		n := int(statusCode.Int64)
		dd.LastStatusCode = &n
	}
	if lastErr.Valid {
		s := lastErr.String
		dd.LastError = &s
	}
	if deliveredAt.Valid {
		t := deliveredAt.Time
		dd.DeliveredAt = &t
	}
	return dd, nil
}

// MarkDeliveryResultTXN is the transaction-scoped variant of MarkDeliveryResult.
func (r *WebhookRepository) MarkDeliveryResultTXN(tx *sql.Tx, id string, status string, statusCode int, errMsg *string) error {
	_, err := tx.Exec(
		`UPDATE webhook_deliveries
		 SET status=$2, attempts=attempts+1, last_status_code=$3, last_error=$4,
		     delivered_at=CASE WHEN $2='delivered' THEN now() ELSE delivered_at END
		 WHERE id=$1`,
		id, status, statusCode, errMsg,
	)
	if err != nil {
		return fmt.Errorf("update webhook delivery: %w", err)
	}
	return nil
}

// ListDeliveriesTXN is the transaction-scoped variant of ListDeliveries.
func (r *WebhookRepository) ListDeliveriesTXN(tx *sql.Tx, businessID string, limit int) ([]*model.WebhookDelivery, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := tx.Query(
		`SELECT id, webhook_id, business_id, event_type, payload, status, attempts,
		        last_status_code, last_error, created_at, delivered_at
		 FROM webhook_deliveries WHERE business_id = $1
		 ORDER BY created_at DESC LIMIT $2`, businessID, limit)
	if err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []*model.WebhookDelivery
	for rows.Next() {
		d := &model.WebhookDelivery{}
		var webhookID sql.NullString
		var payload []byte
		var statusCode sql.NullInt64
		var lastErr sql.NullString
		var deliveredAt sql.NullTime
		if err := rows.Scan(&d.ID, &webhookID, &d.BusinessID, &d.EventType, &payload, &d.Status, &d.Attempts,
			&statusCode, &lastErr, &d.CreatedAt, &deliveredAt); err != nil {
			return nil, fmt.Errorf("scan webhook delivery: %w", err)
		}
		d.Payload = payload
		if webhookID.Valid {
			s := webhookID.String
			d.WebhookID = &s
		}
		if statusCode.Valid {
			n := int(statusCode.Int64)
			d.LastStatusCode = &n
		}
		if lastErr.Valid {
			s := lastErr.String
			d.LastError = &s
		}
		if deliveredAt.Valid {
			t := deliveredAt.Time
			d.DeliveredAt = &t
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, rows.Err()
}
