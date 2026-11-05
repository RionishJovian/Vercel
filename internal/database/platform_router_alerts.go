package database

import (
	"fmt"
	"time"
)

// Kinds of platform router alert.
const (
	RouterAlertOffline   = "offline"
	RouterAlertRecovered = "recovered"
)

// RouterAlert tells the platform owner a connected router dropped or came back.
type RouterAlert struct {
	ID           string     `json:"id"`
	BusinessID   string     `json:"business_id"`
	DeviceID     string     `json:"device_id"`
	Kind         string     `json:"kind"`
	OfflineSince time.Time  `json:"offline_since"`
	Detail       string     `json:"detail"`
	HardwareName string     `json:"hardware_name"`
	CompanyName  string     `json:"company_name"`
	CreatedAt    time.Time  `json:"created_at"`
	ReadAt       *time.Time `json:"read_at,omitempty"`
}

// DueRouterAlerts finds alerts that should exist but don't yet:
//   - offline: the router's latest state is offline, it was up before that, and it
//     has stayed down for at least grace. A router first seen offline never alerts.
//   - recovered: the router is back and its last drop did raise an alert.
//
// Never reachable from a tenant request.
func (r *DeviceRepository) DueRouterAlerts(grace time.Duration) ([]*RouterAlert, error) {
	rows, err := r.db.Query(`
WITH events AS (
  SELECT device_id, business_id, action, reason, created_at,
         lag(action) OVER w AS prev_action,
         lag(created_at) OVER w AS prev_at,
         lag(reason) OVER w AS prev_reason,
         row_number() OVER (PARTITION BY device_id ORDER BY created_at DESC) AS newest
    FROM device_change_events
   WHERE action IN ($1, $2)
  WINDOW w AS (PARTITION BY device_id ORDER BY created_at)
), due AS (
  SELECT 'offline' AS kind, e.device_id, e.business_id, e.created_at AS offline_since, e.reason AS detail
    FROM events e
   WHERE e.newest = 1 AND e.action = $2 AND e.prev_action = $1
     AND e.created_at <= now() - make_interval(secs => $3)
     AND NOT EXISTS (SELECT 1 FROM platform_router_alerts a
                      WHERE a.device_id = e.device_id AND a.kind = 'offline' AND a.offline_since = e.created_at)
  UNION ALL
  SELECT 'recovered', e.device_id, e.business_id, e.prev_at, e.prev_reason
    FROM events e
   WHERE e.newest = 1 AND e.action = $1 AND e.prev_action = $2
     AND EXISTS (SELECT 1 FROM platform_router_alerts a
                  WHERE a.device_id = e.device_id AND a.kind = 'offline' AND a.offline_since = e.prev_at)
     AND NOT EXISTS (SELECT 1 FROM platform_router_alerts a
                      WHERE a.device_id = e.device_id AND a.kind = 'recovered' AND a.offline_since = e.prev_at)
)
SELECT due.kind, due.device_id, due.business_id, due.offline_since, due.detail, d.hardware_name, b.company_name
  FROM due
  JOIN devices d ON d.device_id = due.device_id
  JOIN businesses b ON b.business_id = due.business_id
 ORDER BY due.offline_since`, ActionConnectionUp, ActionConnectionOffline, grace.Seconds())
	if err != nil {
		return nil, fmt.Errorf("due router alerts: %w", err)
	}
	defer rows.Close()
	var out []*RouterAlert
	for rows.Next() {
		a := &RouterAlert{}
		if err := rows.Scan(&a.Kind, &a.DeviceID, &a.BusinessID, &a.OfflineSince, &a.Detail, &a.HardwareName, &a.CompanyName); err != nil {
			return nil, fmt.Errorf("scan due router alert: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateRouterAlert stores an alert. It reports false when it already existed, so a
// retry or a second worker can't announce the same drop twice.
func (r *DeviceRepository) CreateRouterAlert(a *RouterAlert) (bool, error) {
	res, err := r.db.Exec(`INSERT INTO platform_router_alerts (business_id, device_id, kind, offline_since, detail)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (device_id, kind, offline_since) DO NOTHING`,
		a.BusinessID, a.DeviceID, a.Kind, a.OfflineSince, a.Detail)
	if err != nil {
		return false, fmt.Errorf("create router alert: %w", err)
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// ListRouterAlerts returns the newest alerts and how many are unread.
func (r *DeviceRepository) ListRouterAlerts(limit int) ([]*RouterAlert, int, error) {
	rows, err := r.db.Query(`SELECT a.id, a.business_id, a.device_id, a.kind, a.offline_since, a.detail, d.hardware_name, b.company_name, a.created_at, a.read_at
		  FROM platform_router_alerts a
		  JOIN devices d ON d.device_id = a.device_id
		  JOIN businesses b ON b.business_id = a.business_id
		 ORDER BY a.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list router alerts: %w", err)
	}
	defer rows.Close()
	out := []*RouterAlert{}
	for rows.Next() {
		a := &RouterAlert{}
		if err := rows.Scan(&a.ID, &a.BusinessID, &a.DeviceID, &a.Kind, &a.OfflineSince, &a.Detail, &a.HardwareName, &a.CompanyName, &a.CreatedAt, &a.ReadAt); err != nil {
			return nil, 0, fmt.Errorf("scan router alert: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var unread int
	if err := r.db.QueryRow(`SELECT count(*) FROM platform_router_alerts WHERE read_at IS NULL`).Scan(&unread); err != nil {
		return nil, 0, fmt.Errorf("count router alerts: %w", err)
	}
	return out, unread, nil
}

// MarkRouterAlertsRead marks every alert read.
func (r *DeviceRepository) MarkRouterAlertsRead() error {
	if _, err := r.db.Exec(`UPDATE platform_router_alerts SET read_at = now() WHERE read_at IS NULL`); err != nil {
		return fmt.Errorf("mark router alerts read: %w", err)
	}
	return nil
}
