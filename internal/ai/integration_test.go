package ai

import (
	"context"
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
)

// ---------------------------------------------------------------------------
// DB integration harness (package-internal to internal/ai; cannot use
// internal/database's unexported helpers). Same contract as the database
// harness: env-gated on MM_TEST_DB_URL, SKIP (never fail) when absent, and a
// fresh throwaway DB loaded from database/init.sql.
// ---------------------------------------------------------------------------

var (
	itDB    *database.DB
	itAdmin *database.DB
	itName  string
	itSkip  error
	itFatal error
	itOnce  sync.Once
)

func TestMain(m *testing.M) {
	base := os.Getenv("MM_TEST_DB_URL")
	if base != "" {
		itOnce.Do(func() { setupIT(base) })
	}
	code := m.Run()
	teardownIT()
	os.Exit(code)
}

func requireIT(t *testing.T) *database.DB {
	t.Helper()
	if os.Getenv("MM_TEST_DB_URL") == "" {
		t.Skip("AI DB integration harness skipped: set MM_TEST_DB_URL=postgres://user:pass@host:port/db?sslmode=disable to run against a throwaway DB")
	}
	if itSkip != nil {
		t.Skipf("AI DB integration harness skipped: DB not reachable/usable: %v", itSkip)
	}
	if itFatal != nil {
		t.Fatalf("AI DB integration harness setup failed after connect: %v", itFatal)
	}
	return itDB
}

func setupIT(base string) {
	cfg, err := itCfgFromURL(base)
	if err != nil {
		itSkip = fmt.Errorf("bad MM_TEST_DB_URL: %w", err)
		return
	}

	admin, err := database.New(*cfg)
	if err != nil {
		itSkip = fmt.Errorf("connect: %w", err)
		return
	}
	itAdmin = admin

	itName = "mega_magic_ai_test_" + itRandHex(8)
	if _, err := admin.Exec("CREATE DATABASE " + itName); err != nil {
		itSkip = fmt.Errorf("create throwaway db: %w", err)
		return
	}

	tcfg := *cfg
	tcfg.Name = itName
	db, err := database.New(tcfg)
	if err != nil {
		itSkip = fmt.Errorf("connect throwaway db: %w", err)
		return
	}
	itDB = db

	sqlBytes, err := os.ReadFile("../../database/init.sql")
	if err != nil {
		itFatal = fmt.Errorf("read schema: %w", err)
		return
	}
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		itFatal = fmt.Errorf("load schema into %s: %w", itName, err)
		return
	}
}

func teardownIT() {
	if itDB != nil {
		_ = itDB.Close()
	}
	if itAdmin != nil && itName != "" {
		_, _ = itAdmin.Exec("DROP DATABASE IF EXISTS " + itName)
	}
	if itAdmin != nil {
		_ = itAdmin.Close()
	}
}

func itCfgFromURL(raw string) (*config.DatabaseConfig, error) {
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
	name := u.Path
	for len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}
	return &config.DatabaseConfig{
		Host:         u.Hostname(),
		Port:         port,
		User:         u.User.Username(),
		Password:     pass,
		Name:         name,
		SSLMode:      ssl,
		MaxOpenConns: 10,
		MaxIdleConns: 5,
	}, nil
}

func itRandHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// --- fixtures ---------------------------------------------------------------

func itInsertBusiness(t *testing.T, db *database.DB) string {
	t.Helper()
	var id string
	err := db.QueryRow(
		`INSERT INTO businesses (company_name, owner_email, access_tier) VALUES ($1, $2, 'SMALL_WISP') RETURNING business_id`,
		"AI Harness Co", "ai-harness-"+itRandHex(4)+"@example.test",
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert business: %v", err)
	}
	return id
}

func itSeedBalance(t *testing.T, db *database.DB, businessID string, balance int) {
	t.Helper()
	repo := database.NewBusinessMeteredQuotaRepository(db)
	if err := repo.Upsert(&model.BusinessMeteredQuota{BusinessID: businessID, AITokenBalance: balance}); err != nil {
		t.Fatalf("seed balance: %v", err)
	}
}

func itBalance(t *testing.T, db *database.DB, businessID string) int {
	t.Helper()
	q, err := database.NewBusinessMeteredQuotaRepository(db).GetByBusiness(businessID)
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}
	if q == nil {
		return 0
	}
	return q.AITokenBalance
}

