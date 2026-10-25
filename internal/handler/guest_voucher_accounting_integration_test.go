package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"mega-magic/internal/database"
)

type accountingSyncFunc func(context.Context, string, string, []VoucherLoginSpec) ([]VoucherLoginState, error)

func (f accountingSyncFunc) ReadVoucherLogins(ctx context.Context, businessID, deviceID string, specs []VoucherLoginSpec) ([]VoucherLoginState, error) {
	return f(ctx, businessID, deviceID, specs)
}

// Seed the accounting contract independently of provisioning/batch issuance.
// These are explicit guest grants; a legacy business pass has no such grant.
func accountingSeedGrants(t *testing.T, db *database.DB, biz, device string, count int) []string {
	t.Helper()
	var ids []string
	err := withTenant(db, biz, func(tx *sql.Tx) error {
		rows, err := tx.Query(`WITH created AS (
 INSERT INTO vouchers(business_id,device_id,plan_key,plan_label,code,price_mmk,duration_minutes)
 SELECT $1::uuid,$2::uuid,'guest-internet','Accounting plan',$3::text||n::text,1000,60
 FROM generate_series(1,$4::int) n RETURNING id,business_id,device_id)
 INSERT INTO guest_voucher_grants(voucher_id,business_id,device_id,minutes,data_mb)
 SELECT id,business_id,device_id,60,256 FROM created RETURNING voucher_id::text`, biz, device, "ACCOUNT"+strings.ToUpper(randHex(6)), count)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != count {
		t.Fatalf("seeded %d of %d grants", len(ids), count)
	}
	return ids
}

func accountingObserved(specs []VoucherLoginSpec) []VoucherLoginState {
	states := make([]VoucherLoginState, 0, len(specs))
	for _, spec := range specs {
		states = append(states, VoucherLoginState{VoucherID: spec.VoucherID, Exists: true,
			UptimeSeconds: 3, BytesUsed: 1024, ActiveMAC: "02:00:00:00:00:01", FirstUseEpoch: time.Now().Add(-time.Minute).Unix(), FirstMAC: "02:00:00:00:00:01"})
	}
	return states
}

type accountingGrantSnapshot struct {
	status, state                string
	activated, firstUsed, expiry sql.NullTime
}

func accountingSnapshot(t *testing.T, db *database.DB, biz, id string) accountingGrantSnapshot {
	t.Helper()
	var snapshot accountingGrantSnapshot
	err := withTenant(db, biz, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT v.status,g.state,g.activated_at,g.first_used_at,g.expires_at
 FROM vouchers v JOIN guest_voucher_grants g ON g.voucher_id=v.id AND g.business_id=v.business_id
 WHERE v.business_id=$1 AND v.id=$2`, biz, id).Scan(&snapshot.status, &snapshot.state, &snapshot.activated, &snapshot.firstUsed, &snapshot.expiry)
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestGuestVoucherAccountingDataCapExpiresWithoutChangingActivation(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	id := accountingSeedGrants(t, db, biz, device, 1)[0]
	h := loginHandler(db)
	h.Syncer = accountingSyncFunc(func(_ context.Context, _, _ string, specs []VoucherLoginSpec) ([]VoucherLoginState, error) {
		return accountingObserved(specs), nil
	})
	if w := callSync(h, biz, device); w.Code != 200 {
		t.Fatalf("activation status=%d body=%s", w.Code, w.Body.String())
	}
	before := accountingSnapshot(t, db, biz, id)
	if before.state != "active" || before.status != "used" || !before.activated.Valid {
		t.Fatalf("initial observation did not activate grant: %+v", before)
	}
	h.Syncer = accountingSyncFunc(func(_ context.Context, _, _ string, specs []VoucherLoginSpec) ([]VoucherLoginState, error) {
		return []VoucherLoginState{{VoucherID: id, Exists: true, UptimeSeconds: 10, BytesUsed: 256000000}}, nil
	})
	if w := callSync(h, biz, device); w.Code != 200 {
		t.Fatalf("data-limit sync status=%d body=%s", w.Code, w.Body.String())
	}
	after := accountingSnapshot(t, db, biz, id)
	if after.state != "expired" || !after.activated.Valid || !before.activated.Time.Equal(after.activated.Time) ||
		!before.firstUsed.Time.Equal(after.firstUsed.Time) || !before.expiry.Time.Equal(after.expiry.Time) {
		t.Fatalf("data exhaustion must expire while retaining history: before=%+v after=%+v", before, after)
	}
}

func TestGuestVoucherAccountingAbsentUserDoesNotInventExpiry(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	id := accountingSeedGrants(t, db, biz, device, 1)[0]
	h := loginHandler(db)
	h.Syncer = accountingSyncFunc(func(_ context.Context, _, _ string, specs []VoucherLoginSpec) ([]VoucherLoginState, error) {
		return accountingObserved(specs), nil
	})
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := h.syncGatewayVouchers(context.Background(), biz, device, now); err != nil {
		t.Fatal(err)
	}
	before := accountingSnapshot(t, db, biz, id)
	h.Syncer = accountingSyncFunc(func(_ context.Context, _, _ string, _ []VoucherLoginSpec) ([]VoucherLoginState, error) {
		return []VoucherLoginState{{VoucherID: id}}, nil
	})
	// Even an elapsed app deadline cannot turn an absent account into a
	// router-observed expiry. An absent user is unavailable evidence.
	if _, err := h.syncGatewayVouchers(context.Background(), biz, device, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	after := accountingSnapshot(t, db, biz, id)
	if after.state != "active" || after.status != "used" || !before.activated.Time.Equal(after.activated.Time) ||
		!before.firstUsed.Time.Equal(after.firstUsed.Time) || !before.expiry.Time.Equal(after.expiry.Time) {
		t.Fatalf("missing router user changed app history: before=%+v after=%+v", before, after)
	}
}

func TestGuestVoucherAccountingRevokedVoucherIsNotReactivated(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	id := accountingSeedGrants(t, db, biz, device, 1)[0]
	if err := withTenant(db, biz, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE vouchers SET status='revoked' WHERE business_id=$1 AND id=$2`, biz, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	h := loginHandler(db)
	h.Syncer = accountingSyncFunc(func(_ context.Context, _, _ string, specs []VoucherLoginSpec) ([]VoucherLoginState, error) {
		return accountingObserved(specs), nil
	})
	if w := callSync(h, biz, device); w.Code != 200 {
		t.Fatalf("revoked sync status=%d body=%s", w.Code, w.Body.String())
	}
	after := accountingSnapshot(t, db, biz, id)
	if after.status != "revoked" || after.state != "unused" || after.activated.Valid || after.firstUsed.Valid || after.expiry.Valid {
		t.Fatalf("router usage must not reactivate a revoked voucher: %+v", after)
	}
}

func TestGuestVoucherAccountingIgnoresBusinessPassAndForeignGrant(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	id := accountingSeedGrants(t, db, biz, device, 1)[0]
	otherBiz, otherDevice := loginFixture(t, db)
	foreignID := accountingSeedGrants(t, db, otherBiz, otherDevice, 1)[0]
	var passID string
	if err := withTenant(db, biz, func(tx *sql.Tx) error {
		return tx.QueryRow(`INSERT INTO vouchers(business_id,device_id,plan_key,plan_label,code,duration_minutes)
 VALUES ($1,$2,'business-pass','Business pass',$3,60) RETURNING id::text`, biz, device, "PASS"+strings.ToUpper(randHex(6))).Scan(&passID)
	}); err != nil {
		t.Fatal(err)
	}
	h := loginHandler(db)
	calls := 0
	h.Syncer = accountingSyncFunc(func(_ context.Context, observedBiz, observedDevice string, specs []VoucherLoginSpec) ([]VoucherLoginState, error) {
		calls++
		if observedBiz != biz || observedDevice != device || len(specs) != 1 || specs[0].VoucherID != id {
			t.Fatalf("non-guest or foreign voucher sent to router reader: biz=%s device=%s specs=%+v", observedBiz, observedDevice, specs)
		}
		return accountingObserved(specs), nil
	})
	if w := callSync(h, biz, device); w.Code != 200 {
		t.Fatalf("scoped sync status=%d body=%s", w.Code, w.Body.String())
	}
	var passStatus string
	if err := db.QueryRow(`SELECT status FROM vouchers WHERE business_id=$1 AND id=$2`, biz, passID).Scan(&passStatus); err != nil {
		t.Fatal(err)
	}
	foreign := accountingSnapshot(t, db, otherBiz, foreignID)
	if calls != 1 || passStatus != "unused" || foreign.status != "unused" || foreign.state != "unused" || foreign.activated.Valid {
		t.Fatalf("sync crossed product/tenant boundary: calls=%d pass=%s foreign=%+v", calls, passStatus, foreign)
	}
}

func TestGuestVoucherAccountingPagesInventoryAndFailsAtomically(t *testing.T) {
	for _, failLast := range []bool{false, true} {
		t.Run(fmt.Sprint("failLast=", failLast), func(t *testing.T) {
			db := harness_testdb(t)
			biz, device := loginFixture(t, db)
			accountingSeedGrants(t, db, biz, device, voucherAccountingPageSize+1)
			h := loginHandler(db)
			var pages []int
			h.Syncer = accountingSyncFunc(func(_ context.Context, _, _ string, specs []VoucherLoginSpec) ([]VoucherLoginState, error) {
				pages = append(pages, len(specs))
				if len(specs) > 200 {
					t.Fatalf("unbounded reader page: %d", len(specs))
				}
				if failLast && len(pages) == 2 {
					return nil, errors.New("second router page unavailable")
				}
				return accountingObserved(specs), nil
			})
			w := callSync(h, biz, device)
			if len(pages) != 2 || pages[0] != 200 || pages[1] != 1 {
				t.Fatalf("expected bounded pages [200 1], got %v", pages)
			}
			wantHTTP := 200
			wantUsed := voucherAccountingPageSize + 1
			if failLast {
				wantHTTP, wantUsed = 502, 0
			}
			if w.Code != wantHTTP {
				t.Fatalf("sync status=%d want=%d body=%s", w.Code, wantHTTP, w.Body.String())
			}
			var used, active, activated int
			if err := db.QueryRow(`SELECT count(*) FILTER (WHERE v.status='used'),count(*) FILTER (WHERE g.state='active'),count(g.activated_at)
 FROM vouchers v JOIN guest_voucher_grants g ON g.voucher_id=v.id AND g.business_id=v.business_id WHERE v.business_id=$1`, biz).Scan(&used, &active, &activated); err != nil {
				t.Fatal(err)
			}
			if used != wantUsed || active != wantUsed || activated != wantUsed {
				t.Fatalf("paged sync must apply all records or none: used=%d active=%d activated=%d want=%d", used, active, activated, wantUsed)
			}
		})
	}
}

func TestGuestVoucherAccountingGatewayFailureDoesNotStarveHealthyGateway(t *testing.T) {
	db := harness_testdb(t)
	biz, firstDevice := loginFixture(t, db)
	firstID := accountingSeedGrants(t, db, biz, firstDevice, 1)[0]
	var secondDevice string
	if err := db.QueryRow(`INSERT INTO devices(business_id,hardware_name,deployment_layer)
 VALUES ($1,'Second accounting gateway','MAIN_GATEWAY') RETURNING device_id::text`, biz).Scan(&secondDevice); err != nil {
		t.Fatal(err)
	}
	secondID := accountingSeedGrants(t, db, biz, secondDevice, 1)[0]
	ids := map[string]string{firstDevice: firstID, secondDevice: secondID}
	h := loginHandler(db)
	var failedDevice, healthyDevice string
	calls := 0
	h.Syncer = accountingSyncFunc(func(_ context.Context, _, device string, specs []VoucherLoginSpec) ([]VoucherLoginState, error) {
		calls++
		if calls == 1 {
			failedDevice = device
			return nil, errors.New("first gateway is offline")
		}
		healthyDevice = device
		return accountingObserved(specs), nil
	})
	applied, err := h.SyncBusinessVouchers(context.Background(), biz)
	if err == nil || applied != 1 || calls != 2 || failedDevice == "" || healthyDevice == "" || failedDevice == healthyDevice {
		t.Fatalf("worker must report failure while continuing: applied=%d calls=%d failed=%s healthy=%s err=%v", applied, calls, failedDevice, healthyDevice, err)
	}
	failed := accountingSnapshot(t, db, biz, ids[failedDevice])
	healthy := accountingSnapshot(t, db, biz, ids[healthyDevice])
	if failed.status != "unused" || failed.state != "unused" || failed.activated.Valid || healthy.status != "used" || healthy.state != "active" || !healthy.activated.Valid {
		t.Fatalf("wrong cross-gateway result: failed=%+v healthy=%+v", failed, healthy)
	}
}

func TestGuestVoucherAccountingLiveSessionWithoutAnchorNeedsReview(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	id := accountingSeedGrants(t, db, biz, device, 1)[0]
	h := loginHandler(db)
	h.Syncer = accountingSyncFunc(func(_ context.Context, _, _ string, _ []VoucherLoginSpec) ([]VoucherLoginState, error) {
		return []VoucherLoginState{{VoucherID: id, Exists: true, UptimeSeconds: 30, BytesUsed: 512, ActiveMAC: "02:00:00:00:00:01"}}, nil
	})
	w := callSync(h, biz, device)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"first_use_status":"unverified"`) {
		t.Fatalf("missing review status: %d %s", w.Code, w.Body.String())
	}
	g := accountingSnapshot(t, db, biz, id)
	if g.state != "unused" || g.activated.Valid || g.firstUsed.Valid || g.expiry.Valid {
		t.Fatalf("invented first use from scan time: %+v", g)
	}
}
