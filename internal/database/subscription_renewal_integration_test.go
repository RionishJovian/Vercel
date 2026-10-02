package database

import (
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"mega-magic/internal/model"
)

func TestDB_SubscriptionBusinessRenewal(t *testing.T) {
	db := requireHarness(t)
	for _, path := range []string{"../../migrations/20260930070000_store_transaction_core.sql", "../../migrations/20261001160000_subscription_trials.sql"} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(b)); err != nil {
			t.Fatal(err)
		}
	}
	biz, other := insertBusiness(t, db), insertBusiness(t, db)
	u1, u2 := insertUser(t, db, biz), insertUser(t, db, biz)
	start := time.Now().UTC().Truncate(time.Microsecond)
	trialEnd := start.Add(336 * time.Hour)
	if _, err := db.Exec(`INSERT INTO business_subscription_trials(business_id,trial_started_at,trial_expires_at) VALUES($1,$2,$3)`, biz, start, trialEnd); err != nil {
		t.Fatal(err)
	}
	var bank string
	if err := db.QueryRow(`INSERT INTO platform_service_bank_accounts(slot,holder_name,bank_name,account_number)
	 VALUES(1,'Harness','Test','TEST') ON CONFLICT(slot) DO UPDATE SET enabled=true RETURNING id`).Scan(&bank); err != nil {
		t.Fatal(err)
	}
	create := func(user, product string) string {
		var id string
		if err := db.QueryRow(`INSERT INTO service_purchases(business_id,user_id,service_key,service_label,price_mmk,payment_method,bank_account_id,bank_reference,idempotency_key)
		 VALUES($1,$2,$3,'Test subscription',1000,'bank_transfer',$4,$5,$6) RETURNING id`, biz, user, product, bank, randHex(10), randHex(10)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	monthly, annual := create(u1, "monthly"), create(u2, "annual")
	repo := NewStorePurchaseRepository(db)
	decide := func(tenant, id string) (*model.StorePurchase, error) {
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		var p *model.StorePurchase
		err = WithTenant(tx, tenant, func() error {
			var e error
			p, e = repo.DecideSubscriptionTXN(tx, tenant, id, "", true, "", time.Now().UTC())
			return e
		})
		if err != nil {
			return nil, err
		}
		return p, tx.Commit()
	}
	if _, err := decide(other, monthly); !errors.Is(err, ErrStorePurchaseNotFound) {
		t.Fatalf("foreign purchase: %v", err)
	}
	// Different purchasers approving concurrently must add both periods, regardless
	// of which transaction wins. Each must see the other's committed ledger entry.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	gate := make(chan struct{})
	for _, id := range []string{monthly, annual} {
		wg.Add(1)
		go func(id string) { defer wg.Done(); <-gate; _, err := decide(biz, id); results <- err }(id)
	}
	close(gate)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := trialEnd.Add(395 * 24 * time.Hour)
	var got time.Time
	if err := db.QueryRow(`SELECT max(activated_expires_at) FROM service_purchases WHERE business_id=$1`, biz).Scan(&got); err != nil || !got.Equal(want) {
		t.Fatalf("renewal got %v want %v: %v", got, want, err)
	}
	if _, err := decide(biz, monthly); err == nil {
		t.Fatal("duplicate approval extended subscription")
	}
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		s, err := SubscriptionStatusTXN(tx, biz, "require_payment", want)
		if err == nil && s.Access {
			t.Fatal("paid access allowed at exact expiry")
		}
		return err
	})
	var audits int
	if err := db.QueryRow(`SELECT count(*) FROM service_purchase_audit WHERE business_id=$1 AND action='approved_and_activated'`, biz).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("audit count %d: %v", audits, err)
	}
	// Expired history must restart from database time, not the old deadline or
	// an unrelated legacy account pass. A rejected payment adds no time.
	if _, err := db.Exec(`UPDATE service_purchases SET activated_expires_at=clock_timestamp()-interval '1 day' WHERE business_id=$1`, biz); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE business_subscription_trials SET trial_started_at=now()-interval '360 hours',trial_expires_at=now()-interval '24 hours' WHERE business_id=$1`, biz); err != nil {
		t.Fatal(err)
	}
	pending := create(u1, "monthly")
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		_, err := repo.DecideSubscriptionTXN(tx, biz, pending, "", false, "Rejected test", time.Now().UTC())
		return err
	})
	var rejectedExpiry sql.NullTime
	if err := db.QueryRow(`SELECT activated_expires_at FROM service_purchases WHERE id=$1`, pending).Scan(&rejectedExpiry); err != nil || rejectedExpiry.Valid {
		t.Fatal("rejection activated access", err)
	}
	if _, err := db.Exec(`UPDATE account_entitlements SET tier_expires_at=clock_timestamp()+interval '1000 days' WHERE user_id=$1`, u1); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC()
	resumed, err := decide(biz, create(u1, "monthly"))
	after := time.Now().UTC()
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ActivatedExpiresAt == nil || resumed.ActivatedExpiresAt.Before(before.Add(30*24*time.Hour-time.Minute)) || resumed.ActivatedExpiresAt.After(after.Add(30*24*time.Hour+time.Minute)) {
		t.Fatalf("expired subscription revived stale/legacy time: %+v", resumed.ActivatedExpiresAt)
	}
	foreignPurchase := create(insertUser(t, db, other), "annual")
	if _, err := decide(biz, foreignPurchase); err == nil {
		t.Fatal("foreign purchaser activated tenant entitlement")
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM service_purchases WHERE id=$1`, foreignPurchase).Scan(&status); err != nil || status != "pending" {
		t.Fatal("failed activation was not rolled back", status, err)
	}
}
