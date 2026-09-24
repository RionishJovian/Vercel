package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"

	"mega-magic/internal/model"
)

type AlertPolicyRepository struct {
	db *DB
}

func NewAlertPolicyRepository(db *DB) *AlertPolicyRepository {
	return &AlertPolicyRepository{db: db}
}

func (r *AlertPolicyRepository) Create(policy *model.AlertPolicy) (*model.AlertPolicy, error) {
	query := `
		INSERT INTO alert_policies (business_id, kind, enabled, severity, threshold, cooldown_minutes, notification_channels)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, business_id, kind, enabled, severity, threshold, cooldown_minutes, notification_channels, created_at, updated_at`
	p := &model.AlertPolicy{}
	var channels pq.StringArray
	err := r.db.QueryRow(query,
		policy.BusinessID, policy.Kind, policy.Enabled, policy.Severity,
		policy.Threshold, policy.CooldownMinutes, policy.NotificationChannels,
	).Scan(
		&p.ID, &p.BusinessID, &p.Kind, &p.Enabled, &p.Severity,
		&p.Threshold, &p.CooldownMinutes, &channels, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create alert policy: %w", err)
	}
	p.NotificationChannels = channels
	return p, nil
}

func (r *AlertPolicyRepository) GetByID(id string) (*model.AlertPolicy, error) {
	return scanAlertPolicy(r.db.QueryRow(
		`SELECT id, business_id, kind, enabled, severity, threshold, cooldown_minutes, notification_channels, created_at, updated_at FROM alert_policies WHERE id = $1`, id))
}

func (r *AlertPolicyRepository) ListByBusiness(businessID string) ([]*model.AlertPolicy, error) {
	rows, err := r.db.Query(
		`SELECT id, business_id, kind, enabled, severity, threshold, cooldown_minutes, notification_channels, created_at, updated_at FROM alert_policies WHERE business_id = $1 ORDER BY created_at DESC`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list alert policies: %w", err)
	}
	defer rows.Close()
	return scanAlertPolicies(rows)
}

func (r *AlertPolicyRepository) ListEnabled(businessID string) ([]*model.AlertPolicy, error) {
	rows, err := r.db.Query(
		`SELECT id, business_id, kind, enabled, severity, threshold, cooldown_minutes, notification_channels, created_at, updated_at FROM alert_policies WHERE business_id = $1 AND enabled = true ORDER BY created_at DESC`, businessID)
	if err != nil {
		return nil, fmt.Errorf("list enabled alert policies: %w", err)
	}
	defer rows.Close()
	return scanAlertPolicies(rows)
}

func (r *AlertPolicyRepository) Update(policy *model.AlertPolicy) error {
	result, err := r.db.Exec(
		`UPDATE alert_policies SET kind=$1, enabled=$2, severity=$3, threshold=$4, cooldown_minutes=$5, notification_channels=$6, updated_at=$7 WHERE id=$8`,
		policy.Kind, policy.Enabled, policy.Severity, policy.Threshold,
		policy.CooldownMinutes, policy.NotificationChannels, time.Now().UTC(), policy.ID)
	if err != nil {
		return fmt.Errorf("update alert policy: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("alert policy not found: %s", policy.ID)
	}
	return nil
}

func (r *AlertPolicyRepository) Delete(id string) error {
	result, err := r.db.Exec(`DELETE FROM alert_policies WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete alert policy: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("alert policy not found: %s", id)
	}
	return nil
}

func scanAlertPolicy(row interface{ Scan(...interface{}) error }) (*model.AlertPolicy, error) {
	p := &model.AlertPolicy{}
	var channels pq.StringArray
	err := row.Scan(&p.ID, &p.BusinessID, &p.Kind, &p.Enabled, &p.Severity,
		&p.Threshold, &p.CooldownMinutes, &channels, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan alert policy: %w", err)
	}
	p.NotificationChannels = channels
	return p, nil
}

func scanAlertPolicies(rows *sql.Rows) ([]*model.AlertPolicy, error) {
	var policies []*model.AlertPolicy
	for rows.Next() {
		p, err := scanAlertPolicy(rows)
		if err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate alert policies: %w", err)
	}
	return policies, nil
}