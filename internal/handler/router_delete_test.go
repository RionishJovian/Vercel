package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
)

func TestDBRouterDeletionPreservesLinkedHistory(t *testing.T) {
	db := harness_testdb(t)
	business, foreign := insertBusiness(db), insertBusiness(db)
	h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)
	gin.SetMode(gin.TestMode)
	remove := func(id, tenant, role string) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyBusinessID, tenant)
			c.Set(auth.ContextKeyUserRole, role)
		})
		r.DELETE("/routers/:id", auth.RequireMinRole(model.RoleAdmin), h.DeleteRouter)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/routers/"+id, nil))
		return w
	}
	for _, history := range []string{"none", "grant", "batch", "recovery"} {
		t.Run(history, func(t *testing.T) {
			device := insertGatewayNoSite(db, business)
			var voucher string
			if history == "grant" || history == "recovery" {
				if err := db.QueryRow(`INSERT INTO vouchers(business_id,device_id,plan_key,plan_label,code,price_mmk)
VALUES($1,$2,'guest-internet','Daily',$3,500) RETURNING id`, business, device, "DELETE-"+randHex(6)).Scan(&voucher); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO guest_voucher_grants(voucher_id,business_id,device_id,minutes,data_mb) VALUES($1,$2,$3,60,1024)`, voucher, business, device); err != nil {
					t.Fatal(err)
				}
			}
			if history == "recovery" {
				if _, err := db.Exec(`INSERT INTO guest_access_recovery(voucher_id,business_id,device_id,secret_hash,current_mac) VALUES($1,$2,$3,decode(repeat('ab',32),'hex'),'02:00:00:00:00:01')`, voucher, business, device); err != nil {
					t.Fatal(err)
				}
			}
			if history == "batch" {
				if _, err := db.Exec(`INSERT INTO guest_voucher_issue_batches(business_id,id,device_id,request_hash,plan_label,price_mmk,minutes,data_mb,total_count) VALUES($1,uuid_generate_v4(),$2,repeat('a',64),'Daily',500,60,1024,1)`, business, device); err != nil {
					t.Fatal(err)
				}
			}
			for _, denied := range []struct {
				tenant, role string
				status       int
			}{{foreign, "OWNER", 404}, {business, "VIEWER", 403}} {
				w := remove(device, denied.tenant, denied.role)
				if w.Code != denied.status || strings.Contains(w.Body.String(), "history") {
					t.Fatalf("unauthorized deletion disclosed history or escaped boundary: %d", w.Code)
				}
			}
			w := remove(device, business, "OWNER")
			want, count := 409, 1
			if history == "none" {
				want, count = 200, 0
			}
			if w.Code != want {
				t.Fatalf("status %d, want %d: %s", w.Code, want, w.Body.String())
			}
			if history != "none" && (!strings.Contains(w.Body.String(), "No records were deleted") || strings.Contains(w.Body.String(), "_fkey")) {
				t.Fatal("missing actionable conflict or leaked database constraint")
			}
			var actual int
			if err := db.QueryRow(`SELECT count(*) FROM devices WHERE device_id=$1 AND business_id=$2`, device, business).Scan(&actual); err != nil || actual != count {
				t.Fatal("device preservation/deletion mismatch", err)
			}
			if voucher != "" {
				var unchanged bool
				if err := db.QueryRow(`SELECT v.device_id=$2 AND g.device_id=$2 AND g.minutes=60 AND g.data_mb=1024 FROM vouchers v JOIN guest_voucher_grants g ON g.voucher_id=v.id WHERE v.id=$1`, voucher, device).Scan(&unchanged); err != nil || !unchanged {
					t.Fatal("failed deletion changed voucher ownership or allowance", err)
				}
			}
			if history == "batch" || history == "recovery" {
				table := "guest_voucher_issue_batches"
				if history == "recovery" {
					table = "guest_access_recovery"
				}
				if err := db.QueryRow(`SELECT count(*) FROM `+table+` WHERE device_id=$1 AND business_id=$2`, device, business).Scan(&actual); err != nil || actual != 1 {
					t.Fatal("failed deletion removed linked history", err)
				}
			}
		})
	}
}
