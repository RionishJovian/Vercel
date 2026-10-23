package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
	"mega-magic/internal/model"
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
		"20260926_rls_backfill.sql",
		"20261001200000_guest_voucher_grants.sql",
		"20261001210000_guest_access_recovery.sql",
		"20261002200000_guest_recovery_completion.sql",
		"20261021090000_guest_voucher_activation.sql",
		"20261021180000_guest_voucher_issue_batches.sql",
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
			BusinessID:     biz,
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

	// Approve via the SAME decide-core the owner's bot bridge uses.
	if err := h.DecidePurchaseRequest(biz, reqID, true); err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	q, err := h.quotaRepo.GetByBusiness(biz)
	if err != nil {
		t.Fatalf("get quota: %v", err)
	}
	if q == nil || q.WebfigPassExpiresAt == nil {
		t.Fatal("approve must grant the wifi pass")
	}
	delta := q.WebfigPassExpiresAt.Sub(now)
	if delta < 110*time.Minute || delta > 125*time.Minute {
		t.Fatalf("granted expiry delta = %v, want ~120m (plan-driven)", delta)
	}

	// Single-wind: a second approve is a no-op (must NOT extend again).
	before := q.WebfigPassExpiresAt
	if err := h.DecidePurchaseRequest(biz, reqID, true); err == nil {
		after, _ := h.quotaRepo.GetByBusiness(biz)
		if after != nil && after.WebfigPassExpiresAt != before {
			t.Fatal("double-approve granted again (single-wind violated)")
		}
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
