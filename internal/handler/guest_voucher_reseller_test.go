package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestGuestBatchResellerAssignmentPreservesVouchers(t *testing.T) {
	db := harness_testdb(t)
	biz, device := loginFixture(t, db)
	h := loginHandler(db)
	h.Provisioner = &loginProvisionStub{}
	var actor, shop, second, foreignShop string
	if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Shop fixture','fixture','OWNER')RETURNING user_id`, biz, batchTestID()+"@fixture.test").Scan(&actor); e != nil {
		t.Fatal(e)
	}
	foreign, _ := loginFixture(t, db)
	for _, v := range []struct {
		biz, prefix string
		id          *string
	}{{biz, "SHP", &shop}, {biz, "NEXT", &second}, {foreign, "SHP", &foreignShop}} {
		if e := db.QueryRow(`INSERT INTO voucher_resellers(business_id,shop_name,contact_name,code_prefix,active)VALUES($1,$2,'Fixture',$3,true)RETURNING id`, v.biz, "Shop <fixture>", v.prefix).Scan(v.id); e != nil {
			t.Fatal(e)
		}
	}
	id := batchTestID()
	body := strings.TrimSuffix(batchTestBody(id, device, 1000), "}") + fmt.Sprintf(`,"reseller_id":%q}`, shop)
	issue := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		c.Set(auth.ContextKeyBusinessID, biz)
		c.Set(auth.ContextKeyUserID, actor)
		h.Issue(c)
		return w
	}
	initial := issue(body)
	if initial.Code != 201 {
		t.Fatalf("assigned issue %d %s", initial.Code, initial.Body)
	}
	var original []*model.Voucher
	_ = json.Unmarshal(batchTestJSON(t, initial)["vouchers"], &original)
	if len(original) != 2 || original[0].ResellerID != shop || original[0].ResellerShop != "Shop <fixture>" {
		t.Fatal("issued vouchers lost shop readback")
	}
	assign := func(target, expected string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("PUT", "/", strings.NewReader(fmt.Sprintf(`{"reseller_id":%q,"expected_reseller_id":%q,"confirmed":true}`, target, expected)))
		c.Set(auth.ContextKeyBusinessID, biz)
		c.Set(auth.ContextKeyUserID, actor)
		c.Params = gin.Params{{Key: "batchId", Value: id}}
		h.BatchReseller(c)
		return w
	}
	originalBiz := biz
	biz = foreign
	if w := assign(foreignShop, ""); w.Code != 404 {
		t.Fatal("foreign batch exposed", w.Code)
	}
	biz = originalBiz
	if w := assign(foreignShop, shop); w.Code != 409 {
		t.Fatal("foreign shop accepted", w.Code)
	}
	if _, e := db.Exec(`UPDATE voucher_resellers SET active=false WHERE id=$1`, second); e != nil {
		t.Fatal(e)
	}
	if w := assign(second, shop); w.Code != 409 {
		t.Fatal("inactive shop accepted", w.Code)
	}
	if _, e := db.Exec(`UPDATE voucher_resellers SET active=true WHERE id=$1`, second); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- assign(second, shop).Code }()
	}
	wg.Wait()
	close(results)
	for code := range results {
		if code != 200 {
			t.Fatal("idempotent assignment retry failed", code)
		}
	}
	if w := assign(shop, shop); w.Code != 409 {
		t.Fatal("stale overwrite accepted", w.Code)
	}
	var events int
	_ = db.QueryRow(`SELECT count(*) FROM guest_voucher_batch_reseller_events WHERE batch_id=$1`, id).Scan(&events)
	if events != 2 {
		t.Fatal("duplicate audit event", events)
	}
	retry := issue(body)
	if retry.Code != 201 {
		t.Fatal("original issuance request lost idempotency", retry.Code)
	}
	var current []*model.Voucher
	_ = json.Unmarshal(batchTestJSON(t, retry)["vouchers"], &current)
	if len(current) != 2 || current[0].ResellerID != second || current[0].Code != original[0].Code || current[0].Status != "unused" || current[0].FirstSeenAt != nil || current[0].ExpiresAt != nil {
		t.Fatal("assignment changed voucher state or reverted shop")
	}
	list := batchTestCall(h, "GET", biz, id, "", h.ListIssueBatches)
	if list.Code != 200 || !strings.Contains(list.Body.String(), second) {
		t.Fatal("stock assignment missing", list.Body)
	}
	e := runInTenant(nilContextForResellerTest(), db, biz, func(tx *sql.Tx) error {
		return database.NewVoucherResellerRepository(db).DeleteResellerTXN(tx, biz, second)
	})
	if e != database.ErrResellerAssigned {
		t.Fatal("assigned shop deletion was not explained", e)
	}
	if w := assign("", second); w.Code != 200 {
		t.Fatal("unassign failed", w.Code)
	}
}
func nilContextForResellerTest() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	return c
}