func itInTenant(t *testing.T, db *database.DB, businessID string, fn func(tx *sql.Tx) error) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	if err := database.WithTenant(tx, businessID, func() error { return fn(tx) }); err != nil {
		t.Fatalf("tenant tx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 1) AI token spend meter — tenant-scoped, never below zero.
// ---------------------------------------------------------------------------

func TestDB_AISpend_meterTenantScoped(t *testing.T) {
	db := requireIT(t)
	meter := &quotaMeter{db: db, repo: database.NewBusinessMeteredQuotaRepository(db)}

	bizA := itInsertBusiness(t, db)
	bizB := itInsertBusiness(t, db)
	itSeedBalance(t, db, bizA, 3)
	itSeedBalance(t, db, bizB, 3)

	ctx := context.Background()

	spent, err := meter.Spend(ctx, bizA, 1)
	if err != nil || !spent {
		t.Fatalf("spend A: spent=%v err=%v, want true/nil", spent, err)
	}
	if got := itBalance(t, db, bizA); got != 2 {
		t.Fatalf("balance A = %d, want 2", got)
	}
	// RLS isolation: B is untouched by A's spend.
	if got := itBalance(t, db, bizB); got != 3 {
		t.Fatalf("balance B = %d, want 3 (cross-tenant leak)", got)
	}
}

func TestDB_AISpend_neverBelowZero(t *testing.T) {
	db := requireIT(t)
	meter := &quotaMeter{db: db, repo: database.NewBusinessMeteredQuotaRepository(db)}
	biz := itInsertBusiness(t, db)
	itSeedBalance(t, db, biz, 1)
	ctx := context.Background()

	// Spend the last token.
	if spent, err := meter.Spend(ctx, biz, 1); err != nil || !spent {
		t.Fatalf("first spend: spent=%v err=%v", spent, err)
	}
	if got := itBalance(t, db, biz); got != 0 {
		t.Fatalf("balance = %d, want 0", got)
	}
	// Exhausted: no spend, no error, never negative.
	if spent, err := meter.Spend(ctx, biz, 1); err != nil || spent {
		t.Fatalf("exhausted spend: spent=%v err=%v, want false/nil", spent, err)
	}
	if got := itBalance(t, db, biz); got != 0 {
		t.Fatalf("balance = %d, want 0 (must not go negative)", got)
	}
}

// ---------------------------------------------------------------------------
// 2) Incident open — dedupe-then-create, tenant-scoped + RLS isolated.
// ---------------------------------------------------------------------------

func TestDB_AIIncidentOpen_dedupedAndIsolated(t *testing.T) {
	db := requireIT(t)
	store := &databaseIncidentStore{db: db, repo: database.NewIncidentRepository(db)}

	bizA := itInsertBusiness(t, db)
	bizB := itInsertBusiness(t, db)

	sampleA := model.TelemetrySample{BusinessID: bizA, SubjectKind: "router", SubjectID: "subject-a"}
	findA := &Finding{Rule: "latency_spike", Severity: "warn", Summary: "latency 500ms"}
	incA := findingToIncident(sampleA, findA, time.Now().UTC())

	// First open succeeds.
	created, err := store.CreateIncident(bizA, incA)
	if err != nil || !created {
		t.Fatalf("first open: created=%v err=%v, want true/nil", created, err)
	}

	// Second open for the SAME subject+kind is deduped (durable guard).
	again, err := store.CreateIncident(bizA, findingToIncident(sampleA, findA, time.Now().UTC()))
	if err != nil {
		t.Fatalf("second open err: %v", err)
	}
	if again {
		t.Fatal("same subject+kind must be deduped by the store (no stacking)")
	}

	// RLS isolation: A sees exactly its one incident; B sees none of A's.
	var aCount int
	itInTenant(t, db, bizA, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM incidents`).Scan(&aCount)
	})
	if aCount != 1 {
		t.Fatalf("tenant A incidents = %d, want 1", aCount)
	}

	var bCount int
	itInTenant(t, db, bizB, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM incidents`).Scan(&bCount)
	})
	if bCount != 0 {
		t.Fatalf("tenant B incidents = %d, want 0 (cross-tenant leak)", bCount)
	}

	// A different tenant opens its OWN incident; still no cross-visibility.
	sampleB := model.TelemetrySample{BusinessID: bizB, SubjectKind: "router", SubjectID: "subject-b"}
	created, err = store.CreateIncident(bizB, findingToIncident(sampleB, findA, time.Now().UTC()))
	if err != nil || !created {
		t.Fatalf("open for B: created=%v err=%v", created, err)
	}
	itInTenant(t, db, bizA, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM incidents`).Scan(&aCount)
	})
	if aCount != 1 {
		t.Fatalf("tenant A incidents = %d after B opened its own, want 1", aCount)
	}
}
