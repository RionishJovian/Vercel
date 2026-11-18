package database

import (
	"context"
	"time"
)

// WebFigAddonKey is the store add-on that pays for cloud WebFig.
const WebFigAddonKey = "webfig_access"

// WebFigAddonActive reports whether the business has a paid, unexpired WebFig add-on.
//
// Each approved request buys one period (monthly, annual) starting at its approval, or at the
// end of the coverage already paid for, so renewing early extends rather than overlaps. Access
// lasts through the end of the UTC day on which the paid period ends and stops the next day.
// A one_time request never expires. Nothing approved means not active.
func WebFigAddonActive(ctx context.Context, db *DB, businessID string, now time.Time) (bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT r.decided_at, r.period FROM store_addon_requests r
		JOIN store_addons a ON a.id = r.addon_id
		WHERE r.business_id = $1 AND r.status = 'approved' AND a.addon_key = $2 AND r.decided_at IS NOT NULL
		ORDER BY r.decided_at`, businessID, WebFigAddonKey)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	var decided []time.Time
	var periods []string
	for rows.Next() {
		var at time.Time
		var period string
		if err := rows.Scan(&at, &period); err != nil {
			return false, err
		}
		decided, periods = append(decided, at), append(periods, period)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return webFigCoveredUntil(decided, periods, now), nil
}

// webFigCoveredUntil is the pure part of WebFigAddonActive: requests in approval order.
func webFigCoveredUntil(decided []time.Time, periods []string, now time.Time) bool {
	var end time.Time
	for i, at := range decided {
		at = at.UTC()
		switch periods[i] {
		case "one_time":
			return true
		case "annual":
			end = laterOf(end, at).AddDate(1, 0, 0)
		default: // monthly
			end = laterOf(end, at).AddDate(0, 1, 0)
		}
	}
	if end.IsZero() {
		return false
	}
	// Through the end of the last paid day, so it stops the next day.
	endOfDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	return now.Before(endOfDay)
}

func laterOf(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
