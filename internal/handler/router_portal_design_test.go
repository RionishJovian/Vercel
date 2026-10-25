package handler

import (
	"database/sql"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRouterPortalPreviewIsInertAndExcludesPaymentSecrets(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/portal/router-preview", strings.NewReader(`{"business_name":"Cafe <script>bad()</script>","welcome_text":"Saved greeting","primary_hex":"#00745e","glass_tint_hex":"#edf4f7","bank_account":"NEVER-IN-ROUTER-HTML","consent_required":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	(&PortalHandler{}).RouterPreview(c)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var body struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.HTML, `data-preview="true"`) || strings.Contains(body.HTML, "NEVER-IN-ROUTER-HTML") || strings.Contains(body.HTML, "Cafe <script>") || strings.Contains(body.HTML, "$(chap-id)") {
		t.Fatal("unsafe or live router preview")
	}
}

func TestDBRouterPortalDeploymentUsesPublishedTenantDesign(t *testing.T) {
	db := harness_testdb(t)
	migration, err := os.ReadFile("../../migrations/20261016170000_site_portals.sql")
	if err != nil {
		t.Fatal(err)
	}
	var siteTable sql.NullString
	if err = db.QueryRow(`SELECT to_regclass('public.site_portals')::text`).Scan(&siteTable); err != nil {
		t.Fatal(err)
	}
	if !siteTable.Valid {
		if _, err = db.Exec(string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	biz, device := loginFixture(t, db)
	foreign, _ := loginFixture(t, db)
	var site string
	if err = db.QueryRow(`INSERT INTO sites(business_id,name) VALUES($1,'Design site') RETURNING site_id`, biz).Scan(&site); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE devices SET site_id=$1 WHERE device_id=$2`, site, device); err != nil {
		t.Fatal(err)
	}
	err = withTenant(db, biz, func(tx *sql.Tx) error {
		if err := database.NewPortalConfigRepository(db).UpsertTXN(tx, &model.PortalConfig{BusinessID: biz, BusinessName: "Business fallback", VoucherLoginEnabled: true}); err != nil {
			return err
		}
		repo := database.NewSitePortalRepository()
		if err := repo.UpsertDraftTXN(tx, &model.SitePortal{SiteID: site, BusinessID: biz, DraftConfig: &model.PortalConfig{BusinessName: "Published cafe", WelcomeText: "Published welcome", VoucherLoginEnabled: true, ConsentRequired: true, ConsentText: "Published terms"}}); err != nil {
			return err
		}
		if err := repo.PublishTXN(tx, biz, site); err != nil {
			return err
		}
		return repo.UpsertDraftTXN(tx, &model.SitePortal{SiteID: site, BusinessID: biz, DraftConfig: &model.PortalConfig{BusinessName: "Private unpublished draft", VoucherLoginEnabled: true}})
	})
	if err != nil {
		t.Fatal(err)
	}
	h := &SecurityHandler{db: db}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/routers/"+device+"/portal/preview", nil)
	design, err := h.savedRouterPortalDesign(c, biz, device)
	if err != nil || design == nil || design.BusinessName != "Published cafe" || design.WelcomeText != "Published welcome" || !design.ConsentRequired {
		t.Fatalf("wrong saved policy: %+v %v", design, err)
	}
	// Optional local visual-review artifact, generated from the same published
	// design returned above; never includes a live router endpoint or credentials.
	if path := os.Getenv("MM_ROUTER_PORTAL_REVIEW_FILE"); path != "" {
		page, renderErr := routeros.RenderPortalLogin(design, true)
		if renderErr != nil {
			t.Fatal(renderErr)
		}
		if err := os.WriteFile(path, []byte(page), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = h.savedRouterPortalDesign(c, foreign, device); err == nil {
		t.Fatal("foreign device obtained a design")
	}
}
