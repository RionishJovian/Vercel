package database

import (
	"database/sql"
	"testing"
	"time"

	"mega-magic/internal/model"
)

// TestDB_BillingSchema_applied proves the Part-1 schema: purchase_requests
// exists with RLS enabled + a policy, and portal_configs gained bank columns.
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

	var bankCols int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_name='portal_configs' AND column_name IN ('bank_name','bank_account','bank_holder')`).Scan(&bankCols); err != nil {
		t.Fatalf("bank column check: %v", err)
	}
	if bankCols != 3 {
		t.Fatalf("portal_configs bank columns = %d, want 3", bankCols)
	}
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
		r, err := rrepo.CreateRequestTXN(tx, &model.PurchaseRequest{BusinessID: biz, PlanKey: "sovereign", Amount: 5000, Status: "pending"})
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
		exp := now.Add(30 * 24 * time.Hour)
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
		r, err := rrepo.CreateRequestTXN(tx, &model.PurchaseRequest{BusinessID: biz, PlanKey: "sovereign", Status: "pending"})
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
		r, err := rrepo.CreateRequestTXN(tx, &model.PurchaseRequest{BusinessID: bizA, PlanKey: "sovereign", Status: "pending"})
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

	// Still pending for its true tenant.
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
		_, err := rrepo.CreateRequestTXN(tx, &model.PurchaseRequest{
			BusinessID: bizA, DeviceID: &devA.DeviceID, PlanKey: "sovereign", Status: "pending",
		})
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
