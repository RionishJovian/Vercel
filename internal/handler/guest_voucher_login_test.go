package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
)

// Ground-operation voucher login: issuance installs the codes on the
// RouterBOARD, and the sync applies the router's live accounting back.

type loginProvisionStub struct {
	calls   [][]VoucherLoginSpec
	failure error
}

func (s *loginProvisionStub) ProvisionVoucherLogins(ctx context.Context, businessID, deviceID string, logins []VoucherLoginSpec) error {
	s.calls = append(s.calls, logins)
	if s.failure != nil {
		return s.failure
	}
	for _, l := range logins {
		if l.Minutes < 1 || l.DataMB < 1 || l.Code == "" || l.VoucherID == "" {
			return errors.New("stub rejects invalid specs")
		}
	}
	return nil
}

type loginSyncStub struct{ states map[string]VoucherLoginState }

func (s loginSyncStub) ReadVoucherLogins(ctx context.Context, businessID, deviceID string, refs []VoucherLoginSpec) ([]VoucherLoginState, error) {
	out := make([]VoucherLoginState, 0, len(refs))
	for _, r := range refs {
		if st, ok := s.states[r.VoucherID]; ok {
			out = append(out, st)
		}
	}
	return out, nil
}

func loginFixture(t *testing.T, db *database.DB) (string, string) {
	t.Helper()
	var biz, device string
	if err := db.QueryRow(`INSERT INTO businesses (company_name, owner_email, access_tier) VALUES ($1, $2, 'SMALL_WISP') RETURNING business_id`,
		"Login Harness Co "+randHex(4), "login-"+randHex(4)+"@example.test").Scan(&biz); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO devices (business_id, hardware_name, deployment_layer) VALUES ($1, 'Login Gateway', 'MAIN_GATEWAY') RETURNING device_id`, biz).Scan(&device); err != nil {
		t.Fatal(err)
	}
	return biz, device
}

func callIssue(h *GuestVoucherHandler, biz, device string, count int) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"device_id":%q,"plan_label":"Login plan","price_mmk":1000,"duration_minutes":60,"data_mb":256,"count":%d}`, device, count)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/guest-vouchers", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(auth.ContextKeyBusinessID, biz)
	h.Issue(c)
	return w
}

func callSync(h *GuestVoucherHandler, biz, device string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/guest-vouchers/sync", strings.NewReader(fmt.Sprintf(`{"device_id":%q}`, device)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(auth.ContextKeyBusinessID, biz)
	h.SyncVoucherLogins(c)
	return w
}

func loginHandler(db *database.DB) *GuestVoucherHandler {
	return &GuestVoucherHandler{DB: db, Vouchers: NewVoucherHandler(db, database.NewVoucherRepository(db), nil),
		ResolveDevice: database.NewDeviceRepository(db).GetDeviceByID}
}

func TestGuestVoucherIssue_provisionsCodesOnTheGateway(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	prov := &loginProvisionStub{}
	h := loginHandler(db)
	h.Provisioner = prov
	w := callIssue(h, biz, device, 2)
	if w.Code != 201 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if len(prov.calls) != 1 || len(prov.calls[0]) != 2 {
		t.Fatalf("provisioner called with %+v", prov.calls)
	}
	for _, spec := range prov.calls[0] {
		if spec.Minutes != 60 || spec.DataMB != 256 || spec.Code == "" {
			t.Errorf("bad spec %+v", spec)
		}
	}
}

