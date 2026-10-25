package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/database"
	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

func TestGuestPortalResolverMissingDatabase(t *testing.T) {
	if cfg, err := GuestPortalConfigResolver(nil)(context.Background(), "tenant", "device"); cfg != nil || err == nil {
		t.Fatal("missing database must fail closed")
	}
}

func TestDBGuestPortalPublishedPolicyMatchesVoucherAndResume(t *testing.T) {
	db := harness_testdb(t)
	schema, err := os.ReadFile("../../migrations/20261016170000_site_portals.sql")
	if err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err = db.QueryRow(`SELECT to_regclass('public.site_portals') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		if _, err = db.Exec(string(schema)); err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	configRepo := database.NewPortalConfigRepository(db)
	deviceRepo := database.NewDeviceRepository(db)
	resolve := GuestPortalConfigResolver(db)
	for _, tc := range []struct {
		name                                                                              string
		businessAllows, siteAllows, siteConsent, published, consent, unassigned, inactive bool
		want                                                                              int
	}{
		{name: "published site enables voucher with consent", siteAllows: true, siteConsent: true, published: true, consent: true, want: 200},
		{name: "site terms override business without terms", siteAllows: true, siteConsent: true, published: true, want: 400},
		{name: "published site disables business-enabled voucher", businessAllows: true, published: true, consent: true, want: 403},
		{name: "unpublished disabled draft preserves business fallback", businessAllows: true, want: 200},
		{name: "unassigned gateway uses business policy", businessAllows: true, unassigned: true, want: 200},
		{name: "inactive business still cannot redeem", siteAllows: true, published: true, consent: true, inactive: true, want: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			biz := insertBusiness(db)
			var site, device string
			if err := db.QueryRow(`INSERT INTO sites(business_id,name) VALUES($1,'Test site') RETURNING site_id`, biz).Scan(&site); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`INSERT INTO devices(business_id,hardware_name,site_id) VALUES($1,'Gateway',$2) RETURNING device_id`, biz, site).Scan(&device); err != nil {
				t.Fatal(err)
			}
			if tc.unassigned {
				if _, err := db.Exec(`UPDATE devices SET site_id=NULL WHERE device_id=$1`, device); err != nil {
					t.Fatal(err)
				}
			}
			legacy := &model.PortalConfig{BusinessID: biz, BusinessName: "Business fallback", VoucherLoginEnabled: tc.businessAllows}
			if err := withTenant(db, biz, func(tx *sql.Tx) error { return configRepo.UpsertTXN(tx, legacy) }); err != nil {
				t.Fatal(err)
			}
			published := &model.PortalConfig{BusinessID: biz, BusinessName: "Published site", VoucherLoginEnabled: tc.siteAllows, ConsentRequired: tc.siteConsent, ConsentText: "Site terms"}
			if err := withTenant(db, biz, func(tx *sql.Tx) error {
				repo := database.NewSitePortalRepository()
				if err := repo.UpsertDraftTXN(tx, &model.SitePortal{SiteID: site, BusinessID: biz, DraftConfig: published}); err != nil {
					return err
				}
				if tc.published {
					if err := repo.PublishTXN(tx, biz, site); err != nil {
						return err
					}
				}
				// A later opposite draft must not change the effective policy.
				return repo.UpsertDraftTXN(tx, &model.SitePortal{SiteID: site, BusinessID: biz, DraftConfig: &model.PortalConfig{BusinessID: biz, BusinessName: "Unpublished opposite draft", VoucherLoginEnabled: !tc.siteAllows, ConsentRequired: !tc.siteConsent}})
			}); err != nil {
				t.Fatal(err)
			}
			if tc.inactive {
				if _, err := db.Exec(`UPDATE businesses SET is_active=false WHERE business_id=$1`, biz); err != nil {
					t.Fatal(err)
				}
			}
			public := &PortalHandler{db: db, deviceRepo: deviceRepo, configRepo: configRepo}
			redeemer := &guestRedeemStub{}
			resumer := &resumeHandlerStub{result: guestaccess.ResumeResult{ExpiresAt: time.Now().Add(time.Hour), RemainingBytes: 1024}}
			h := &GuestVoucherHandler{ResolveDevice: deviceRepo.GetDeviceByID, PortalConfig: resolve, Redeemer: redeemer, Resumer: resumer}
			r := gin.New()
			r.GET("/portal/:deviceId/config", public.PublicConfig)
			r.POST("/portal/:deviceId/guest-voucher", h.Redeem)
			r.POST("/portal/:deviceId/guest-resume", h.Resume)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/portal/"+device+"/config", nil))
			if w.Code != 200 {
				t.Fatal(w.Body.String())
			}
			var shown struct {
				Config model.PortalConfig `json:"config"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &shown); err != nil {
				t.Fatal(err)
			}
			wantShown := published
			if !tc.published || tc.unassigned {
				wantShown = legacy
			}
			if shown.Config.BusinessName != wantShown.BusinessName || shown.Config.VoucherLoginEnabled != wantShown.VoucherLoginEnabled || shown.Config.ConsentRequired != wantShown.ConsentRequired {
				t.Fatalf("guest saw wrong policy: %+v", shown.Config)
			}
			secret, err := guestaccess.RecoverySecret()
			if err != nil {
				t.Fatal(err)
			}
			for _, action := range []string{"guest-voucher", "guest-resume"} {
				consent := "false"
				if tc.consent {
					consent = "true"
				}
				body := `{"code":"CODE","client_mac":"02:00:00:00:00:01","consent":` + consent + `,"business_id":"attacker","site_id":"attacker"}`
				q := httptest.NewRequest("POST", "/portal/"+device+"/"+action, strings.NewReader(body))
				q.Header.Set("Content-Type", "application/json")
				q.AddCookie(&http.Cookie{Name: guestRecoveryCookieName, Value: "22222222-2222-4222-8222-222222222222." + secret})
				w = httptest.NewRecorder()
				r.ServeHTTP(w, q)
				if w.Code != tc.want {
					t.Fatalf("%s: status=%d wanted=%d body=%s", action, w.Code, tc.want, w.Body.String())
				}
			}
			wantCalls := 0
			if tc.want == 200 {
				wantCalls = 1
			}
			if redeemer.calls != wantCalls || resumer.calls != wantCalls {
				t.Fatalf("policy denial reached provisioning: redeem=%d resume=%d", redeemer.calls, resumer.calls)
			}
			if !tc.inactive {
				foreign := insertBusiness(db)
				if cfg, err := resolve(context.Background(), foreign, device); err != nil || cfg != nil {
					t.Fatalf("foreign device policy leaked: cfg=%+v err=%v", cfg, err)
				}
				if cfg, err := resolve(context.Background(), biz, "11111111-1111-4111-8111-111111111111"); err != nil || cfg != nil {
					t.Fatalf("missing device used business fallback: cfg=%+v err=%v", cfg, err)
				}
			}
		})
	}
}
