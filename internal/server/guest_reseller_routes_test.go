package server

import (
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/handler"
	"mega-magic/internal/model"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDBGuestResellerRouteAuthorization(t *testing.T) {
	db := subscriptionHTTPDatabase(t)
	for _, path := range []string{"20261001200000_guest_voucher_grants.sql", "20261001210000_guest_access_recovery.sql", "20261002200000_guest_recovery_completion.sql", "20261021090000_guest_voucher_activation.sql", "20261021180000_guest_voucher_issue_batches.sql", "20261024170000_guest_voucher_resellers.sql"} {
		b, e := os.ReadFile("../../migrations/" + path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(b)); e != nil {
			t.Fatal(e)
		}
	}
	svc, _ := auth.NewService("reseller-route-test-signing-secret", time.Hour, time.Hour, 4)
	svc.UseSessions(db)
	var biz string
	if e := db.QueryRow(`INSERT INTO businesses(company_name,owner_email,access_tier)VALUES('Route fixture','route@fixture.test','SMALL_WISP')RETURNING business_id`).Scan(&biz); e != nil {
		t.Fatal(e)
	}
	tokens := map[model.UserRole]string{}
	for _, role := range []model.UserRole{model.RoleOwner, model.RoleViewer} {
		u := &model.User{BusinessID: biz, UserRole: role, PasswordHash: "fixture"}
		if e := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role)VALUES($1,$2,'Fixture','fixture',$3)RETURNING user_id`, biz, string(role)+"@reseller.test", role).Scan(&u.UserID); e != nil {
			t.Fatal(e)
		}
		token, _, e := svc.StartSession(u)
		if e != nil {
			t.Fatal(e)
		}
		tokens[role] = token
	}
	r := gin.New()
	registerGuestVoucherEndpoints(r, svc, db, &handler.GuestVoucherHandler{DB: db})
	for _, tc := range []struct {
		name string
		role model.UserRole
		want int
	}{{"unauthenticated", "", 401}, {"viewer", model.RoleViewer, 403}, {"owner absent batch", model.RoleOwner, 404}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "https://app.test/api/v1/guest-vouchers/batches/11111111-1111-4111-8111-111111111111/reseller", strings.NewReader(`{"reseller_id":"","expected_reseller_id":"","confirmed":true}`))
			req.Header.Set("Content-Type", "application/json")
			if token := tokens[tc.role]; token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status %d %s", w.Code, w.Body)
			}
		})
	}
	var n int
	if e := db.QueryRow(`SELECT count(*) FROM guest_voucher_batch_reseller_events`).Scan(&n); e != nil || n != 0 {
		t.Fatal("denied/missing requests wrote history", n, e)
	}
}