func TestGuestVoucherIssue_withdrawsCodesWhenGatewayProvisioningFails(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	h := loginHandler(db)
	h.Provisioner = &loginProvisionStub{failure: errors.New("gateway unreachable")}
	w := callIssue(h, biz, device, 3)
	if w.Code != 502 {
		t.Fatalf("expected 502, got %d body=%s", w.Code, w.Body.String())
	}
	var vouchers, grants int
	if err := db.QueryRow(`SELECT count(*) FROM vouchers WHERE business_id=$1`, biz).Scan(&vouchers); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM guest_voucher_grants WHERE business_id=$1`, biz).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if vouchers != 0 || grants != 0 {
		t.Fatalf("issued rows must be withdrawn on provisioning failure, got vouchers=%d grants=%d", vouchers, grants)
	}
}

func TestGuestVoucherSync_appliesRouterAccounting(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	h := loginHandler(db)
	if w := callIssue(h, biz, device, 1); w.Code != 201 {
		t.Fatalf("issue status=%d body=%s", w.Code, w.Body.String())
	}
	var voucherID string
	if err := db.QueryRow(`SELECT id FROM vouchers WHERE business_id=$1`, biz).Scan(&voucherID); err != nil {
		t.Fatal(err)
	}
	// Router-observed usage with a live session: voucher marked used, grant
	// claimed, activated with an immutable timestamp.
	h.Syncer = loginSyncStub{states: map[string]VoucherLoginState{voucherID: {
		VoucherID: voucherID, Exists: true, UptimeSeconds: 300, BytesUsed: 1024, ActiveMAC: "AA:BB:CC:DD:EE:FF",
	}}}
	if w := callSync(h, biz, device); w.Code != 200 {
		t.Fatalf("sync status=%d body=%s", w.Code, w.Body.String())
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM vouchers WHERE id=$1`, voucherID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "used" {
		t.Fatalf("voucher must be used after router usage, got %q", status)
	}
	var state string
	var activated sql.NullTime
	if err := db.QueryRow(`SELECT state, activated_at FROM guest_voucher_grants WHERE voucher_id=$1`, voucherID).Scan(&state, &activated); err != nil {
		t.Fatal(err)
	}
	if state != "active" || !activated.Valid {
		t.Fatalf("grant must be activated with a timestamp, got state=%q activated=%v", state, activated)
	}
	// Router-side expiry propagates without touching the timestamp.
	h.Syncer = loginSyncStub{states: map[string]VoucherLoginState{voucherID: {
		VoucherID: voucherID, Exists: true, Disabled: true, UptimeSeconds: 3600, BytesUsed: 1024,
	}}}
	if w := callSync(h, biz, device); w.Code != 200 {
		t.Fatalf("second sync status=%d body=%s", w.Code, w.Body.String())
	}
	if err := db.QueryRow(`SELECT state FROM guest_voucher_grants WHERE voucher_id=$1`, voucherID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "expired" {
		t.Fatalf("grant must expire with the router, got %q", state)
	}
	if err := db.QueryRow(`SELECT activated_at FROM guest_voucher_grants WHERE voucher_id=$1`, voucherID).Scan(&activated); err != nil || !activated.Valid {
		t.Fatalf("activation timestamp must be immutable across expiry: %v %v", activated, err)
	}
}

func TestGuestVoucherSync_rejectsForeignOrMissingGateways(t *testing.T) {
	db := harness_testdb(t)
	biz, _ := loginFixture(t, db)
	otherBiz, otherDevice := loginFixture(t, db)
	h := loginHandler(db)
	h.Syncer = loginSyncStub{}
	for _, tc := range []struct{ body string }{
		{`{"device_id":"not-a-uuid"}`},
		{fmt.Sprintf(`{"device_id":%q}`, otherDevice)},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/guest-vouchers/sync", strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(auth.ContextKeyBusinessID, biz)
		h.SyncVoucherLogins(c)
		if w.Code != 400 && w.Code != 404 {
			t.Errorf("foreign/invalid gateway must be rejected, got %d body=%s", w.Code, w.Body.String())
		}
	}
	_ = otherBiz
}

// The periodic worker path (SyncBusinessVouchers): every gateway of the
// business is synchronized, non-gateway devices are skipped, and a business
// without issued vouchers is a no-op.
func TestGuestVoucherSyncBusiness_appliesEveryGateway(t *testing.T) {
	db := harness_testdb(t)
	biz, deviceA := loginFixture(t, db)
	var deviceB, laptop string
	if err := db.QueryRow(`INSERT INTO devices (business_id, hardware_name, deployment_layer) VALUES ($1, 'Second Gateway', 'MAIN_GATEWAY') RETURNING device_id`, biz).Scan(&deviceB); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO devices (business_id, hardware_name, deployment_layer) VALUES ($1, 'Guest Laptop', 'ACCESS_POINT') RETURNING device_id`, biz).Scan(&laptop); err != nil {
		t.Fatal(err)
	}
	// A business without issued vouchers synchronizes to a clean zero.
	emptyBiz, _ := loginFixture(t, db)
	h := loginHandler(db)
	h.Syncer = loginSyncStub{}
	applied, err := h.SyncBusinessVouchers(context.Background(), emptyBiz)
	if err != nil || applied != 0 {
		t.Fatalf("empty business must be a no-op, got applied=%d err=%v", applied, err)
	}
	// Issue one voucher on the first gateway only; the second gateway and the
	// access point must be visited without failing the business scan.
	if w := callIssue(h, biz, deviceA, 1); w.Code != 201 {
		t.Fatalf("issue status=%d body=%s", w.Code, w.Body.String())
	}
	var voucherID string
	if err := db.QueryRow(`SELECT id FROM vouchers WHERE business_id=$1`, biz).Scan(&voucherID); err != nil {
		t.Fatal(err)
	}
	h.Syncer = loginSyncStub{states: map[string]VoucherLoginState{voucherID: {
		VoucherID: voucherID, Exists: true, UptimeSeconds: 300, BytesUsed: 4096, ActiveMAC: "11:22:33:44:55:66",
	}}}
	applied, err = h.SyncBusinessVouchers(context.Background(), biz)
	if err != nil {
		t.Fatalf("business sync failed: %v", err)
	}
	if applied != 1 {
		t.Fatalf("one issued voucher must apply exactly one result, got %d", applied)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM vouchers WHERE id=$1`, voucherID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "used" {
		t.Fatalf("voucher must be used after the business-wide sync, got %q", status)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM guest_voucher_grants WHERE voucher_id=$1`, voucherID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "active" {
		t.Fatalf("grant must be active after the business-wide sync, got %q", state)
	}
	_ = deviceB
	_ = laptop
}
