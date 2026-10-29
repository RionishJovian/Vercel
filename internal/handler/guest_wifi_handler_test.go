package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/telegram"
)

// -----------------------------------------------------------------------------
// Self-contained DB harness for handler integration tests (mirrors the database
// package harness; kept local to avoid an import cycle between the two packages).
// -----------------------------------------------------------------------------

var (
	harnessAdmin            *database.DB
	harnessDB               *database.DB
	harnessName             string
	harnessSkip             error
	harnessFatal            error
	harnessOnce             sync.Once
	harnessRestrictedNeeded bool // unused; kept for symmetry with the DB package
)

func TestMain(m *testing.M) {
	base := os.Getenv("MM_TEST_DB_URL")
	if base != "" {
		harnessOnce.Do(func() { setupHarness(base) })
	}
	code := m.Run()
	teardownHarness()
	os.Exit(code)
}

func harness_testdb(t *testing.T) *database.DB {
	if os.Getenv("MM_TEST_DB_URL") == "" {
		t.Skip("DB integration harness skipped: set MM_TEST_DB_URL=postgres://user:***@host:port/db?sslmode=disable to run against a throwaway DB")
	}
	if harnessSkip != nil {
		t.Skipf("DB integration harness skipped: %v", harnessSkip)
	}
	if harnessFatal != nil {
		t.Fatalf("DB integration harness setup failed: %v", harnessFatal)
	}
	return harnessDB
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func setupHarness(base string) {
	cfg, err := cfgFromURL(base)
	if err != nil {
		harnessSkip = fmt.Errorf("bad MM_TEST_DB_URL: %w", err)
		return
	}
	admin, err := database.New(*cfg)
	if err != nil {
		harnessSkip = fmt.Errorf("connect: %w", err)
		return
	}
	harnessAdmin = admin
	harnessName = "mega_magic_test_" + randHex(8)
	if _, err := admin.Exec("CREATE DATABASE " + harnessName); err != nil {
		harnessSkip = fmt.Errorf("create throwaway db: %w", err)
		return
	}
	tcfg := *cfg
	tcfg.Name = harnessName
	db, err := database.New(tcfg)
	if err != nil {
		harnessSkip = fmt.Errorf("connect throwaway db: %w", err)
		return
	}
	harnessDB = db
	sqlBytes, err := os.ReadFile("../../database/init.sql")
	if err != nil {
		harnessFatal = fmt.Errorf("read schema: %w", err)
		return
	}
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		harnessFatal = fmt.Errorf("load schema into %s: %w", harnessName, err)
		return
	}
	// The live deployment carries the guest-voucher migration chain on top of
	// the baseline (grants, recovery and the activation guard), so the
	// integration tests must exercise that schema, not the bare baseline.
	for _, name := range []string{
		"20260926_guest_wifi_schema.sql",
		"20261024020000_guest_purchase_terms.sql",
		"20261024030000_app_account_passes.sql",
		"20261024040000_individual_pass_rollout.sql",
		"20261029090000_account_pass_checkout.sql",
		"20261024070000_management_address_migration.sql",
		"20260926_rls_backfill.sql",
		"20261001200000_guest_voucher_grants.sql",
		"20261001210000_guest_access_recovery.sql",
		"20261002200000_guest_recovery_completion.sql",
		"20261021090000_guest_voucher_activation.sql",
		"20261021180000_guest_voucher_issue_batches.sql",
		"20261024170000_guest_voucher_resellers.sql",
	} {
		migration, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			harnessFatal = fmt.Errorf("read migration %s: %w", name, err)
			return
		}
		if _, err := db.Exec(string(migration)); err != nil {
			harnessFatal = fmt.Errorf("load migration %s into %s: %w", name, harnessName, err)
			return
		}
	}
}

func teardownHarness() {
	if harnessDB != nil {
		_ = harnessDB.Close()
	}
	if harnessAdmin != nil && harnessName != "" {
		_, _ = harnessAdmin.Exec("DROP DATABASE IF EXISTS " + harnessName)
	}
	if harnessAdmin != nil {
		_ = harnessAdmin.Close()
	}
}

