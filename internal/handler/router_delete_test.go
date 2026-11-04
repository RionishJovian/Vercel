package handler

import (
	"context"
	"errors"
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

// Delete ends the Hub connection and hands back the cleanup script. If the Hub cannot
// end it, the record stays, so a connection can never outlive its record again.
func TestDBRouterDeletionEndsHubConnection(t *testing.T) {
	db := harness_testdb(t)
	business := insertBusiness(db)
	gin.SetMode(gin.TestMode)
	remove := func(h *RouterHandler, id string) *httptest.ResponseRecorder {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyBusinessID, business)
			c.Set(auth.ContextKeyUserID, "owner")
			c.Set(auth.ContextKeyUserRole, "OWNER")
		})
		r.DELETE("/routers/:id", h.DeleteRouter)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/routers/"+id, nil))
		return w
	}
	exists := func(id string) bool {
		var found bool
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM devices WHERE device_id=$1)`, id).Scan(&found); err != nil {
			t.Fatal(err)
		}
		return found
	}
	h := NewRouterHandler(db, database.NewDeviceRepository(db), nil, nil)

	t.Run("Hub unavailable keeps the record", func(t *testing.T) {
		device := insertGatewayNoSite(db, business)
		h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) { return "", errors.New("hub down") })
		if w := remove(h, device); w.Code != http.StatusServiceUnavailable || !exists(device) {
			t.Fatalf("status %d, record exists %v", w.Code, exists(device))
		}
	})
	t.Run("success returns the cleanup script", func(t *testing.T) {
		device := insertGatewayNoSite(db, business)
		var called string
		h.ConfigureHubRemoval(func(_ context.Context, biz, _, id string) (string, error) {
			called = biz + "/" + id
			return "cleanup-script", nil
		})
		w := remove(h, device)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"router_cleanup_script":"cleanup-script"`) || exists(device) {
			t.Fatalf("status %d body %s", w.Code, w.Body.String())
		}
		if called != business+"/"+device {
			t.Fatalf("removed the wrong peer: %s", called)
		}
	})
	t.Run("a record with history keeps its Hub connection", func(t *testing.T) {
		device := insertGatewayNoSite(db, business)
		if _, err := db.Exec(`INSERT INTO vouchers(business_id,device_id,plan_key,plan_label,code,price_mmk) VALUES($1,$2,'guest-internet','Daily',$3,500)`, business, device, "HUBDEL-"+randHex(6)); err != nil {
			t.Fatal(err)
		}
		var grantVoucher string
		if err := db.QueryRow(`SELECT id FROM vouchers WHERE device_id=$1`, device).Scan(&grantVoucher); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO guest_voucher_grants(voucher_id,business_id,device_id,minutes,data_mb) VALUES($1,$2,$3,60,1024)`, grantVoucher, business, device); err != nil {
			t.Fatal(err)
		}
		called := false
		h.ConfigureHubRemoval(func(context.Context, string, string, string) (string, error) { called = true; return "", nil })
		if w := remove(h, device); w.Code != http.StatusConflict || called || !exists(device) {
			t.Fatalf("status %d, Hub called %v", w.Code, called)
		}
	})
}
