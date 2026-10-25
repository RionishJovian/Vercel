package server

import (
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/config"
	"mega-magic/internal/model"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDBMediaPublicationIndependentOfConnector(t *testing.T) {
	db := subscriptionHTTPDatabase(t)
	raw, err := os.ReadFile("../../migrations/20261003120000_magic_media_publications.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(raw)); err != nil {
		t.Fatal(err)
	}
	svc, _ := auth.NewService("fixture-media-publication-auth-secret", time.Hour, time.Hour, 4)
	svc.UseSessions(db)
	var biz string
	if err := db.QueryRow(`INSERT INTO businesses(company_name,owner_email,access_tier,device_quota_limit) VALUES('Media fixture','media-fixture@example.test','SMALL_WISP',10) RETURNING business_id`).Scan(&biz); err != nil {
		t.Fatal(err)
	}
	tokens := map[model.UserRole]string{}
	for _, role := range []model.UserRole{model.RoleOwner, model.RoleViewer} {
		u := &model.User{BusinessID: biz, UserRole: role, PasswordHash: "fixture"}
		if err := db.QueryRow(`INSERT INTO users(business_id,email,full_name,password_hash,user_role) VALUES($1,$2,'Fixture','fixture',$3) RETURNING user_id`, biz, string(role)+"@media.test", role).Scan(&u.UserID); err != nil {
			t.Fatal(err)
		}
		token, _, err := svc.StartSession(u)
		if err != nil {
			t.Fatal(err)
		}
		tokens[role] = token
	}
	cfg := &config.Config{}
	cfg.Media.PublishingEnabled = true
	r := gin.New()
	if err := registerMediaPublishingRoutes(r, svc, db, cfg); err != nil {
		t.Fatal(err)
	}
	request := func(method, path string, role model.UserRole, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tokens[role])
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	if w := request("GET", "/api/v1/media/capabilities", model.RoleOwner, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"publication":{"available":true`) || !strings.Contains(w.Body.String(), `"connector":{"available":false}`) || !strings.Contains(w.Body.String(), `"paid_access":{"available":false`) {
		t.Fatalf("capabilities %d %s", w.Code, w.Body)
	}
	if w := request("GET", "/api/v1/media/publication", model.RoleOwner, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"publication":null`) {
		t.Fatal(w.Code, w.Body)
	}
	if w := request("PUT", "/api/v1/media/publication?revision=0", model.RoleViewer, `{}`); w.Code != 403 {
		t.Fatal("viewer write", w.Code)
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM magic_media_publications`).Scan(&n)
	if n != 0 {
		t.Fatal("denied write mutated database")
	}
	design := `{"name":"Test cinema","headline":"Welcome","accent":"#eeeeee","theme":"charcoal","price":1500,"duration":24,"layout":"shelves","posterDensity":"comfortable","movies":true}`
	if w := request("PUT", "/api/v1/media/publication?revision=0", model.RoleOwner, design); w.Code != 200 {
		t.Fatalf("independent publication: %d %s", w.Code, w.Body)
	}
	if w := request("PUT", "/api/v1/media/publication?revision=0", model.RoleOwner, design); w.Code != 409 {
		t.Fatalf("stale publication: %d", w.Code)
	}
	if w := request("GET", "/api/v1/media/connection", model.RoleOwner, ""); w.Code != 404 {
		t.Fatal("connector implicitly enabled", w.Code)
	}
}
