package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/voucher"
)

func batchTestID() string {
	return randHex(4) + "-" + randHex(2) + "-" + randHex(2) + "-" + randHex(2) + "-" + randHex(6)
}

func batchTestCall(h *GuestVoucherHandler, method, biz, batchID, body string, endpoint gin.HandlerFunc) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/guest-vouchers/batches/"+batchID, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(auth.ContextKeyBusinessID, biz)
	c.Params = gin.Params{{Key: "batchId", Value: batchID}}
	endpoint(c)
	return w
}

func batchTestBody(id, device string, price int) string {
	return fmt.Sprintf(`{"batch_id":%q,"device_id":%q,"plan_label":" Test plan ","price_mmk":%d,"duration_minutes":60,"data_mb":256,"count":2}`, id, device, price)
}

func batchTestJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var out map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON %s: %v", w.Body.String(), err)
	}
	return out
}

func TestGuestVoucherIssueBatchKeyBoundary(t *testing.T) {
	const device = "11111111-1111-4111-8111-111111111111"
	h := &GuestVoucherHandler{}
	for _, id := range []string{"", "not-a-uuid"} {
		w := batchTestCall(h, "POST", device, id, batchTestBody(id, device, 1000), h.Issue)
		if w.Code != 400 {
			t.Fatalf("missing/bad key status=%d %s", w.Code, w.Body.String())
		}
	}
	id := batchTestID()
	w := batchTestCall(h, "POST", device, id, batchTestBody(id, device, 1000), h.Issue)
	if w.Code != 503 {
		t.Fatalf("unwired provisioner must fail closed: %d %s", w.Code, w.Body.String())
	}
}

func TestGuestVoucherIssueBatchReusesIdentityAndRejectsMismatch(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	h := loginHandler(db)
	prov := &loginProvisionStub{}
	h.Provisioner = prov
	id, body := batchTestID(), ""
	body = batchTestBody(id, device, 1000)
	first := batchTestCall(h, "POST", biz, id, body, h.Issue)
	if first.Code != 201 {
		t.Fatalf("first status=%d %s", first.Code, first.Body.String())
	}
	retry := batchTestCall(h, "POST", biz, id, body, h.Issue)
	if retry.Code != 201 || len(prov.calls) != 1 {
		t.Fatalf("retry reissued codes/provisioned: status=%d calls=%d %s", retry.Code, len(prov.calls), retry.Body.String())
	}
	var initial, repeated []*model.Voucher
	_ = json.Unmarshal(batchTestJSON(t, first)["vouchers"], &initial)
	_ = json.Unmarshal(batchTestJSON(t, retry)["vouchers"], &repeated)
	if len(initial) != 2 || len(repeated) != 2 || initial[0].ID != repeated[0].ID || initial[0].Code != repeated[0].Code {
		t.Fatalf("retry changed voucher identities: %+v %+v", initial, repeated)
	}
	for _, v := range initial {
		if len(v.Code) != 8 || strings.Trim(v.Code, voucher.GuestCodeAlphabet) != "" {
			t.Fatal("new guest batch did not use eight uppercase letters")
		}
	}
	mismatch := batchTestCall(h, "POST", biz, id, batchTestBody(id, device, 2000), h.Issue)
	if mismatch.Code != 409 {
		t.Fatalf("same key changed allowance: %d %s", mismatch.Code, mismatch.Body.String())
	}
	var count int
	_ = db.QueryRow(`SELECT count(*) FROM vouchers WHERE business_id=$1`, biz).Scan(&count)
	if count != 2 {
		t.Fatalf("unexpected voucher count %d", count)
	}
}

