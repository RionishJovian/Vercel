package database

import (
	"database/sql"
	"errors"
	"mega-magic/internal/subscription"
	"time"
)

func SubscriptionStatusTXN(tx *sql.Tx, businessID, existingPolicy string, now time.Time) (subscription.Status, error) {
	var trial, paid sql.NullTime
	err := tx.QueryRow(`SELECT
	 (SELECT trial_expires_at FROM business_subscription_trials WHERE business_id=$1),
	 (SELECT max(activated_expires_at) FROM service_purchases
	 WHERE business_id=$1 AND status='approved' AND service_key IN ('monthly','annual'))`, businessID).Scan(&trial, &paid)
	if err != nil {
		return subscription.Status{}, err
	}
	var t, p *time.Time
	if trial.Valid {
		t = &trial.Time
	}
	if paid.Valid {
		p = &paid.Time
	}
	return subscription.Evaluate(now, t, p, existingPolicy), nil
}

func CheckSubscriptionSchema(db *DB) error {
	if db == nil {
		return errors.New("subscription database unavailable")
	}
	if _, err := db.Exec(`SELECT business_id, trial_started_at, trial_expires_at FROM business_subscription_trials LIMIT 0`); err != nil {
		return err
	}
	if _, err := db.Exec(`SELECT business_id,status,service_key,activated_expires_at FROM service_purchases LIMIT 0`); err != nil {
		return err
	}
	var ready bool
	err := db.QueryRow(`SELECT count(*)=2 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
	 WHERE n.nspname='public' AND c.relkind='r' AND c.relrowsecurity AND
	 (c.relname='service_purchases' OR (c.relname='business_subscription_trials' AND c.relforcerowsecurity))`).Scan(&ready)
	if err != nil {
		return err
	}
	if !ready {
		return errors.New("subscription tenant protection unavailable")
	}
	return nil
}
