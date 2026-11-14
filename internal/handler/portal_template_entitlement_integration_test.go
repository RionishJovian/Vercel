package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/database"
)

// Runs only with MM_TEST_DB_URL (a throwaway database with baseline + migrations applied).
func TestPortalTemplateEntitlementOverHTTP(t *testing.T) {
	db := harness_testdb(t)
	gin.SetMode(gin.TestMode)
	biz := insertBusiness(db)
	var user string
	if err := db.QueryRow(`INSERT INTO users (business_id, full_name, email, password_hash, user_role) VALUES ($1,'Owner',$2,'x','OWNER') RETURNING user_id`, biz, "own-"+randHex(4)+"@example.test").Scan(&user); err != nil {
		t.Fatal(err)
	}
	h := &PortalHandler{db: db, configRepo: database.NewPortalConfigRepository(db)}
	put := func(role, template string) int {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("business_id", biz)
		c.Set("user_id", user)
		c.Set("user_role", role)
		c.Request = httptest.NewRequest(http.MethodPut, "/portal/config", bytes.NewBufferString(`{"business_name":"Cafe","template_id":"`+template+`"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		h.StaffUpdateConfig(c)
		return w.Code
	}
	for _, free := range []string{"starlink_mm", "daylight"} {
		if code := put("OWNER", free); code != http.StatusOK {
			t.Fatalf("free template %s: %d", free, code)
		}
	}
	for _, id := range premiumTemplates {
		if code := put("OWNER", id); code != http.StatusForbidden {
			t.Fatalf("%s must be 403 before purchase, got %d", id, code)
		}
	}
	if code := put("DEVELOPER", "hotel"); code != http.StatusOK {
		t.Fatalf("platform administrator must be exempt, got %d", code)
	}
	if code := put("OWNER", "daylight"); code != http.StatusOK {
		t.Fatal(code)
	}
	var addon string
	if err := db.QueryRow(`SELECT id FROM store_addons WHERE addon_key='portal_theme_cafe'`).Scan(&addon); err != nil {
		t.Fatal(err)
	}
	var req string
	if err := db.QueryRow(`INSERT INTO store_addon_requests (business_id,user_id,addon_id,price_mmk,period,bank_reference,idempotency_key) VALUES ($1,$2,$3,5000,'one_time','REF-12345',$4) RETURNING id`, biz, user, addon, "idem-"+randHex(6)).Scan(&req); err != nil {
		t.Fatal(err)
	}
	if code := put("OWNER", "cafe"); code != http.StatusForbidden {
		t.Fatalf("a pending request must not unlock, got %d", code)
	}
	if _, err := db.Exec(`UPDATE store_addon_requests SET status='approved' WHERE id=$1`, req); err != nil {
		t.Fatal(err)
	}
	if code := put("OWNER", "cafe"); code != http.StatusOK {
		t.Fatalf("approved add-on must unlock immediately, got %d", code)
	}
	if code := put("OWNER", "hotel"); code != http.StatusForbidden {
		t.Fatalf("another theme stays locked, got %d", code)
	}
}