func cfgFromURL(raw string) (*config.DatabaseConfig, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return nil, fmt.Errorf("expected a postgres:// URL, got scheme %q", u.Scheme)
	}
	port := 5432
	if p := u.Port(); p != "" {
		if _, err := fmt.Sscanf(p, "%d", &port); err != nil {
			return nil, fmt.Errorf("bad port %q", p)
		}
	}
	pass, _ := u.User.Password()
	ssl := u.Query().Get("sslmode")
	if ssl == "" {
		ssl = "disable"
	}
	return &config.DatabaseConfig{
		Host:         u.Hostname(),
		Port:         port,
		User:         u.User.Username(),
		Password:     pass,
		Name:         trimSlash(u.Path),
		SSLMode:      ssl,
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	}, nil
}

func trimSlash(s string) string {
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}

func insertBusiness(db *database.DB) string {
	var id string
	err := db.QueryRow(
		`INSERT INTO businesses (company_name, owner_email, access_tier) VALUES ($1, $2, 'SMALL_WISP') RETURNING business_id`,
		"HandlerHarness Co", "hh-"+randHex(4)+"@example.test",
	).Scan(&id)
	if err != nil {
		panic("insert business: " + err.Error())
	}
	return id
}

// withTenant wraps fn in a transaction bound to businessID (RLS-scoped).
func withTenant(db *database.DB, businessID string, fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := database.WithTenant(tx, businessID, func() error { return fn(tx) }); err != nil {
		return err
	}
	return tx.Commit()
}

// -----------------------------------------------------------------------------
// d4: approving a guest purchase grants the wifi time extension EXACTLY ONCE;
// rejecting grants nothing. This is the money-redirect the whole slice exists for.
// -----------------------------------------------------------------------------
func TestGuestWifi_approveGrantsExtensionOnce(t *testing.T) {
	db := harness_testdb(t)
	t.Helper()
	h := NewGuestWifiHandler(
		db,
		database.NewDeviceRepository(db),
		database.NewBusinessTelegramBotRepository(db),
		database.NewGuestPurchaseRequestRepository(db),
		database.NewPortalConfigRepository(db),
		database.NewBusinessMeteredQuotaRepository(db),
		"/tmp/guest-receipts-"+randHex(4),
		func(token string) (telegram.Messenger, bool) { return nil, false },
	)

	biz := insertBusiness(db)
	var device string
	if e := db.QueryRow(`INSERT INTO devices(business_id,hardware_name,deployment_layer) VALUES($1,'payment-fixture','MAIN_GATEWAY') RETURNING device_id`, biz).Scan(&device); e != nil {
		t.Fatal(e)
	}
	grant := &paidTermsFixture{}
	h.EnableGuestGrace(grant)
	now := time.Now().UTC()

	// Seed portal config: one guest plan ("daily" = 120 min) + a 10-min grace.
	withTenant(db, biz, func(tx *sql.Tx) error {
		return h.configRepo.UpsertTXN(tx, &model.PortalConfig{
			BusinessID:        biz,
			BusinessName:      "Cafe A",
			GuestGraceMinutes: 10,
			GuestPlans:        []model.GuestPlan{{Key: "daily", Label: "Day", Minutes: 120, Amount: 2000}},
		})
	})

	var reqID string
	graceMax := now.Add(10 * time.Minute)
	withTenant(db, biz, func(tx *sql.Tx) error {
		r, err := h.reqRepo.CreateTXN(tx, &model.GuestPurchaseRequest{
			BusinessID: biz,
			DeviceID:   &device, PurchasedMinutes: ptr(120),
			ClientMAC:      "AA:BB:CC:DD:EE:FF",
			PlanKey:        "daily",
			Amount:         2000,
			GraceStartedAt: &now,
			GraceExpiresAt: &graceMax,
			Status:         "pending",
		})
		if err != nil {
			return err
		}
		reqID = r.ID
		return nil
	})

	if _, err := db.Exec(`DELETE FROM portal_configs WHERE business_id=$1`, biz); err != nil {
		t.Fatal(err)
	}
	// Approve via the SAME decide-core the owner's bot bridge uses.
	if err := h.DecidePurchaseRequest(biz, reqID, true); err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	if grant.calls != 1 || grant.minutes != 120 {
		t.Fatalf("wrong purchased grant: %+v", grant)
	}
	if err := h.DecidePurchaseRequest(biz, reqID, true); err == nil {
		t.Fatal("duplicate decision accepted")
	}
	if grant.calls != 1 {
		t.Fatal("duplicate granted again")
	}
	q, err := h.quotaRepo.GetByBusiness(biz)
	if err != nil {
		t.Fatal(err)
	}
	if q != nil && q.WebfigPassExpiresAt != nil {
		t.Fatal("guest payment changed business Hub pass")
	}
}

