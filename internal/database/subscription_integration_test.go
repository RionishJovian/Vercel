package database

import (
	"database/sql"
	"os"
	"testing"
	"time"
)

func TestDB_SubscriptionTransitionTrials(t *testing.T) {
	db := requireHarness(t)
	storeSchema, e := os.ReadFile("../../migrations/20260930070000_store_transaction_core.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(storeSchema)); e != nil {
		t.Fatal(e)
	}
	biz, other := insertBusiness(t, db), insertBusiness(t, db)
	m, e := os.ReadFile("../../migrations/20261001160000_subscription_trials.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(m)); e != nil {
		t.Fatal(e)
	}
	if e = CheckSubscriptionSchema(db); e != nil {
		t.Fatal("subscription startup preflight", e)
	}
	var start, expiry time.Time
	if e = db.QueryRow(`SELECT trial_started_at,trial_expires_at FROM business_subscription_trials WHERE business_id=$1`, biz).Scan(&start, &expiry); e != nil {
		t.Fatal(e)
	}
	if expiry.Sub(start) != 336*time.Hour {
		t.Fatalf("duration %v", expiry.Sub(start))
	}
	if _, e = db.Exec(string(m)); e != nil {
		t.Fatal(e)
	}
	var repeated time.Time
	if e = db.QueryRow(`SELECT trial_expires_at FROM business_subscription_trials WHERE business_id=$1`, biz).Scan(&repeated); e != nil || !repeated.Equal(expiry) {
		t.Fatal("rerun changed trial", e)
	}
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		var count int
		if e := tx.QueryRow(`SELECT count(*) FROM business_subscription_trials WHERE business_id=$1`, other).Scan(&count); e != nil {
			return e
		}
		if count != 0 {
			t.Fatal("foreign trial visible")
		}
		s, e := SubscriptionStatusTXN(tx, biz, "require_payment", start)
		if e != nil {
			return e
		}
		if !s.Access || s.State != "trial" {
			t.Fatalf("trial %+v", s)
		}
		s, e = SubscriptionStatusTXN(tx, biz, "require_payment", expiry)
		if e != nil {
			return e
		}
		if s.Access {
			t.Fatal("exact expiry allowed")
		}
		return nil
	})
}