func TestGuestVoucherBatchRetryKeepsLegacyLongCodes(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	h := loginHandler(db)
	p := &loginProvisionStub{}
	h.Provisioner = p
	id := batchTestID()
	codes := []string{"ABCDEFGH2345", "JKMNPQRS6789"}
	ids := []string{}
	err := withTenant(db, biz, func(tx *sql.Tx) error {
		repo := database.GuestVoucherBatchRepository{}
		_, err := repo.CreateTXN(tx, database.GuestVoucherIssueBatch{ID: id, BusinessID: biz, DeviceID: device, RequestHash: strings.Repeat("a", 64), Label: "Legacy batch", PriceMMK: 1000, Minutes: 60, DataMB: 256, TotalCount: 2})
		if err != nil {
			return err
		}
		for i, code := range codes {
			v, err := database.NewVoucherRepository(db).CreateVoucherTXN(tx, &model.Voucher{BusinessID: biz, DeviceID: &device, PlanKey: "guest-internet", PlanLabel: "Legacy batch", Code: code, PriceMMK: 1000, Status: "unused"})
			if err != nil {
				return err
			}
			ids = append(ids, v.ID)
			if err = (database.GuestVoucherRepository{}).IssueTXN(tx, biz, v.ID, device, 60, 256); err != nil {
				return err
			}
			if err = repo.AddItemTXN(tx, biz, id, v.ID, i); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	w := batchTestCall(h, "POST", biz, id, "", h.RetryIssueBatch)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"ready"`) {
		t.Fatalf("legacy retry: %d %s", w.Code, w.Body.String())
	}
	var got []*model.Voucher
	if err = json.Unmarshal(batchTestJSON(t, w)["vouchers"], &got); err != nil || len(got) != 2 || len(p.calls) != 1 {
		t.Fatal("legacy batch not verified", err)
	}
	for i, v := range got {
		if v.ID != ids[i] || v.Code != codes[i] || p.calls[0][i].Code != codes[i] {
			t.Fatal("legacy retry replaced an original code or identity")
		}
	}
}

type uncertainBatchProvisioner struct {
	mu        sync.Mutex
	installed map[string]VoucherLoginSpec
	fail      bool
	calls     int
}

func (p *uncertainBatchProvisioner) ProvisionVoucherLogins(_ context.Context, _, _ string, specs []VoucherLoginSpec) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	for _, spec := range specs {
		if old, ok := p.installed[spec.VoucherID]; ok && old != spec {
			return errors.New("retry changed an installed entitlement")
		}
		p.installed[spec.VoucherID] = spec
		if p.fail {
			return context.DeadlineExceeded // remote add succeeded; response lost.
		}
	}
	return nil
}

func TestGuestVoucherIssueBatchRecoversPartialRemoteWriteWithoutDuplicates(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	h := loginHandler(db)
	p := &uncertainBatchProvisioner{installed: map[string]VoucherLoginSpec{}, fail: true}
	h.Provisioner = p
	id := batchTestID()
	w := batchTestCall(h, "POST", biz, id, batchTestBody(id, device, 1000), h.Issue)
	if w.Code != 202 || strings.Contains(w.Body.String(), `"code":`) {
		t.Fatalf("uncertain outcome exposed ready codes: %d %s", w.Code, w.Body.String())
	}
	var rows, unknown int
	_ = db.QueryRow(`SELECT count(*) FROM vouchers WHERE business_id=$1`, biz).Scan(&rows)
	_ = db.QueryRow(`SELECT count(*) FROM guest_voucher_issue_items WHERE business_id=$1 AND install_state='unknown'`, biz).Scan(&unknown)
	if rows != 2 || unknown != 2 || len(p.installed) != 1 {
		t.Fatalf("lost recoverable identity: rows=%d unknown=%d installed=%d", rows, unknown, len(p.installed))
	}
	list := batchTestCall(h, "GET", biz, id, "", h.ListIssueBatches)
	if list.Code != 200 || !strings.Contains(list.Body.String(), id) || strings.Contains(list.Body.String(), `"code":`) {
		t.Fatalf("batch not safely discoverable after reload: %d %s", list.Code, list.Body.String())
	}
	// Existing general inventory carries explicit unverified installation state.
	wInventory := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(wInventory)
	c.Request = httptest.NewRequest("GET", "/vouchers", nil)
	c.Set(auth.ContextKeyBusinessID, biz)
	h.Vouchers.ListVouchers(c)
	if wInventory.Code != 200 || strings.Count(wInventory.Body.String(), `"install_state":"unknown"`) != 2 {
		t.Fatalf("inventory lost installation state: %d %s", wInventory.Code, wInventory.Body.String())
	}
	p.fail = false
	w = batchTestCall(h, "POST", biz, id, "", h.RetryIssueBatch)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"ready"`) || len(p.installed) != 2 {
		t.Fatalf("retry did not verify original batch: %d %s installed=%d", w.Code, w.Body.String(), len(p.installed))
	}
	_ = db.QueryRow(`SELECT count(*) FROM vouchers WHERE business_id=$1`, biz).Scan(&rows)
	if rows != 2 || p.calls != 2 {
		t.Fatalf("retry created duplicates: rows=%d calls=%d", rows, p.calls)
	}
	list = batchTestCall(h, "GET", biz, id, "", h.ListIssueBatches)
	if strings.Contains(string(batchTestJSON(t, list)["batches"]), id) {
		t.Fatalf("ready batch still shown as incomplete: %s", list.Body.String())
	}
}

type heldBatchProvisioner struct {
	start, release chan struct{}
	mu             sync.Mutex
	calls          int
}

func (p *heldBatchProvisioner) ProvisionVoucherLogins(ctx context.Context, _, _ string, _ []VoucherLoginSpec) error {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	close(p.start)
	select {
	case <-p.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestGuestVoucherIssueBatchConcurrentRetryIsLeased(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	h := loginHandler(db)
	p := &heldBatchProvisioner{start: make(chan struct{}), release: make(chan struct{})}
	h.Provisioner = p
	id := batchTestID()
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- batchTestCall(h, "POST", biz, id, batchTestBody(id, device, 1000), h.Issue) }()
	<-p.start
	second := batchTestCall(h, "POST", biz, id, batchTestBody(id, device, 1000), h.Issue)
	close(p.release)
	completed := <-first
	if second.Code != 202 || !strings.Contains(second.Body.String(), `"state":"installing"`) || completed.Code != 201 || p.calls != 1 {
		t.Fatalf("lease failed: second=%d %s first=%d calls=%d", second.Code, second.Body.String(), completed.Code, p.calls)
	}
}

func TestGuestVoucherIssueBatchTenantIsolationAndImmutableMembership(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	other, _ := loginFixture(t, db)
	h := loginHandler(db)
	id := batchTestID()
	if w := batchTestCall(h, "POST", biz, id, batchTestBody(id, device, 1000), h.Issue); w.Code != 201 {
		t.Fatalf("issue %d %s", w.Code, w.Body.String())
	}
	for _, endpoint := range []gin.HandlerFunc{h.GetIssueBatch, h.RetryIssueBatch} {
		w := batchTestCall(h, "POST", other, id, "", endpoint)
		if w.Code != 404 {
			t.Fatalf("foreign batch visible: %d %s", w.Code, w.Body.String())
		}
	}
	if err := withDeviceTenant(db, other, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM guest_voucher_issue_items WHERE business_id=$1`, biz).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatal("RLS exposed another tenant's membership")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE guest_voucher_issue_batches SET minutes=minutes+1 WHERE business_id=$1`,
		`UPDATE guest_voucher_issue_items SET install_state='unknown',verified_at=NULL WHERE business_id=$1`,
	} {
		if err := withDeviceTenant(db, biz, func(tx *sql.Tx) error { _, err := tx.Exec(statement, biz); return err }); err == nil {
			t.Fatalf("immutable batch protection missing: %s", statement)
		}
	}
}

func TestGuestVoucherIssueBatchStaleWorkerCannotVerifyReplacementLease(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	h := loginHandler(db)
	h.Provisioner = &loginProvisionStub{failure: context.DeadlineExceeded}
	id := batchTestID()
	if w := batchTestCall(h, "POST", biz, id, batchTestBody(id, device, 1000), h.Issue); w.Code != 202 {
		t.Fatalf("expected uncertain batch, got %d %s", w.Code, w.Body.String())
	}
	repo := database.GuestVoucherBatchRepository{}
	if err := withDeviceTenant(db, biz, func(tx *sql.Tx) error {
		claimed, err := repo.ClaimTXN(tx, biz, id, "first-worker")
		if err != nil || !claimed {
			return fmt.Errorf("first lease: claimed=%v error=%v", claimed, err)
		}
		_, err = tx.Exec(`UPDATE guest_voucher_issue_batches SET lease_until=clock_timestamp()-interval '1 second' WHERE business_id=$1 AND id=$2`, biz, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := withDeviceTenant(db, biz, func(tx *sql.Tx) error {
		claimed, err := repo.ClaimTXN(tx, biz, id, "replacement-worker")
		if err != nil || !claimed {
			return fmt.Errorf("replacement lease: claimed=%v error=%v", claimed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := withDeviceTenant(db, biz, func(tx *sql.Tx) error {
		return repo.FinishTXN(tx, biz, id, "first-worker", true)
	}); !errors.Is(err, database.ErrGuestVoucherBusy) {
		t.Fatalf("stale worker unexpectedly won verification: %v", err)
	}
	if err := withDeviceTenant(db, biz, func(tx *sql.Tx) error {
		return repo.FinishTXN(tx, biz, id, "replacement-worker", true)
	}); err != nil {
		t.Fatal(err)
	}
}