// TestGuestWifi_rejectGrantsNothing proves rejecting a guest purchase does not
// touch the wifi entitlement (no extension, request ends rejected).
func TestGuestWifi_rejectGrantsNothing(t *testing.T) {
	db := harness_testdb(t)
	t.Helper()
	h := NewGuestWifiHandler(
		db,
		database.NewDeviceRepository(db),
		database.NewBusinessTelegramBotRepository(db),
		database.NewGuestPurchaseRequestRepository(db),
		database.NewPortalConfigRepository(db),
		database.NewBusinessMeteredQuotaRepository(db),
		"/tmp/guest-receipts-"+randHex(4),
		func(token string) (telegram.Messenger, bool) { return nil, false },
	)
	biz := insertBusiness(db)
	now := time.Now().UTC()

	withTenant(db, biz, func(tx *sql.Tx) error {
		return h.configRepo.UpsertTXN(tx, &model.PortalConfig{
			BusinessID:        biz,
			BusinessName:      "Cafe B",
			GuestGraceMinutes: 10,
			GuestPlans:        []model.GuestPlan{{Key: "hourly", Label: "Hour", Minutes: 60, Amount: 500}},
		})
	})

	var reqID string
	graceMax := now.Add(10 * time.Minute)
	withTenant(db, biz, func(tx *sql.Tx) error {
		r, err := h.reqRepo.CreateTXN(tx, &model.GuestPurchaseRequest{
			BusinessID:     biz,
			PlanKey:        "hourly",
			Amount:         500,
			GraceStartedAt: &now,
			GraceExpiresAt: &graceMax,
			Status:         "pending",
		})
		if err != nil {
			return err
		}
		reqID = r.ID
		return nil
	})

	if err := h.DecidePurchaseRequest(biz, reqID, false); err != nil {
		t.Fatalf("reject failed: %v", err)
	}

	// No entitlement may exist.
	q, _ := h.quotaRepo.GetByBusiness(biz)
	if q != nil && q.WebfigPassExpiresAt != nil {
		t.Fatal("reject must not grant the wifi pass")
	}

	// Request ended rejected (readable in-tenant).
	withTenant(db, biz, func(tx *sql.Tx) error {
		r, err := h.reqRepo.GetByIDTXN(tx, biz, reqID)
		if err != nil {
			return err
		}
		if r == nil || r.Status != "rejected" {
			got := "nil"
			if r != nil {
				got = r.Status
			}
			t.Fatalf("request status = %q, want rejected", got)
		}
		return nil
	})
}

// TestGuestWifi_graceWindowBounds proves the grace window is stored on the
// request and enforced as the receipt-attach deadline. The request carries
// grace_started_at = now and grace_expires_at = now + configured minutes.
func TestGuestWifi_graceWindowBounds(t *testing.T) {
	db := harness_testdb(t)
	t.Helper()
	h := NewGuestWifiHandler(
		db,
		database.NewDeviceRepository(db),
		database.NewBusinessTelegramBotRepository(db),
		database.NewGuestPurchaseRequestRepository(db),
		database.NewPortalConfigRepository(db),
		database.NewBusinessMeteredQuotaRepository(db),
		"/tmp/guest-receipts-"+randHex(4),
		func(token string) (telegram.Messenger, bool) { return nil, false },
	)
	biz := insertBusiness(db)
	now := time.Now().UTC()

	withTenant(db, biz, func(tx *sql.Tx) error {
		return h.configRepo.UpsertTXN(tx, &model.PortalConfig{
			BusinessID:        biz,
			BusinessName:      "Cafe C",
			GuestGraceMinutes: 10,
			GuestPlans:        []model.GuestPlan{{Key: "daily", Label: "Day", Minutes: 120, Amount: 2000}},
		})
	})

	var req *model.GuestPurchaseRequest
	graceNow := now.Add(10 * time.Minute)
	graceExp2 := &graceNow
	withTenant(db, biz, func(tx *sql.Tx) error {
		r, err := h.reqRepo.CreateTXN(tx, &model.GuestPurchaseRequest{
			BusinessID:     biz,
			PlanKey:        "daily",
			Amount:         2000,
			GraceStartedAt: &now,
			GraceExpiresAt: graceExp2,
			Status:         "pending",
		})
		if err != nil {
			return err
		}
		req = r
		return nil
	})
	if req == nil || req.GraceStartedAt == nil || req.GraceExpiresAt == nil {
		t.Fatal("guest request must carry the grace window bounds")
	}
	if req.GraceExpiresAt.Sub(*req.GraceStartedAt) != 10*time.Minute {
		t.Fatalf("grace window = %v, want 10m", req.GraceExpiresAt.Sub(*req.GraceStartedAt))
	}
}

