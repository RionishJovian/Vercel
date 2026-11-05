package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/provisioner/routeros"
)

type guestActionsFake struct {
	sessions []routeros.GuestSession
	blocked  []string
	unblock  []string
	err      error
}

func (f *guestActionsFake) DisconnectGuest(_ context.Context, _, _ string, s routeros.GuestSession) error {
	f.sessions = append(f.sessions, s)
	return f.err
}
func (f *guestActionsFake) BlockGuestMAC(_ context.Context, _, _, mac string) error {
	f.blocked = append(f.blocked, mac)
	return f.err
}
func (f *guestActionsFake) UnblockGuestMAC(_ context.Context, _, _, mac string) error {
	f.unblock = append(f.unblock, mac)
	return f.err
}

type retirerFake struct {
	logins []VoucherLoginSpec
	device string
	err    error
}

func (f *retirerFake) RemoveVoucherLogin(_ context.Context, _, device string, l VoucherLoginSpec) error {
	f.logins, f.device = append(f.logins, l), device
	return f.err
}

func guestActionsOn(t *testing.T) {
	t.Helper()
	t.Setenv("MM_NETWORK_CONTROLS_ENABLED", "true")
	t.Setenv("MM_GUEST_ACTIONS_ENABLED", "true")
}

func TestGuestActionsNeedBothSwitches(t *testing.T) {
	for _, env := range [][2]string{{"", ""}, {"true", ""}, {"", "true"}, {"false", "true"}} {
		t.Setenv("MM_NETWORK_CONTROLS_ENABLED", env[0])
		t.Setenv("MM_GUEST_ACTIONS_ENABLED", env[1])
		if guestActionsEnabled() {
			t.Fatalf("enabled with %v", env)
		}
	}
	guestActionsOn(t)
	if !guestActionsEnabled() {
		t.Fatal("both switches on should enable")
	}
}

func callRouterAction(h *RouterHandler, biz, device, body string, endpoint gin.HandlerFunc) *httptest.ResponseRecorder {
	_ = h
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/routers/"+device+"/guests/x", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(auth.ContextKeyBusinessID, biz)
	c.Set(auth.ContextKeyUserRole, "OWNER")
	c.Params = gin.Params{{Key: "id", Value: device}}
	endpoint(c)
	return w
}

const kickBody = `{"id":"*1","mac":"aa:bb:cc:dd:ee:01","address":"10.0.0.2","user":"GUEST","confirm":true}`
const macBody = `{"mac":"aa:bb:cc:dd:ee:01","confirm":true}`

func TestGuestActionsStayRefusedWhileTheSwitchIsOff(t *testing.T) {
	db := harness_testdb(t)
	t.Setenv("MM_GUEST_ACTIONS_ENABLED", "")
	biz, device := loginFixture(t, db)
	h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)
	fake := &guestActionsFake{}
	h.EnableGuestActions(fake)
	for name, call := range map[string]func() *httptest.ResponseRecorder{
		"kick": func() *httptest.ResponseRecorder {
			return callRouterAction(h, biz, device, kickBody, h.DisconnectGuest)
		},
		"ban":     func() *httptest.ResponseRecorder { return callRouterAction(h, biz, device, macBody, h.BlockGuest) },
		"unblock": func() *httptest.ResponseRecorder { return callRouterAction(h, biz, device, macBody, h.UnblockGuest) },
	} {
		if w := call(); w.Code != 503 || strings.Contains(w.Body.String(), `"verified":true`) {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if len(fake.sessions)+len(fake.blocked)+len(fake.unblock) != 0 {
		t.Fatalf("reached the router: %+v", fake)
	}
}

func TestGuestActionsReachTheRouterOnlyForTheTenantsGateway(t *testing.T) {
	db := harness_testdb(t)
	guestActionsOn(t)
	biz, device := loginFixture(t, db)
	foreign, _ := loginFixture(t, db)
	h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)
	fake := &guestActionsFake{}
	h.EnableGuestActions(fake)

	if w := callRouterAction(h, foreign, device, macBody, h.BlockGuest); w.Code != 404 || len(fake.blocked) != 0 {
		t.Fatalf("foreign tenant: %d %+v", w.Code, fake)
	}
	if w := callRouterAction(h, biz, device, `{"mac":"aa:bb:cc:dd:ee:01"}`, h.BlockGuest); w.Code != 400 || len(fake.blocked) != 0 {
		t.Fatalf("no confirmation: %d", w.Code)
	}
	if w := callRouterAction(h, biz, device, macBody, h.BlockGuest); w.Code != 200 || !strings.Contains(w.Body.String(), `"verified":true`) || fake.blocked[0] != "AA:BB:CC:DD:EE:01" {
		t.Fatalf("ban: %d %s", w.Code, w.Body.String())
	}
	if w := callRouterAction(h, biz, device, macBody, h.UnblockGuest); w.Code != 200 || fake.unblock[0] != "AA:BB:CC:DD:EE:01" {
		t.Fatalf("unban: %d %s", w.Code, w.Body.String())
	}
	if w := callRouterAction(h, biz, device, kickBody, h.DisconnectGuest); w.Code != 200 || fake.sessions[0].MAC != "AA:BB:CC:DD:EE:01" {
		t.Fatalf("kick: %d %s", w.Code, w.Body.String())
	}
	fake.err = routeros.ErrGuestSessionChanged
	if w := callRouterAction(h, biz, device, kickBody, h.DisconnectGuest); w.Code != 409 || strings.Contains(w.Body.String(), `"verified":true`) {
		t.Fatalf("changed session: %d %s", w.Code, w.Body.String())
	}
	fake.err = routeros.ErrGuestBindingForeign
	if w := callRouterAction(h, biz, device, macBody, h.BlockGuest); w.Code != 409 {
		t.Fatalf("foreign binding: %d", w.Code)
	}
	fake.err = errors.New("timeout")
	if w := callRouterAction(h, biz, device, kickBody, h.DisconnectGuest); w.Code != 502 || strings.Contains(w.Body.String(), `"verified":true`) {
		t.Fatalf("router failure: %d %s", w.Code, w.Body.String())
	}
}

