package database

import (
	"database/sql"
	"testing"
	"time"

	"mega-magic/internal/model"
)

// TestDB_BillingSchema_applied proves the hardened schema: purchase_requests
// exists with RLS enabled + a policy, duration_minutes is present, and the
// per-business bank-reference uniqueness index exists; portal_configs has the
// bank columns.
func TestDB_BillingSchema_applied(t *testing.T) {
	db := requireHarness(t)

	var exists bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='purchase_requests')`).Scan(&exists); err != nil {
		t.Fatalf("table check: %v", err)
	}
	if !exists {
		t.Fatal("init.sql must declare purchase_requests")
	}

	var rls bool
	if err := db.QueryRow(`SELECT relrowsecurity FROM pg_class WHERE relname='purchase_requests'`).Scan(&rls); err != nil {
		t.Fatalf("rowsecurity check: %v", err)
	}
	if !rls {
		t.Fatal("RLS must be enabled on purchase_requests")
	}

	var pol int
	if err := db.QueryRow(`SELECT count(*) FROM pg_policies WHERE schemaname='public' AND tablename='purchase_requests'`).Scan(&pol); err != nil {
		t.Fatalf("policy check: %v", err)
	}
	if pol < 1 {
		t.Fatal("purchase_requests needs an RLS policy")
	}

	var uniq int
	if err := db.QueryRow(`SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname='idx_purchase_requests_bank_ref_unique'`).Scan(&uniq); err != nil {
		t.Fatalf("unique index check: %v", err)
	}
	if uniq != 1 {
		t.Fatal("per-business bank-reference uniqueness index must exist")
	}

	var durCol int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_name='purchase_requests' AND column_name='duration_minutes'`).Scan(&durCol); err != nil {
		t.Fatalf("duration column check: %v", err)
	}
	if durCol != 1 {
		t.Fatal("purchase_requests.duration_minutes must exist")
	}

	var bankCols int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_name='portal_configs' AND column_name IN ('bank_name','bank_account','bank_holder')`).Scan(&bankCols); err != nil {
		t.Fatalf("bank column check: %v", err)
	}
	if bankCols != 3 {
		t.Fatalf("portal_configs bank columns = %d, want 3", bankCols)
	}
}

// newPurchase builds a valid request (bank ref + duration are required now).
func newPurchase(biz, ref string, minutes int) *model.PurchaseRequest {
	return &model.PurchaseRequest{
		BusinessID:      biz,
		PlanKey:         "sovereign",
		Amount:          5000,
		BankReference:   ref,
		DurationMinutes: minutes,
		Status:          "pending",
	}
}

// inTenantAttempt runs fn in a tenant tx where fn is EXPECTED to hit a DB error
// (constraint violation). It tolerates the aborted-tx reset noise and always
// rolls back; callers capture the DB error themselves.
func inTenantAttempt(t *testing.T, db *DB, businessID string, fn func(tx *sql.Tx) error) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	_ = WithTenant(tx, businessID, func() error { return fn(tx) })
	_ = tx.Rollback()
}

// TestDB_Billing_approveGrantsOnce proves approve is single-wind: the first
// approve transitions pending->approved and grants the pass; a second approve
// transitions nothing (so the handler grants nothing).
func TestDB_Billing_approveGrantsOnce(t *testing.T) {
	db := requireHarness(t)
	rrepo := NewPurchaseRequestRepository(db)
	qrepo := NewBusinessMeteredQuotaRepository(db)
	biz := insertBusiness(t, db)
	now := time.Now().UTC()

	var reqID string
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		r, err := rrepo.CreateRequestTXN(tx, newPurchase(biz, "APPROVE-REF-1", 43200))
		if err != nil {
			return err
		}
		reqID = r.ID
		return nil
	})
	if reqID == "" {
		t.Fatal("create request failed")
	}

	// First approve: transition + grant (mirrors the handler's two calls in one tx).
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		claimed, err := rrepo.ApproveRequestTXN(tx, biz, reqID, now)
		if err != nil {
			return err
		}
		if claimed == nil || claimed.Status != "approved" {
			t.Fatalf("first approve = %+v, want approved", claimed)
		}
		exp := now.Add(time.Duration(claimed.DurationMinutes) * time.Minute)
		return qrepo.UpsertTXN(tx, &model.BusinessMeteredQuota{BusinessID: biz, WebfigPassExpiresAt: &exp})
	})

	// Second approve: must NOT transition => no double grant.
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		second, err := rrepo.ApproveRequestTXN(tx, biz, reqID, now)
		if err != nil {
			return err
		}
		if second != nil {
			t.Fatal("second approve must not transition (would double-grant)")
		}
		return nil
	})

	inTenant(t, db, biz, func(tx *sql.Tx) error {
		r, err := rrepo.GetRequestByIDTXN(tx, biz, reqID)
		if err != nil {
			return err
		}
		if r == nil || r.Status != "approved" || r.DecidedAt == nil {
			t.Fatalf("request = %+v, want approved with decided_at", r)
		}
		return nil
	})
}

// TestDB_Billing_rejectGrantsZero proves reject grants nothing.
func TestDB_Billing_rejectGrantsZero(t *testing.T) {
	db := requireHarness(t)
	rrepo := NewPurchaseRequestRepository(db)
	qrepo := NewBusinessMeteredQuotaRepository(db)
	biz := insertBusiness(t, db)

	var reqID string
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		r, err := rrepo.CreateRequestTXN(tx, newPurchase(biz, "REJECT-REF-1", 43200))
		if err != nil {
			return err
		}
		reqID = r.ID
		return nil
	})

	inTenant(t, db, biz, func(tx *sql.Tx) error {
		claimed, err := rrepo.RejectRequestTXN(tx, biz, reqID, time.Now().UTC())
		if err != nil {
			return err
		}
		if claimed == nil || claimed.Status != "rejected" {
			t.Fatalf("reject = %+v, want rejected", claimed)
		}
		return nil
	})

	q, err := qrepo.GetByBusiness(biz)
	if err != nil {
		t.Fatalf("get quota: %v", err)
	}
	if q != nil && q.WebfigPassExpiresAt != nil {
		t.Fatal("reject must grant NOTHING (no pass expiry)")
	}
}

// TestDB_Billing_crossBusinessBlocked proves tenant isolation for requests:
// operator B cannot see or approve operator A's request.
func TestDB_Billing_crossBusinessBlocked(t *testing.T) {
	db := requireHarness(t)
	rrepo := NewPurchaseRequestRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)

	var reqA string
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		r, err := rrepo.CreateRequestTXN(tx, newPurchase(bizA, "CROSS-REF-1", 43200))
		if err != nil {
			return err
		}
		reqA = r.ID
		return nil
	})

	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		r, err := rrepo.GetRequestByIDTXN(tx, bizB, reqA)
		if err != nil {
			return err
		}
		if r != nil {
			t.Fatal("cross-business purchase request visible to tenant B")
		}
		return nil
	})

	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		c, err := rrepo.ApproveRequestTXN(tx, bizB, reqA, time.Now().UTC())
		if err != nil {
			return err
		}
		if c != nil {
			t.Fatal("cross-business approve must affect 0 rows")
		}
		return nil
	})

	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		r, err := rrepo.GetRequestByIDTXN(tx, bizA, reqA)
		if err != nil {
			return err
		}
		if r == nil || r.Status != "pending" {
			t.Fatalf("request = %+v, want still pending", r)
		}
		return nil
	})
}

// TestDB_Billing_deviceAnchored proves a request created under a device's
// business is visible ONLY to that business.
func TestDB_Billing_deviceAnchored(t *testing.T) {
	db := requireHarness(t)
	drepo := NewDeviceRepository(db)
	rrepo := NewPurchaseRequestRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)

	devA, err := drepo.CreateDevice(&model.Device{BusinessID: bizA, HardwareName: "ap",
		DeploymentLayer: "ACCESS_POINT", Vendor: "mikrotik", Category: "ap", Transport: "direct"})
	if err != nil {
		t.Fatalf("create device: %v", err)
	}

	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		req := newPurchase(bizA, "DEVICE-REF-1", 43200)
		req.DeviceID = &devA.DeviceID
		_, err := rrepo.CreateRequestTXN(tx, req)
		return err
	})

	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		rs, err := rrepo.ListRequestsByBusinessTXN(tx, bizA)
		if err != nil {
			return err
		}
		if len(rs) != 1 {
			t.Fatalf("tenant A requests = %d, want 1", len(rs))
		}
		return nil
	})

	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		rs, err := rrepo.ListRequestsByBusinessTXN(tx, bizB)
		if err != nil {
			return err
		}
		if len(rs) != 0 {
			t.Fatal("device-anchored request visible to unrelated tenant B")
		}
		return nil
	})
}

// TestDB_Billing_missingBankRefRejected proves a request with no bank reference
// is rejected at the DB (CHECK/NOT NULL) and leaves NO row (the handler maps
// this to 400 before creating anything).
func TestDB_Billing_missingBankRefRejected(t *testing.T) {
	db := requireHarness(t)
	rrepo := NewPurchaseRequestRepository(db)
	biz := insertBusiness(t, db)

	var createErr error
	inTenantAttempt(t, db, biz, func(tx *sql.Tx) error {
		_, createErr = rrepo.CreateRequestTXN(tx, newPurchase(biz, "   ", 43200))
		return nil
	})
	if createErr == nil {
		t.Fatal("blank bank_reference must be rejected")
	}

	inTenant(t, db, biz, func(tx *sql.Tx) error {
		rs, err := rrepo.ListRequestsByBusinessTXN(tx, biz)
		if err != nil {
			return err
		}
		if len(rs) != 0 {
			t.Fatalf("no row must be stored, got %d", len(rs))
		}
		return nil
	})
}

// TestDB_Billing_duplicateBankRefOneWins proves one bank transfer funds at most
// one live pass: a second live request with the same (business, ref) is rejected
// (the handler maps the violation to 409).
func TestDB_Billing_duplicateBankRefOneWins(t *testing.T) {
	db := requireHarness(t)
	rrepo := NewPurchaseRequestRepository(db)
	biz := insertBusiness(t, db)

	inTenant(t, db, biz, func(tx *sql.Tx) error {
		if _, err := rrepo.CreateRequestTXN(tx, newPurchase(biz, "DUP-REF-1", 43200)); err != nil {
			return err
		}
		return nil
	})

	// Same reference, different case: the lower() index must reject it.
	var dupErr error
	inTenantAttempt(t, db, biz, func(tx *sql.Tx) error {
		_, dupErr = rrepo.CreateRequestTXN(tx, newPurchase(biz, "dup-ref-1", 43200))
		return nil
	})
	if dupErr == nil {
		t.Fatal("duplicate bank_reference must be rejected")
	}

	inTenant(t, db, biz, func(tx *sql.Tx) error {
		rs, err := rrepo.ListRequestsByBusinessTXN(tx, biz)
		if err != nil {
			return err
		}
		if len(rs) != 1 {
			t.Fatalf("exactly one live request must exist, got %d", len(rs))
		}
		return nil
	})
}

// TestDB_Billing_approveGrantsRequestedDuration proves approve grants the
// duration STORED on the request (plan-driven), not any global constant.
func TestDB_Billing_approveGrantsRequestedDuration(t *testing.T) {
	db := requireHarness(t)
	rrepo := NewPurchaseRequestRepository(db)
	qrepo := NewBusinessMeteredQuotaRepository(db)
	biz := insertBusiness(t, db)
	now := time.Now().UTC()

	var reqID string
	inTenant(t, db, biz, func(tx *sql.Tx) error {
		r, err := rrepo.CreateRequestTXN(tx, newPurchase(biz, "DURATION-REF-1", 90))
		if err != nil {
			return err
		}
		reqID = r.ID
		return nil
	})

	inTenant(t, db, biz, func(tx *sql.Tx) error {
		claimed, err := rrepo.ApproveRequestTXN(tx, biz, reqID, now)
		if err != nil {
			return err
		}
		if claimed == nil {
			t.Fatal("approve must transition")
		}
		if claimed.DurationMinutes != 90 {
			t.Fatalf("stored duration = %d, want 90", claimed.DurationMinutes)
		}
		exp := now.Add(time.Duration(claimed.DurationMinutes) * time.Minute)
		return qrepo.UpsertTXN(tx, &model.BusinessMeteredQuota{BusinessID: biz, WebfigPassExpiresAt: &exp})
	})

	q, err := qrepo.GetByBusiness(biz)
	if err != nil {
		t.Fatalf("get quota: %v", err)
	}
	if q == nil || q.WebfigPassExpiresAt == nil {
		t.Fatal("pass must be granted")
	}
	// ~90 minutes, definitely not 30 days.
	delta := q.WebfigPassExpiresAt.Sub(now)
	if delta < 85*time.Minute || delta > 95*time.Minute {
		t.Fatalf("granted expiry delta = %v, want ~90m (plan-driven)", delta)
	}
}