type paidTermsFixture struct{ calls, minutes, data int }

func (f *paidTermsFixture) ConfigureGuestGrace(context.Context, string, string, routeros.GuestGrace) error {
	return nil
}
func (f *paidTermsFixture) DecideGuestAccess(_ context.Context, b, d, id, mac string, approve bool, minutes, data int) error {
	f.calls++
	f.minutes = minutes
	f.data = data
	return nil
}

func TestGuestWifiPublishedPaymentTerms(t *testing.T) {
	db := harness_testdb(t)
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('public.site_portals') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		schema, err := os.ReadFile("../../migrations/20261016170000_site_portals.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(schema)); err != nil {
			t.Fatal(err)
		}
	}
	biz := insertBusiness(db)
	var site, device string
	if err := db.QueryRow(`INSERT INTO sites(business_id,name) VALUES($1,'Payment site') RETURNING site_id`, biz).Scan(&site); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO devices(business_id,hardware_name,site_id) VALUES($1,'Payment gateway',$2) RETURNING device_id`, biz, site).Scan(&device); err != nil {
		t.Fatal(err)
	}
	configRepo := database.NewPortalConfigRepository(db)
	siteRepo := database.NewSitePortalRepository()
	published := &model.PortalConfig{BusinessID: biz, BusinessName: "Published payment", MobilePaymentEnabled: true, GuestGraceMinutes: 10, GuestGraceDataMB: 50, GuestPlans: []model.GuestPlan{{Key: "daily", Label: "Daily", Minutes: 90, Amount: 1500, DataMB: 256}}}
	if err := withTenant(db, biz, func(tx *sql.Tx) error {
		if err := configRepo.UpsertTXN(tx, &model.PortalConfig{BusinessID: biz, BusinessName: "Disabled business fallback", MobilePaymentEnabled: false}); err != nil {
			return err
		}
		if err := siteRepo.UpsertDraftTXN(tx, &model.SitePortal{SiteID: site, BusinessID: biz, DraftConfig: published}); err != nil {
			return err
		}
		return siteRepo.PublishTXN(tx, biz, site)
	}); err != nil {
		t.Fatal(err)
	}
	repo := database.NewGuestPurchaseRequestRepository(db)
	h := NewGuestWifiHandler(db, database.NewDeviceRepository(db), database.NewBusinessTelegramBotRepository(db), repo, configRepo, database.NewBusinessMeteredQuotaRepository(db), t.TempDir(), func(string) (telegram.Messenger, bool) { return nil, false })
	h.EnableGuestGrace(&paidTermsFixture{})
	if err := h.EnableGuestCookies(strings.Repeat("k", 32)); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/portal/:deviceId/purchase", h.PublicGuestPurchase)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/portal/"+device+"/purchase", strings.NewReader("plan_key=daily&client_mac=02:00:00:00:01:02"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("published site offer refused: %d %s", w.Code, w.Body.String())
	}
	var minutes, amount, data int
	if err := db.QueryRow(`SELECT purchased_minutes,amount,data_mb FROM guest_purchase_requests WHERE business_id=$1`, biz).Scan(&minutes, &amount, &data); err != nil {
		t.Fatal(err)
	}
	if minutes != 90 || amount != 1500 || data != 256 {
		t.Fatalf("wrong saved offer: %d/%d/%d", minutes, amount, data)
	}
	var historical string
	if err := withTenant(db, biz, func(tx *sql.Tx) error {
		v, err := repo.CreateTXN(tx, &model.GuestPurchaseRequest{BusinessID: biz, DeviceID: &device, ClientMAC: "02:00:00:00:01:04", PlanKey: "daily", Amount: 1500, Status: "pending"})
		if err == nil {
			historical = v.ID
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.DecidePurchaseRequest(biz, historical, true); err == nil {
		t.Fatal("historical request without purchased terms was approved")
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM guest_purchase_requests WHERE id=$1`, historical).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("historical request mutated: %s", status)
	}
}