func callRetire(h *GuestVoucherHandler, biz, id, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/guest-vouchers/codes/"+id+"/retire", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(auth.ContextKeyBusinessID, biz)
	c.Set(auth.ContextKeyUserRole, "OWNER")
	c.Params = gin.Params{{Key: "voucherId", Value: id}}
	h.RetireVoucherLogin(c)
	return w
}

func issuedVoucher(t *testing.T, h *GuestVoucherHandler, biz, device string) (string, string) {
	t.Helper()
	h.Provisioner = &uncertainBatchProvisioner{installed: map[string]VoucherLoginSpec{}}
	id := batchTestID()
	if w := batchTestCall(h, "POST", biz, id, batchTestBody(id, device, 1000), h.Issue); w.Code >= 300 {
		t.Fatalf("issue: %d %s", w.Code, w.Body.String())
	}
	body := callVoucherPage(t, h.Vouchers, biz, "page=1&batch="+id)
	return body.Vouchers[0].ID, body.Vouchers[0].Code
}

func TestRetireRemovesTheLoginAndKeepsTheVoucherAsRevoked(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	h := loginHandler(db)
	fake := &retirerFake{}
	h.Retirer = fake
	id, code := issuedVoucher(t, h, biz, device)

	t.Setenv("MM_GUEST_ACTIONS_ENABLED", "")
	if w := callRetire(h, biz, id, `{"confirm":true}`); w.Code != 503 || len(fake.logins) != 0 {
		t.Fatalf("switch off: %d %s", w.Code, w.Body.String())
	}
	guestActionsOn(t)
	if w := callRetire(h, biz, id, `{}`); w.Code != 400 {
		t.Fatalf("no confirmation: %d", w.Code)
	}
	other, _ := loginFixture(t, db)
	if w := callRetire(h, other, id, `{"confirm":true}`); w.Code != 404 || len(fake.logins) != 0 {
		t.Fatalf("foreign tenant: %d", w.Code)
	}
	fake.err = routeros.ErrVoucherLoginInUse
	if w := callRetire(h, biz, id, `{"confirm":true}`); w.Code != 409 {
		t.Fatalf("used on router: %d", w.Code)
	}
	fake.err = errors.New("timeout")
	if w := callRetire(h, biz, id, `{"confirm":true}`); w.Code != 502 {
		t.Fatalf("router failure: %d", w.Code)
	}
	if got := voucherStatus(t, db, biz, id); got != "unused" {
		t.Fatalf("status changed after a failed removal: %s", got)
	}
	fake.err, fake.logins = nil, nil
	w := callRetire(h, biz, id, `{"confirm":true}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"verified":true`) {
		t.Fatalf("retire: %d %s", w.Code, w.Body.String())
	}
	if len(fake.logins) != 1 || fake.logins[0].Code != code || fake.device != device {
		t.Fatalf("wrong login: %+v", fake)
	}
	if got := voucherStatus(t, db, biz, id); got != "revoked" {
		t.Fatalf("status = %s", got)
	}
	var kept int
	_ = db.QueryRow(`SELECT count(*) FROM guest_voucher_issue_items WHERE business_id=$1 AND voucher_id=$2`, biz, id).Scan(&kept)
	if kept != 1 {
		t.Fatal("issue record lost; batch recovery depends on it")
	}
	fake.logins = nil
	if w := callRetire(h, biz, id, `{"confirm":true}`); w.Code != 409 || len(fake.logins) != 0 {
		t.Fatalf("second delete: %d", w.Code)
	}
}

