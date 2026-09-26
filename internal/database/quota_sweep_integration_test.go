package database

import (
	"database/sql"
	"testing"
	"time"

	"mega-magic/internal/model"
)

// TestDB_QuotaSweep_listAllQuotas proves the cross-tenant enumeration the quota
// enforcement sweep relies on: ListAllQuotas() sees EVERY tenant's pass expiry
// (system-level, like ListAllPeerKeys), and a PAST expiry is distinguishable
// from a future/nil one.
func TestDB_QuotaSweep_listAllQuotas(t *testing.T) {
	db := requireHarness(t)
	qrepo := NewBusinessMeteredQuotaRepository(db)

	bizPast := insertBusiness(t, db)
	bizFuture := insertBusiness(t, db)
	bizNil := insertBusiness(t, db)

	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	inTenant(t, db, bizPast, func(tx *sql.Tx) error {
		return qrepo.UpsertTXN(tx, &model.BusinessMeteredQuota{BusinessID: bizPast, WebfigPassExpiresAt: &past})
	})
	inTenant(t, db, bizFuture, func(tx *sql.Tx) error {
		return qrepo.UpsertTXN(tx, &model.BusinessMeteredQuota{BusinessID: bizFuture, WebfigPassExpiresAt: &future})
	})
	inTenant(t, db, bizNil, func(tx *sql.Tx) error {
		return qrepo.UpsertTXN(tx, &model.BusinessMeteredQuota{BusinessID: bizNil})
	})

	rows, err := qrepo.ListAllQuotas()
	if err != nil {
		t.Fatalf("ListAllQuotas: %v", err)
	}

	seen := map[string]*time.Time{}
	for _, r := range rows {
		seen[r.BusinessID] = r.ExpiresAt
	}

	// Cross-tenant: all three businesses are visible.
	for _, b := range []string{bizPast, bizFuture, bizNil} {
		if _, ok := seen[b]; !ok {
			t.Fatalf("ListAllQuotas must be cross-tenant; missing business %s", b)
		}
	}

	// PAST expiry is detected (the sweep's trigger).
	if seen[bizPast] == nil || !seen[bizPast].Before(now) {
		t.Fatalf("past expiry not detected for %s: %v", bizPast, seen[bizPast])
	}
	// FUTURE expiry is after now (must NOT emit).
	if seen[bizFuture] == nil || !seen[bizFuture].After(now) {
		t.Fatalf("future expiry wrong for %s: %v", bizFuture, seen[bizFuture])
	}
	// NIL expiry stays nil (must NOT emit).
	if seen[bizNil] != nil {
		t.Fatalf("nil expiry must stay nil for %s, got %v", bizNil, seen[bizNil])
	}
}
