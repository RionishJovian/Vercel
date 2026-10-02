// guest-cookie-check exercises the real anonymous purchase handler and RLS
// repositories in a disposable database, with a fake router (no guest access).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
)

type fakeRouter struct{ grants []routeros.GuestGrace }

func (r *fakeRouter) ConfigureGuestGrace(_ context.Context, _, _ string, g routeros.GuestGrace) error {
	r.grants = append(r.grants, g)
	return nil
}
func (*fakeRouter) DecideGuestAccess(context.Context, string, string, string, string, bool, int, int) error {
	return nil
}

func main() {
	path := flag.String("config", "config.yaml", "configuration supplying database connection")
	schema := flag.String("schema", "database/init.sql", "schema loaded only into the disposable database")
	flag.Parse()
	if err := run(*path, *schema); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path, schema string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("test configuration unavailable")
	}
	admin, err := database.New(cfg.Database)
	if err != nil {
		return fmt.Errorf("test database connection unavailable")
	}
	defer admin.Close()
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("test database name unavailable")
	}
	name := "mega_magic_test_" + hex.EncodeToString(raw)
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		return fmt.Errorf("cannot create disposable test database")
	}
	defer func() { _, _ = admin.Exec("DROP DATABASE " + name) }()
	testCfg := cfg.Database
	testCfg.Name = name
	db, err := database.New(testCfg)
	if err != nil {
		return fmt.Errorf("cannot connect disposable database")
	}
	defer db.Close()
	init, err := os.ReadFile(schema)
	if err != nil {
		return fmt.Errorf("test schema unavailable")
	}
	if _, err := db.Exec(string(init)); err != nil {
		return fmt.Errorf("disposable schema did not apply")
	}
	var business, device string
	if err := db.QueryRow(`INSERT INTO businesses(company_name,owner_email) VALUES ('Cookie test', $1) RETURNING business_id`, name+"@example.test").Scan(&business); err != nil {
		return fmt.Errorf("business fixture failed")
	}
	if err := db.QueryRow(`INSERT INTO devices(business_id,hardware_name) VALUES ($1,'Cookie test gateway') RETURNING device_id`, business).Scan(&device); err != nil {
		return fmt.Errorf("device fixture failed")
	}
	configRepo := database.NewPortalConfigRepository(db)
	if err := configRepo.Upsert(&model.PortalConfig{BusinessID: business, BusinessName: "Test", MobilePaymentEnabled: true, GuestGraceMinutes: 1, GuestGraceDataMB: 50, GuestPlans: []model.GuestPlan{{Key: "daily", Label: "Daily", Amount: 1000, DataMB: 75}}}); err != nil {
		return fmt.Errorf("portal fixture failed")
	}
	h := handler.NewGuestWifiHandler(db, database.NewDeviceRepository(db), database.NewBusinessTelegramBotRepository(db), database.NewGuestPurchaseRequestRepository(db), configRepo, database.NewBusinessMeteredQuotaRepository(db), "", nil)
	if err := h.EnableGuestCookies(cfg.Security.JWTSecret); err != nil {
		return fmt.Errorf("cookie signing unavailable")
	}
	router := &fakeRouter{}
	h.EnableGuestGrace(router)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v1/portal/:deviceId/guest-purchase", h.PublicGuestPurchase)
	post := func(mac string, cookie *http.Cookie) *httptest.ResponseRecorder {
		body := url.Values{"plan_key": {"daily"}, "client_mac": {mac}}
		req := httptest.NewRequest("POST", "https://portal.example/api/v1/portal/"+device+"/guest-purchase", strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}
	first := post("AA:BB:CC:DD:EE:FE", nil)
	if first.Code != 201 || len(router.grants) != 1 || len(first.Result().Cookies()) != 1 {
		return fmt.Errorf("initial grant or cookie failed")
	}
	cookie := first.Result().Cookies()[0]
	var payload struct {
		ID      string    `json:"request_id"`
		Expires time.Time `json:"grace_expires_at"`
	}
	if json.Unmarshal(first.Body.Bytes(), &payload) != nil || payload.ID == "" {
		return fmt.Errorf("initial response invalid")
	}
	if cookie.Name != "__Secure-mm_guest_grace" || !cookie.Secure || !cookie.HttpOnly {
		return fmt.Errorf("cookie safety attributes failed")
	}
	fmt.Println("PASS: initial committed grant and secure recognition cookie")
	resume := post("AA:BB:CC:DD:EE:FE", cookie)
	var resumed struct {
		ID      string    `json:"request_id"`
		Expires time.Time `json:"grace_expires_at"`
	}
	_ = json.Unmarshal(resume.Body.Bytes(), &resumed)
	if resume.Code != 201 || len(router.grants) != 1 || resumed.ID != payload.ID || !resumed.Expires.Equal(payload.Expires) {
		return fmt.Errorf("resume verification failed: status=%d grants=%d same_request=%t expiry_delta=%s", resume.Code, len(router.grants), resumed.ID == payload.ID, resumed.Expires.Sub(payload.Expires))
	}
	fmt.Println("PASS: same-MAC resume preserves request, expiry and one router grant")
	if post("AA:BB:CC:DD:EE:FC", cookie).Code != 409 || len(router.grants) != 1 {
		return fmt.Errorf("rotated MAC received fresh grace")
	}
	forged := *cookie
	forged.Value += "x"
	if post("AA:BB:CC:DD:EE:FC", &forged).Code != 403 || len(router.grants) != 1 {
		return fmt.Errorf("forged cookie received grace")
	}
	fmt.Println("PASS: changed-MAC live window and forged cookie deny new grant")
	if _, err := db.Exec(`UPDATE guest_purchase_requests SET grace_expires_at=now()-interval '1 minute' WHERE id=$1`, payload.ID); err != nil {
		return fmt.Errorf("expiry fixture failed")
	}
	if post("AA:BB:CC:DD:EE:FC", cookie).Code != 429 || len(router.grants) != 1 {
		return fmt.Errorf("rotated MAC bypassed six-hour cooldown")
	}
	fmt.Println("PASS: expired request retains six-hour cooldown across MAC change")
	if _, err := db.Exec(`UPDATE guest_purchase_requests SET grace_expires_at=now()-interval '6 hours 1 minute' WHERE id=$1`, payload.ID); err != nil {
		return fmt.Errorf("retry fixture failed")
	}
	if post("AA:BB:CC:DD:EE:FC", cookie).Code != 201 || len(router.grants) != 2 {
		return fmt.Errorf("eligible six-hour retry failed")
	}
	if _, err := db.Exec(`UPDATE guest_purchase_requests SET status='approved' WHERE id=$1`, payload.ID); err != nil {
		return fmt.Errorf("approved fixture failed")
	}
	if post("AA:BB:CC:DD:EE:FC", cookie).Code != 409 || len(router.grants) != 2 {
		return fmt.Errorf("approved purchase obtained free grace")
	}
	fmt.Println("PASS: eligible retry works; approved original request cannot mint grace")
	fmt.Println("PASS: disposable database-backed handler verification; no real router access changed")
	return nil
}