func TestRetireRefusesACodeInAnUnfinishedBatch(t *testing.T) {
	db := harness_testdb(t)
	guestActionsOn(t)
	biz, device := loginFixture(t, db)
	h := loginHandler(db)
	fake := &retirerFake{}
	h.Retirer = fake
	h.Provisioner = &uncertainBatchProvisioner{installed: map[string]VoucherLoginSpec{}, fail: true}
	batch := batchTestID()
	batchTestCall(h, "POST", biz, batch, batchTestBody(batch, device, 1000), h.Issue)
	body := callVoucherPage(t, h.Vouchers, biz, "page=1&batch="+batch)
	if w := callRetire(h, biz, body.Vouchers[0].ID, `{"confirm":true}`); w.Code != 409 || len(fake.logins) != 0 {
		t.Fatalf("unfinished batch: %d %s", w.Code, w.Body.String())
	}
}

func voucherStatus(t *testing.T, db *database.DB, biz, id string) string {
	t.Helper()
	var s string
	if err := db.QueryRow(`SELECT status FROM vouchers WHERE business_id=$1 AND id=$2`, biz, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func callHandOut(h *VoucherHandler, biz, id, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("PUT", "/vouchers/"+id+"/hand-out", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(auth.ContextKeyBusinessID, biz)
	c.Set(auth.ContextKeyUserRole, "OWNER")
	c.Params = gin.Params{{Key: "id", Value: id}}
	h.SetVoucherHandout(c)
	return w
}

func TestHandOutRecordsAndClearsPerTenant(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	foreign, _ := loginFixture(t, db)
	h := loginHandler(db)
	id, _ := issuedVoucher(t, h, biz, device)

	// Before the migration the screen must show it as unavailable.
	if _, err := db.Exec(`DROP TABLE IF EXISTS voucher_handouts`); err != nil {
		t.Fatal(err)
	}
	if w := callHandOut(h.Vouchers, biz, id, `{"handed_out":true}`); w.Code != 503 {
		t.Fatalf("no table: %d %s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile("../../db/migrations/20261105100000_voucher_handouts.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // The migration must be safe to re-run.
		if _, err := db.Exec(string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	if w := callHandOut(h.Vouchers, foreign, id, `{"handed_out":true}`); w.Code != 404 {
		t.Fatalf("foreign tenant: %d", w.Code)
	}
	if w := callHandOut(h.Vouchers, biz, id, `{}`); w.Code != 400 {
		t.Fatalf("no value: %d", w.Code)
	}
	for i := 0; i < 2; i++ { // Repeating is harmless.
		if w := callHandOut(h.Vouchers, biz, id, `{"handed_out":true}`); w.Code != 200 {
			t.Fatalf("hand out: %d %s", w.Code, w.Body.String())
		}
	}
	read := func() (bool, bool) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/vouchers?page=1", nil)
		c.Set(auth.ContextKeyBusinessID, biz)
		h.Vouchers.ListVouchers(c)
		var body struct {
			HandedOut map[string]string `json:"handed_out"`
			Enabled   bool              `json:"hand_out_enabled"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		_, ok := body.HandedOut[id]
		return ok, body.Enabled
	}
	if on, enabled := read(); !on || !enabled {
		t.Fatalf("not recorded: %v %v", on, enabled)
	}
	if w := callHandOut(h.Vouchers, biz, id, `{"handed_out":false}`); w.Code != 200 {
		t.Fatalf("undo: %d", w.Code)
	}
	if on, _ := read(); on {
		t.Fatal("not cleared")
	}
	if _, err := db.Exec(`UPDATE vouchers SET status='used' WHERE id=$1`, id); err == nil {
		if w := callHandOut(h.Vouchers, biz, id, `{"handed_out":true}`); w.Code != 409 {
			t.Fatalf("used code: %d", w.Code)
		}
	}
}
