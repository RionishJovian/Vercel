package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/lib/pq"

	"mega-magic/internal/config"
	"mega-magic/internal/model"
	"mega-magic/internal/voucher"
)

// DB integration harness for the voucher money-path SQL.
//
// It creates a THROWAWAY database per run (mega_magic_test_<hex>), loads
// database/init.sql into it, exercises the real repo TXN paths, then drops it.
// The dev database is never written to.
//
// Gating: activated by MM_TEST_DB_URL (a postgres:// URL). If it is unset or the
// server cannot be reached, every harness test SKIPS — so `go test ./...` stays
// green anywhere. A failure AFTER a successful connect (e.g. init.sql will not
// load) is a real failure, not a skip.

var (
	harnessDB    *DB                   // throwaway DB under test
	harnessAdmin *DB                   // admin connection (creates/drops the throwaway)
	harnessName  string                // throwaway database name
	harnessCfg   config.DatabaseConfig // throwaway DB config (restricted pools)
	harnessSkip  error                 // connect-level problem => SKIP
	harnessFatal error                 // setup problem after connect => FAIL
	harnessOnce  sync.Once
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

// requireHarness returns the throwaway DB, skipping (never failing) when the
// harness is not activated or the DB is unreachable.
func requireHarness(t *testing.T) *DB {
	t.Helper()
	if os.Getenv("MM_TEST_DB_URL") == "" {
		t.Skip("DB integration harness skipped: set MM_TEST_DB_URL=postgres://user:pass@host:port/db?sslmode=disable to run against a throwaway DB")
	}
	if harnessSkip != nil {
		t.Skipf("DB integration harness skipped: DB not reachable/usable: %v", harnessSkip)
	}
	if harnessFatal != nil {
		t.Fatalf("DB integration harness setup failed after connect: %v", harnessFatal)
	}
	return harnessDB
}

func setupHarness(base string) {
	cfg, err := cfgFromURL(base)
	if err != nil {
		harnessSkip = fmt.Errorf("bad MM_TEST_DB_URL: %w", err)
		return
	}

	admin, err := New(*cfg)
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
	db, err := New(tcfg)
	if err != nil {
		harnessSkip = fmt.Errorf("connect throwaway db: %w", err)
		return
	}
	harnessDB = db
	harnessCfg = tcfg

	sqlBytes, err := os.ReadFile("../../database/init.sql")
	if err != nil {
		harnessFatal = fmt.Errorf("read schema: %w", err)
		return
	}
	// Strict load: the schema must apply cleanly on a fresh DB (the RLS ordering
	// fix guarantees the policies no longer need the old role-DDL workaround).
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		harnessFatal = fmt.Errorf("load schema into %s: %w", harnessName, err)
		return
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

func trimSlash(p string) string {
	for len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	return p
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// --- fixtures ---------------------------------------------------------------

func insertBusiness(t *testing.T, db *DB) string {
	t.Helper()
	var id string
	err := db.QueryRow(
		`INSERT INTO businesses (company_name, owner_email, access_tier) VALUES ($1, $2, 'SMALL_WISP') RETURNING business_id`,
		"Harness Co", "harness-"+randHex(4)+"@example.test",
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert business: %v", err)
	}
	return id
}

func insertQuota(t *testing.T, db *DB, businessID string, expiry *time.Time) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO business_metered_quotas (business_id, webfig_pass_expires_at) VALUES ($1, $2)`,
		businessID, expiry,
	); err != nil {
		t.Fatalf("insert quota: %v", err)
	}
}

func newActiveVoucher(t *testing.T, repo *VoucherRepository, businessID string, minutes int, expires *time.Time) *model.Voucher {
	t.Helper()
	code, err := voucher.GenerateCode(rand.Reader, voucher.DefaultCodeLength)
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	v := &model.Voucher{
		BusinessID:      businessID,
		PlanKey:         "sovereign",
		PlanLabel:       "Sovereign",
		Code:            code,
		PriceMMK:        5000,
		DurationMinutes: &minutes,
		Status:          voucher.StatusActive,
		ExpiresAt:       expires,
	}
	created, err := repo.CreateVoucher(v)
	if err != nil {
		t.Fatalf("create voucher: %v", err)
	}
	return created
}

// --- money-path cases -------------------------------------------------------

// TestDB_ClaimVoucher_claimsExactlyOne: the real atomic claim flips one row to
// 'used' and a second claim against the same code is a no-op.
func TestDB_ClaimVoucher_claimsExactlyOne(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	biz := insertBusiness(t, db)
	v := newActiveVoucher(t, repo, biz, 60, nil)
	now := time.Now().UTC()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	claimed, err := repo.ClaimVoucherTXN(tx, biz, v.Code, now)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if claimed == nil {
		t.Fatal("active voucher must be claimable")
	}
	if claimed.Status != voucher.StatusUsed {
		t.Fatalf("status = %q, want used", claimed.Status)
	}
	if claimed.FirstSeenAt == nil {
		t.Fatal("first_seen_at must be set on claim")
	}

	tx2, _ := db.Begin()
	second, err := repo.ClaimVoucherTXN(tx2, biz, v.Code, now)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	_ = tx2.Rollback()
	if second != nil {
		t.Fatal("second claim must return no row (idempotent no-op)")
	}
}

// TestDB_ClaimVoucher_expiredNotClaimed: the WHERE expiry guard rejects a lapsed
// voucher.
func TestDB_ClaimVoucher_expiredNotClaimed(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	biz := insertBusiness(t, db)
	past := time.Now().UTC().Add(-time.Hour)
	v := newActiveVoucher(t, repo, biz, 60, &past)

	tx, _ := db.Begin()
	claimed, err := repo.ClaimVoucherTXN(tx, biz, v.Code, time.Now().UTC())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	_ = tx.Rollback()
	if claimed != nil {
		t.Fatal("expired voucher must not be claimable")
	}
}

// TestDB_ClaimVoucher_raceExactlyOneWinner: two concurrent transactions race for
// the same code; the WHERE status='active' guard must yield exactly one winner.
func TestDB_ClaimVoucher_raceExactlyOneWinner(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	biz := insertBusiness(t, db)
	v := newActiveVoucher(t, repo, biz, 60, nil)
	now := time.Now().UTC()

	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := 0
	errs := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := db.Begin()
			if err != nil {
				errs <- err
				return
			}
			claimed, err := repo.ClaimVoucherTXN(tx, biz, v.Code, now)
			if err != nil {
				_ = tx.Rollback()
				errs <- err
				return
			}
			if err := tx.Commit(); err != nil {
				errs <- err
				return
			}
			if claimed != nil {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("race test timed out (possible deadlock)")
	}
	close(errs)
	for err := range errs {
		t.Fatalf("race claim error: %v", err)
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1", winners)
	}
}

// TestDB_RollbackOnTierFail_leavesVoucherActive: claim + a FAILING entitlement
// write in one tx, rolled back => the voucher is still 'active' and the quota is
// unchanged. This is the crash-window guarantee proven on a real DB.
func TestDB_RollbackOnTierFail_leavesVoucherActive(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	qrepo := NewBusinessMeteredQuotaRepository(db)
	biz := insertBusiness(t, db)
	original := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	insertQuota(t, db, biz, &original)
	v := newActiveVoucher(t, repo, biz, 60, nil)
	now := time.Now().UTC()

	tx, _ := db.Begin()
	claimed, err := repo.ClaimVoucherTXN(tx, biz, v.Code, now)
	if err != nil || claimed == nil {
		t.Fatalf("claim: %v / %v", claimed, err)
	}
	// Force the entitlement write to fail: an invalid uuid violates the column
	// type inside the transaction.
	exp := now.Add(time.Hour)
	badErr := qrepo.UpsertTXN(tx, &model.BusinessMeteredQuota{BusinessID: "not-a-uuid", WebfigPassExpiresAt: &exp})
	if badErr == nil {
		t.Fatal("expected the forced entitlement write to fail")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	// Voucher must be untouched.
	got := mustGetVoucher(t, db, biz, v.Code)
	if got == nil {
		t.Fatal("voucher vanished")
	}
	if got.Status != voucher.StatusActive {
		t.Fatalf("voucher status = %q, want still active after rollback", got.Status)
	}
	if got.FirstSeenAt != nil {
		t.Fatal("first_seen_at must NOT be stamped when the tx rolled back")
	}
	// Quota must be unchanged.
	q, err := qrepo.GetByBusiness(biz)
	if err != nil {
		t.Fatalf("get quota: %v", err)
	}
	if q == nil || q.WebfigPassExpiresAt == nil || !q.WebfigPassExpiresAt.Equal(original) {
		t.Fatalf("quota expiry changed on rollback: %+v, want %v", q, original)
	}
}

// TestDB_CommitPath_sovereignGrantsPass: the happy path commits the claim and
// extends the Sovereign webfig pass by the voucher duration.
func TestDB_CommitPath_sovereignGrantsPass(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	qrepo := NewBusinessMeteredQuotaRepository(db)
	now := time.Now().UTC()

	t.Run("no existing quota row -> upsert", func(t *testing.T) {
		biz := insertBusiness(t, db)
		v := newActiveVoucher(t, repo, biz, 90, nil)

		tx, _ := db.Begin()
		claimed, err := repo.ClaimVoucherTXN(tx, biz, v.Code, now)
		if err != nil || claimed == nil {
			t.Fatalf("claim: %v / %v", claimed, err)
		}
		newExpiry := voucher.ExtendExpiry(nil, now, 90)
		if err := qrepo.UpsertTXN(tx, &model.BusinessMeteredQuota{BusinessID: biz, WebfigPassExpiresAt: &newExpiry}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}

		got := mustGetVoucher(t, db, biz, v.Code)
		if got.Status != voucher.StatusUsed || got.FirstSeenAt == nil {
			t.Fatalf("voucher after commit = %+v, want used+first_seen", got)
		}
		q, _ := qrepo.GetByBusiness(biz)
		if q == nil || q.WebfigPassExpiresAt == nil {
			t.Fatal("quota row must exist with a pass expiry")
		}
		if diff := q.WebfigPassExpiresAt.Sub(newExpiry); diff > 2*time.Second || diff < -2*time.Second {
			t.Fatalf("pass expiry = %v, want ~%v", q.WebfigPassExpiresAt, newExpiry)
		}
	})

	t.Run("existing future pass -> stacked", func(t *testing.T) {
		biz := insertBusiness(t, db)
		base := now.Add(24 * time.Hour).Truncate(time.Second)
		insertQuota(t, db, biz, &base)
		v := newActiveVoucher(t, repo, biz, 30, nil)

		tx, _ := db.Begin()
		if _, err := repo.ClaimVoucherTXN(tx, biz, v.Code, now); err != nil {
			t.Fatalf("claim: %v", err)
		}
		want := voucher.ExtendExpiry(&base, now, 30)
		if err := qrepo.ExtendWebfigPassTXN(tx, biz, want); err != nil {
			t.Fatalf("extend: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}

		q, _ := qrepo.GetByBusiness(biz)
		if q == nil || q.WebfigPassExpiresAt == nil || !q.WebfigPassExpiresAt.Equal(want) {
			t.Fatalf("pass expiry = %+v, want %v", q, want)
		}
	})
}

// TestDB_CodeUniquenessConstraint: UNIQUE (business_id, code) is enforced by the
// DB (the handler's pre-check is not the guarantee).
func TestDB_CodeUniquenessConstraint(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	mins := 60

	v := newActiveVoucher(t, repo, bizA, mins, nil)

	dup := &model.Voucher{BusinessID: bizA, PlanKey: "sovereign", PlanLabel: "Sovereign",
		Code: v.Code, DurationMinutes: &mins, Status: voucher.StatusActive}
	if _, err := repo.CreateVoucher(dup); err == nil {
		t.Fatal("duplicate (business_id, code) must be rejected")
	} else if !isUniqueViolationErr(err) {
		t.Fatalf("expected unique violation, got: %v", err)
	}

	// Same code for a different business is allowed (business-scoped uniqueness).
	other := &model.Voucher{BusinessID: bizB, PlanKey: "sovereign", PlanLabel: "Sovereign",
		Code: v.Code, DurationMinutes: &mins, Status: voucher.StatusActive}
	if _, err := repo.CreateVoucher(other); err != nil {
		t.Fatalf("same code in another business should be allowed: %v", err)
	}
}

// TestDB_ClaimVoucher_businessScoped: a claim with the wrong business id claims
// nothing.
func TestDB_ClaimVoucher_businessScoped(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	v := newActiveVoucher(t, repo, bizA, 60, nil)

	tx, _ := db.Begin()
	claimed, err := repo.ClaimVoucherTXN(tx, bizB, v.Code, time.Now().UTC())
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	_ = tx.Rollback()
	if claimed != nil {
		t.Fatal("cross-business claim must return no row")
	}
}

func mustGetVoucher(t *testing.T, db *DB, businessID, code string) *model.Voucher {
	t.Helper()
	v, err := NewVoucherRepository(db).GetVoucherByCode(businessID, code)
	if err != nil {
		t.Fatalf("get voucher: %v", err)
	}
	return v
}

// isUniqueViolationErr reports whether err is a postgres unique-constraint error.
func isUniqueViolationErr(err error) bool {
	var pgErr *pq.Error
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// --- RLS isolation proof ----------------------------------------------------

// tenantTx opens a transaction acting as the NON-owner `authenticated` role with
// the tenant set, so RLS policies actually apply.
func tenantTx(t *testing.T, db *DB, businessID string) *sql.Tx {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(`SET LOCAL ROLE authenticated`); err != nil {
		t.Fatalf("set role authenticated: %v", err)
	}
	if _, err := tx.Exec(`SELECT set_config('app.current_business_id', $1, true)`, businessID); err != nil {
		t.Fatalf("set tenant context: %v", err)
	}
	return tx
}

// TestDB_RLS_schemaApplied proves the fixed init.sql actually applies RLS: the
// role exists, RLS is enabled, and all 10 policies are present.
func TestDB_RLS_schemaApplied(t *testing.T) {
	db := requireHarness(t)

	var policies int
	if err := db.QueryRow(`SELECT count(*) FROM pg_policies WHERE schemaname = 'public'`).Scan(&policies); err != nil {
		t.Fatalf("count policies: %v", err)
	}
	if policies < 10 {
		t.Fatalf("pg_policies = %d, want >= 10 (RLS block did not apply)", policies)
	}

	var roleExists bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated')`).Scan(&roleExists); err != nil {
		t.Fatalf("check role: %v", err)
	}
	if !roleExists {
		t.Fatal("role 'authenticated' must exist after init.sql")
	}

	var rlsOn bool
	if err := db.QueryRow(`SELECT relrowsecurity FROM pg_class WHERE relname = 'vouchers'`).Scan(&rlsOn); err != nil {
		t.Fatalf("check rowsecurity: %v", err)
	}
	if !rlsOn {
		t.Fatal("RLS must be enabled on vouchers")
	}
}

// TestDB_RLS_tenantIsolation is the payoff: on a REAL DB with the fixed schema,
// a non-owner tenant session sees ONLY its own business's rows; a cross-business
// row is invisible.
func TestDB_RLS_tenantIsolation(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	vA := newActiveVoucher(t, repo, bizA, 60, nil)
	vB := newActiveVoucher(t, repo, bizB, 60, nil)

	// The owner connection bypasses RLS (current app posture) — both rows visible.
	var ownerSeen int
	if err := db.QueryRow(`SELECT count(*) FROM vouchers WHERE business_id IN ($1, $2)`, bizA, bizB).Scan(&ownerSeen); err != nil {
		t.Fatalf("owner count: %v", err)
	}
	if ownerSeen != 2 {
		t.Fatalf("owner should see both tenants' rows (RLS bypass), got %d", ownerSeen)
	}

	// Tenant A: filtered query returns only A's row; B's row is invisible.
	tx := tenantTx(t, db, bizA)
	var aSeen int
	if err := tx.QueryRow(`SELECT count(*) FROM vouchers WHERE business_id IN ($1, $2)`, bizA, bizB).Scan(&aSeen); err != nil {
		t.Fatalf("tenant A count: %v", err)
	}
	if aSeen != 1 {
		t.Fatalf("tenant A should see exactly its own row, got %d", aSeen)
	}
	var aCross int
	if err := tx.QueryRow(`SELECT count(*) FROM vouchers WHERE code = $1`, vB.Code).Scan(&aCross); err != nil {
		t.Fatalf("tenant A cross count: %v", err)
	}
	if aCross != 0 {
		t.Fatal("tenant A can see tenant B's voucher — isolation broken")
	}
	var aOwn int
	if err := tx.QueryRow(`SELECT count(*) FROM vouchers WHERE code = $1`, vA.Code).Scan(&aOwn); err != nil {
		t.Fatalf("tenant A own count: %v", err)
	}
	if aOwn != 1 {
		t.Fatal("tenant A cannot see its own voucher")
	}
	_ = tx.Rollback()

	// Tenant B: symmetric.
	tx2 := tenantTx(t, db, bizB)
	var bSeen int
	if err := tx2.QueryRow(`SELECT count(*) FROM vouchers WHERE business_id IN ($1, $2)`, bizA, bizB).Scan(&bSeen); err != nil {
		t.Fatalf("tenant B count: %v", err)
	}
	if bSeen != 1 {
		t.Fatalf("tenant B should see exactly its own row, got %d", bSeen)
	}
	var bCross int
	if err := tx2.QueryRow(`SELECT count(*) FROM vouchers WHERE code = $1`, vA.Code).Scan(&bCross); err != nil {
		t.Fatalf("tenant B cross count: %v", err)
	}
	if bCross != 0 {
		t.Fatal("tenant B can see tenant A's voucher — isolation broken")
	}
	_ = tx2.Rollback()
}

// --- WithTenant mechanism proof ---------------------------------------------

// TestDB_WithTenant_isolatesThroughHelper proves the reusable helper establishes
// a reliable tenant scope on the transaction's pinned connection: queries inside
// fn see ONLY the current business's rows.
func TestDB_WithTenant_isolatesThroughHelper(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	vA := newActiveVoucher(t, repo, bizA, 60, nil)
	vB := newActiveVoucher(t, repo, bizB, 60, nil)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()

	ran := false
	err = WithTenant(tx, bizA, func() error {
		ran = true
		var seen int
		if err := tx.QueryRow(`SELECT count(*) FROM vouchers WHERE business_id IN ($1, $2)`, bizA, bizB).Scan(&seen); err != nil {
			return err
		}
		if seen != 1 {
			t.Fatalf("tenant A saw %d rows through WithTenant, want 1", seen)
		}
		var cross int
		if err := tx.QueryRow(`SELECT count(*) FROM vouchers WHERE code = $1`, vB.Code).Scan(&cross); err != nil {
			return err
		}
		if cross != 0 {
			t.Fatal("cross-business row visible through WithTenant — isolation broken")
		}
		var own int
		if err := tx.QueryRow(`SELECT count(*) FROM vouchers WHERE code = $1`, vA.Code).Scan(&own); err != nil {
			return err
		}
		if own != 1 {
			t.Fatal("own row not visible through WithTenant")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTenant: %v", err)
	}
	if !ran {
		t.Fatal("fn must run when the scope is established")
	}
}

// TestDB_WithTenant_failClosed proves the guard: a missing/malformed business id
// or a nil tx fails and fn is NOT run (state guard, fail-closed).
func TestDB_WithTenant_failClosed(t *testing.T) {
	db := requireHarness(t)
	cases := []struct {
		name  string
		biz   string
		nilTx bool
	}{
		{"empty business id", "", false},
		{"malformed business id", "not-a-uuid", false},
		{"nil tx", "00000000-0000-0000-0000-0000000000aa", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ran := false
			fn := func() error { ran = true; return nil }

			var err error
			if tc.nilTx {
				err = WithTenant(nil, tc.biz, fn)
			} else {
				tx, berr := db.Begin()
				if berr != nil {
					t.Fatalf("begin: %v", berr)
				}
				defer tx.Rollback()
				err = WithTenant(tx, tc.biz, fn)
			}
			if err == nil {
				t.Fatal("must fail closed")
			}
			if ran {
				t.Fatal("fn must NOT run when the tenant scope cannot be established")
			}
		})
	}
}

// TestDB_WithTenant_unscopedIsOff_boundary names the danger step 3 must close:
// a query NOT wrapped in WithTenant (a bare tx, or the bare pool helpers used by
// the repos) has NO guaranteed tenant scoping and sees across businesses.
func TestDB_WithTenant_unscopedIsOff_boundary(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	_ = newActiveVoucher(t, repo, bizA, 60, nil)
	_ = newActiveVoucher(t, repo, bizB, 60, nil)

	// Bare transaction, no WithTenant / no role set: no isolation applied.
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	var txSeen int
	if err := tx.QueryRow(`SELECT count(*) FROM vouchers WHERE business_id IN ($1, $2)`, bizA, bizB).Scan(&txSeen); err != nil {
		t.Fatalf("unscoped tx count: %v", err)
	}
	if txSeen != 2 {
		t.Fatalf("unscoped tx saw %d rows, want 2 (no tenant scoping) — boundary", txSeen)
	}

	// Bare POOL query (the shape the 67 repo call sites use): also unscoped.
	var poolSeen int
	if err := db.QueryRow(`SELECT count(*) FROM vouchers WHERE business_id IN ($1, $2)`, bizA, bizB).Scan(&poolSeen); err != nil {
		t.Fatalf("unscoped pool count: %v", err)
	}
	if poolSeen != 2 {
		t.Fatalf("unscoped pool query saw %d rows, want 2 (no tenant scoping) — boundary", poolSeen)
	}
}

// inTenant runs fn inside a committed tenant-scoped transaction (WithTenant).
func inTenant(t *testing.T, db *DB, businessID string, fn func(tx *sql.Tx) error) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()
	if err := WithTenant(tx, businessID, func() error { return fn(tx) }); err != nil {
		t.Fatalf("tenant tx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// TestDB_TXN_moneyPathIsolation proves the CONVERTED money-path repo calls (the
// TXN variants the handler now uses under WithTenant) isolate: own-business
// succeeds, cross-business is blocked, and the write path fails closed.
func TestDB_TXN_moneyPathIsolation(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	now := time.Now().UTC()
	mins := 60

	newV := func(biz string) *model.Voucher {
		code, _ := voucher.GenerateCode(rand.Reader, voucher.DefaultCodeLength)
		return &model.Voucher{BusinessID: biz, PlanKey: "sovereign", PlanLabel: "Sovereign",
			Code: code, DurationMinutes: &mins, Status: voucher.StatusActive}
	}

	// create-own (through TXN + WithTenant).
	var vA, vB *model.Voucher
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		v, err := repo.CreateVoucherTXN(tx, newV(bizA))
		vA = v
		return err
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		v, err := repo.CreateVoucherTXN(tx, newV(bizB))
		vB = v
		return err
	})
	if vA == nil || vB == nil {
		t.Fatal("create via TXN failed")
	}

	// list-own: each tenant sees ONLY its own voucher.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		vs, err := repo.ListVouchersByBusinessTXN(tx, bizA)
		if err != nil {
			return err
		}
		if len(vs) != 1 || vs[0].Code != vA.Code {
			t.Fatalf("tenant A list = %+v, want only its own voucher", vs)
		}
		return nil
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		vs, err := repo.ListVouchersByBusinessTXN(tx, bizB)
		if err != nil {
			return err
		}
		if len(vs) != 1 || vs[0].Code != vB.Code {
			t.Fatalf("tenant B list = %+v, want only its own voucher", vs)
		}
		return nil
	})

	// cross-business read blocked: B cannot fetch A's code.
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		got, err := repo.GetVoucherByCodeTXN(tx, bizB, vA.Code)
		if err != nil {
			return err
		}
		if got != nil {
			t.Fatal("cross-business voucher visible to tenant B")
		}
		return nil
	})

	// Cross-business mutations are blocked even for an allowed status change.
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		upd, err := repo.UpdateVoucherStatusByBusinessTXN(tx, bizB, vA.ID, "revoked", nil)
		if err != nil {
			return err
		}
		if upd != nil {
			t.Fatal("cross-business update mutated tenant A's row")
		}
		return nil
	})
	// Administrative status edits cannot claim guest redemption authority.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		upd, err := repo.UpdateVoucherStatusByBusinessTXN(tx, bizA, vA.ID, voucher.StatusUsed, nil)
		if err != nil {
			return err
		}
		if upd != nil {
			t.Fatal("administrative status edit marked voucher used")
		}
		got, err := repo.GetVoucherByCodeTXN(tx, bizA, vA.Code)
		if err != nil {
			return err
		}
		if got == nil || got.Status != voucher.StatusActive || got.FirstSeenAt != nil {
			t.Fatal("denied administrative redemption changed voucher")
		}
		return nil
	})
	// Own-business revocation remains an allowed administrative operation.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		upd, err := repo.UpdateVoucherStatusByBusinessTXN(tx, bizA, vA.ID, "revoked", nil)
		if err != nil {
			return err
		}
		if upd == nil || upd.Status != "revoked" {
			t.Fatalf("own update failed: %+v", upd)
		}
		return nil
	})

	// delete only own-business.
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		ok, err := repo.DeleteVoucherIfUnusedTXN(tx, bizB, vA.ID)
		if err != nil {
			return err
		}
		if ok {
			t.Fatal("cross-business delete succeeded")
		}
		return nil
	})
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		ok, err := repo.DeleteVoucherIfUnusedTXN(tx, bizA, vB.ID)
		if err != nil {
			return err
		}
		if ok {
			t.Fatal("tenant A deleted tenant B's voucher")
		}
		return nil
	})

	// fail-closed write: an invalid tenant id is refused before fn runs.
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	ran := false
	werr := WithTenant(tx, "not-a-uuid", func() error { ran = true; return nil })
	_ = tx.Rollback()
	if werr == nil || ran {
		t.Fatal("invalid tenant must fail closed without running fn")
	}

	// quota TXN under tenant context: own-business pass write succeeds.
	qrepo := NewBusinessMeteredQuotaRepository(db)
	exp := now.Add(time.Hour)
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		return qrepo.UpsertTXN(tx, &model.BusinessMeteredQuota{BusinessID: bizA, WebfigPassExpiresAt: &exp})
	})
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		q, err := qrepo.GetByBusinessTXN(tx, bizA)
		if err != nil {
			return err
		}
		if q == nil || q.WebfigPassExpiresAt == nil {
			t.Fatal("quota pass not written under tenant context")
		}
		return nil
	})
}

// testUUID returns a canonical hyphenated UUID string (matching Postgres'
// canonical output, so scanned ids compare equal).
func testUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func insertUser(t *testing.T, db *DB, businessID string) string {
	t.Helper()
	var id string
	err := db.QueryRow(
		`INSERT INTO users (business_id, full_name, email, password_hash, user_role) VALUES ($1, $2, $3, $4, 'OWNER') RETURNING user_id`,
		businessID, "Harness User", "u-"+randHex(6)+"@example.test", "x",
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

// TestDB_Alerting_isolation proves the CONVERTED operational-alerting path
// (incident + alert_policy TXN variants under WithTenant) isolates: own-business
// create/list/get/ack/resolve works, cross-business is blocked.
func TestDB_Alerting_isolation(t *testing.T) {
	db := requireHarness(t)
	irepo := NewIncidentRepository(db)
	prepo := NewAlertPolicyRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	userA := insertUser(t, db, bizA)
	now := time.Now().UTC()

	newPolicy := func(biz string) *model.AlertPolicy {
		return &model.AlertPolicy{BusinessID: biz, Kind: "site_offline", Enabled: true,
			Severity: "warning", CooldownMinutes: 30, NotificationChannels: []string{}}
	}

	// alert_policies: create own-business.
	var polA, polB *model.AlertPolicy
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		p, err := prepo.CreateTXN(tx, newPolicy(bizA))
		polA = p
		return err
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		p, err := prepo.CreateTXN(tx, newPolicy(bizB))
		polB = p
		return err
	})
	if polA == nil || polB == nil {
		t.Fatal("policy create via TXN failed")
	}

	// list-own only.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		ps, err := prepo.ListByBusinessTXN(tx, bizA)
		if err != nil {
			return err
		}
		if len(ps) != 1 || ps[0].ID != polA.ID {
			t.Fatalf("tenant A policies = %+v, want only its own", ps)
		}
		return nil
	})

	// cross-business policy read blocked.
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		p, err := prepo.GetByIDTXN(tx, polA.ID)
		if err != nil {
			return err
		}
		if p != nil {
			t.Fatal("cross-business policy visible to tenant B")
		}
		return nil
	})

	// cross-business policy update/delete affect 0 rows.
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		cross := newPolicy(bizB)
		cross.ID = polA.ID // target A's row, as tenant B
		if err := prepo.UpdateTXN(tx, cross); err == nil {
			t.Fatal("cross-business policy update should affect 0 rows")
		}
		return nil
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		if err := prepo.DeleteTXN(tx, polA.ID); err == nil {
			t.Fatal("cross-business policy delete should affect 0 rows")
		}
		return nil
	})

	// incidents: create own-business.
	incA := &model.Incident{ID: testUUID(), BusinessID: bizA, Kind: "site_offline",
		Severity: "warning", SubjectID: "sA", SubjectLabel: "Site A", OpenedAt: now}
	incB := &model.Incident{ID: testUUID(), BusinessID: bizB, Kind: "site_offline",
		Severity: "warning", SubjectID: "sB", SubjectLabel: "Site B", OpenedAt: now}
	inTenant(t, db, bizA, func(tx *sql.Tx) error { _, err := irepo.CreateTXN(tx, incA); return err })
	inTenant(t, db, bizB, func(tx *sql.Tx) error { _, err := irepo.CreateTXN(tx, incB); return err })

	// list-own only.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		list, err := irepo.ListOpenByBusinessTXN(tx, bizA)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != incA.ID {
			t.Fatalf("tenant A incidents = %+v, want only its own", list)
		}
		return nil
	})

	// cross-business incident read blocked (B fetching A's incident id).
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		got, err := irepo.GetByIDTXN(tx, incA.ID)
		if err != nil {
			return err
		}
		if got != nil {
			t.Fatal("cross-business incident visible to tenant B")
		}
		return nil
	})

	// ack: own succeeds; cross affects 0 rows.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		return irepo.AcknowledgeTXN(tx, incA.ID, userA)
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		if err := irepo.AcknowledgeTXN(tx, incA.ID, userA); err == nil {
			t.Fatal("cross-business acknowledge should affect 0 rows")
		}
		return nil
	})

	// resolve: own succeeds; cross affects 0 rows.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		return irepo.ResolveTXN(tx, incA.ID, userA)
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		if err := irepo.ResolveTXN(tx, incA.ID, userA); err == nil {
			t.Fatal("cross-business resolve should affect 0 rows")
		}
		return nil
	})
}

// TestDB_Telemetry_isolation proves the CONVERTED telemetry path (TXN variants
// under WithTenant) isolates: own-business ingest + list succeed, cross-business
// telemetry is invisible.
func TestDB_Telemetry_isolation(t *testing.T) {
	db := requireHarness(t)
	trepo := NewTelemetrySampleRepository(db)
	drepo := NewDeviceRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	now := time.Now().UTC()

	// Devices are set up on the owner path (device handler conversion is a later
	// slice); telemetry_samples.device_id has an FK to devices.
	mkDevice := func(biz string) string {
		d, err := drepo.CreateDevice(&model.Device{BusinessID: biz, HardwareName: "harness",
			DeploymentLayer: "ACCESS_POINT", Vendor: "mikrotik", Category: "ap", Transport: "direct"})
		if err != nil {
			t.Fatalf("create device: %v", err)
		}
		return d.DeviceID
	}
	devA := mkDevice(bizA)
	devB := mkDevice(bizB)

	mkSample := func(biz, dev string) *model.TelemetrySample {
		return &model.TelemetrySample{BusinessID: biz, DeviceID: &dev, SubjectKind: "router",
			SubjectID: "s", ObservedAt: now}
	}

	// ingest own-business through the converted TXN path.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		_, err := trepo.InsertTXN(tx, mkSample(bizA, devA))
		return err
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		_, err := trepo.InsertTXN(tx, mkSample(bizB, devB))
		return err
	})

	// list-own only.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		xs, err := trepo.ListByBusinessTXN(tx, bizA, nil, nil)
		if err != nil {
			return err
		}
		if len(xs) != 1 || xs[0].BusinessID != bizA {
			t.Fatalf("tenant A telemetry list = %d rows, want only its own", len(xs))
		}
		return nil
	})

	// cross-business telemetry read BLOCKED (B reading A's device).
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		xs, err := trepo.ListByDeviceTXN(tx, devA, nil, nil)
		if err != nil {
			return err
		}
		if len(xs) != 0 {
			t.Fatal("cross-business telemetry visible to tenant B")
		}
		return nil
	})

	// own-device read works.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		xs, err := trepo.ListByDeviceTXN(tx, devA, nil, nil)
		if err != nil {
			return err
		}
		if len(xs) != 1 {
			t.Fatalf("own-device telemetry = %d rows, want 1", len(xs))
		}
		return nil
	})

	// latest: own works; cross-business blocked.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		s, err := trepo.GetLatestByDeviceTXN(tx, devA)
		if err != nil {
			return err
		}
		if s == nil {
			t.Fatal("own latest telemetry is nil")
		}
		return nil
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		s, err := trepo.GetLatestByDeviceTXN(tx, devA)
		if err != nil {
			return err
		}
		if s != nil {
			t.Fatal("cross-business latest telemetry visible to tenant B")
		}
		return nil
	})
}

// TestDB_RestrictedConn_isolation proves the KEYSTONE mechanism at CONNECTION
// level: a restricted pool's TenantConn pins ONE connection to the tenant, so
// every query on it (not merely a single transaction) is RLS-bound. This is what
// the tenant-pinning middleware relies on (flag ON).
func TestDB_RestrictedConn_isolation(t *testing.T) {
	requireHarness(t)
	pool, err := NewRestricted(harnessCfg)
	if err != nil {
		t.Fatalf("NewRestricted: %v", err)
	}
	defer pool.Close()

	repo := NewVoucherRepository(harnessDB)
	bizA := insertBusiness(t, harnessDB)
	bizB := insertBusiness(t, harnessDB)
	vA := newActiveVoucher(t, repo, bizA, 60, nil)
	vB := newActiveVoucher(t, repo, bizB, 60, nil)
	ctx := context.Background()

	// Tenant A pinned connection: own rows visible, cross-business invisible, and
	// it stays scoped across MULTIPLE statements (connection-level, not per-tx).
	connA, err := pool.TenantConn(ctx, bizA)
	if err != nil {
		t.Fatalf("TenantConn A: %v", err)
	}
	var aSeen int
	if err := connA.QueryRowContext(ctx, `SELECT count(*) FROM vouchers WHERE business_id IN ($1, $2)`, bizA, bizB).Scan(&aSeen); err != nil {
		t.Fatalf("query A: %v", err)
	}
	if aSeen != 1 {
		t.Fatalf("pinned A saw %d rows, want 1", aSeen)
	}
	var aCross int
	if err := connA.QueryRowContext(ctx, `SELECT count(*) FROM vouchers WHERE code = $1`, vB.Code).Scan(&aCross); err != nil {
		t.Fatalf("cross A: %v", err)
	}
	if aCross != 0 {
		t.Fatal("cross-business row visible on pinned A connection")
	}
	var aSeen2 int
	if err := connA.QueryRowContext(ctx, `SELECT count(*) FROM vouchers`).Scan(&aSeen2); err != nil {
		t.Fatalf("query A2: %v", err)
	}
	if aSeen2 != 1 {
		t.Fatalf("pinned A second statement saw %d rows, want 1 (connection-level scoping)", aSeen2)
	}
	ReleaseTenantConn(ctx, connA)

	// Tenant B pinned connection: symmetric.
	connB, err := pool.TenantConn(ctx, bizB)
	if err != nil {
		t.Fatalf("TenantConn B: %v", err)
	}
	var bCross int
	if err := connB.QueryRowContext(ctx, `SELECT count(*) FROM vouchers WHERE code = $1`, vA.Code).Scan(&bCross); err != nil {
		t.Fatalf("cross B: %v", err)
	}
	if bCross != 0 {
		t.Fatal("cross-business row visible on pinned B connection")
	}
	var bSeen int
	if err := connB.QueryRowContext(ctx, `SELECT count(*) FROM vouchers`).Scan(&bSeen); err != nil {
		t.Fatalf("query B: %v", err)
	}
	if bSeen != 1 {
		t.Fatalf("pinned B saw %d rows, want 1", bSeen)
	}
	ReleaseTenantConn(ctx, connB)

	// Fail-closed: a malformed tenant id is refused before any role change.
	if _, err := pool.TenantConn(ctx, "not-a-uuid"); err == nil {
		t.Fatal("TenantConn must refuse a malformed business id")
	}
}

// TestDB_PortalDevice_isolation proves the converted portal-config / device
// request paths isolate (cross-business blocked) while ListAllPeerKeys stays a
// system-level (owner) sweep path.
func TestDB_PortalDevice_isolation(t *testing.T) {
	db := requireHarness(t)
	prepo := NewPortalConfigRepository(db)
	srepo := NewPortalSessionRepository(db)
	drepo := NewDeviceRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	now := time.Now().UTC()

	// portal_config: upsert own-business, read own; cross-business read blocked.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		return prepo.UpsertTXN(tx, &model.PortalConfig{BusinessID: bizA, BusinessName: "A Wi-Fi", ConsentRequired: true})
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		return prepo.UpsertTXN(tx, &model.PortalConfig{BusinessID: bizB, BusinessName: "B Wi-Fi", ConsentRequired: true})
	})
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		c, err := prepo.GetByBusinessTXN(tx, bizA)
		if err != nil {
			return err
		}
		if c == nil || c.BusinessName != "A Wi-Fi" {
			t.Fatalf("own portal config = %+v", c)
		}
		return nil
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		// Tenant B asking for A's config sees nothing (RLS-bound).
		c, err := prepo.GetByBusinessTXN(tx, bizA)
		if err != nil {
			return err
		}
		if c != nil {
			t.Fatal("cross-business portal config visible to tenant B")
		}
		return nil
	})

	// portal_session: create own; list-own only.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		_, err := srepo.CreateTXN(tx, &model.PortalSession{BusinessID: bizA, ClientMAC: "aa:bb:cc:00:00:01", AuthenticatedAt: now, IsActive: true})
		return err
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		_, err := srepo.CreateTXN(tx, &model.PortalSession{BusinessID: bizB, ClientMAC: "aa:bb:cc:00:00:02", AuthenticatedAt: now, IsActive: true})
		return err
	})
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		ss, err := srepo.ListActiveByBusinessTXN(tx, bizA)
		if err != nil {
			return err
		}
		if len(ss) != 1 || ss[0].BusinessID != bizA {
			t.Fatalf("tenant A sessions = %d, want only its own", len(ss))
		}
		return nil
	})

	// device: create own; cross-business read blocked; update/delete own only.
	mk := func(biz string) string {
		var id string
		inTenant(t, db, biz, func(tx *sql.Tx) error {
			d, err := drepo.CreateDeviceTXN(tx, &model.Device{BusinessID: biz, HardwareName: "harness",
				DeploymentLayer: "ACCESS_POINT", Vendor: "mikrotik", Category: "ap", Transport: "direct"})
			if err != nil {
				return err
			}
			id = d.DeviceID
			return nil
		})
		return id
	}
	devA := mk(bizA)
	devB := mk(bizB)

	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		ds, err := drepo.ListDevicesByBusinessTXN(tx, bizA)
		if err != nil {
			return err
		}
		if len(ds) != 1 || ds[0].DeviceID != devA {
			t.Fatalf("tenant A devices = %d, want only its own", len(ds))
		}
		return nil
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		d, err := drepo.GetDeviceByIDTXN(tx, devA)
		if err != nil {
			return err
		}
		if d != nil {
			t.Fatal("cross-business device visible to tenant B")
		}
		return nil
	})
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		if err := drepo.DeleteDeviceTXN(tx, devA); err == nil {
			t.Fatal("cross-business device delete should affect 0 rows")
		}
		return nil
	})

	// ListAllPeerKeys is a SYSTEM-LEVEL sweep path (owner) — still sees every
	// tenant's devices, unchanged by enforcement.
	keys, err := drepo.ListAllPeerKeys()
	if err != nil {
		t.Fatalf("ListAllPeerKeys: %v", err)
	}
	seen := map[string]bool{}
	for _, k := range keys {
		seen[k.DeviceID] = true
	}
	if !seen[devA] || !seen[devB] {
		t.Fatal("ListAllPeerKeys must remain cross-tenant (system-level) and include all devices")
	}
}

// TestDB_PortalDeviceAnchored_isolation proves the DEVICE is the tenant anchor:
// a client attached to device A (business A) cannot redeem a voucher code that
// belongs to business B. Codes are only business-scoped-unique, so resolution
// MUST come from the device — and under A's tenant, B's code is invisible.
func TestDB_PortalDeviceAnchored_isolation(t *testing.T) {
	db := requireHarness(t)
	repo := NewVoucherRepository(db)
	prepo := NewPortalConfigRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)
	vB := newActiveVoucher(t, repo, bizB, 60, nil)
	codeB := vB.Code

	// Device A -> business A: B's code is not visible under A's tenant.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		got, err := repo.GetVoucherByCodeTXN(tx, bizA, codeB)
		if err != nil {
			return err
		}
		if got != nil {
			t.Fatal("device-anchored tenant A must NOT see business B's voucher code")
		}
		return nil
	})

	// Under its true tenant (B), the code IS visible; cross-business config read
	// is blocked.
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		got, err := repo.GetVoucherByCodeTXN(tx, bizB, codeB)
		if err != nil {
			return err
		}
		if got == nil {
			t.Fatal("business B must see its own voucher code")
		}
		c, err := prepo.GetByBusinessTXN(tx, bizA)
		if err != nil {
			return err
		}
		if c != nil {
			t.Fatal("cross-business portal config visible to tenant B")
		}
		return nil
	})
}

// TestDB_WebhookSchema_applied proves the Part-1 drift fix: a fresh init.sql load
// now declares the webhook tables + indexes, and the Part-2 RLS policies apply.
func TestDB_WebhookSchema_applied(t *testing.T) {
	db := requireHarness(t)

	for _, tbl := range []string{"webhook_endpoints", "webhook_deliveries"} {
		var exists bool
		if err := db.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`, tbl,
		).Scan(&exists); err != nil {
			t.Fatalf("table check %s: %v", tbl, err)
		}
		if !exists {
			t.Fatalf("init.sql must declare %s", tbl)
		}
	}

	var idx int
	if err := db.QueryRow(`SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND indexname LIKE 'idx_webhook_%'`).Scan(&idx); err != nil {
		t.Fatalf("index check: %v", err)
	}
	if idx < 4 {
		t.Fatalf("webhook indexes = %d, want >= 4", idx)
	}

	var pol int
	if err := db.QueryRow(`SELECT count(*) FROM pg_policies WHERE schemaname='public' AND tablename LIKE 'webhook_%'`).Scan(&pol); err != nil {
		t.Fatalf("policy check: %v", err)
	}
	if pol < 2 {
		t.Fatalf("webhook RLS policies = %d, want >= 2", pol)
	}

	for _, tbl := range []string{"webhook_endpoints", "webhook_deliveries"} {
		var rls bool
		if err := db.QueryRow(`SELECT relrowsecurity FROM pg_class WHERE relname=$1`, tbl).Scan(&rls); err != nil {
			t.Fatalf("rowsecurity check %s: %v", tbl, err)
		}
		if !rls {
			t.Fatalf("RLS must be enabled on %s", tbl)
		}
	}
}

// TestDB_Webhook_isolation proves the owner CRUD + notifier worker paths isolate:
// own-business endpoint/delivery work, cross-business is invisible, and the
// worker (WithTenant-wrapped) path still succeeds.
func TestDB_Webhook_isolation(t *testing.T) {
	db := requireHarness(t)
	repo := NewWebhookRepository(db)
	bizA := insertBusiness(t, db)
	bizB := insertBusiness(t, db)

	mkEndpoint := func(biz, url string) *model.WebhookEndpoint {
		var e *model.WebhookEndpoint
		inTenant(t, db, biz, func(tx *sql.Tx) error {
			var err error
			e, err = repo.CreateEndpointTXN(tx, &model.WebhookEndpoint{
				BusinessID: biz, URL: url, EventTypes: []string{"incident.opened"}, Enabled: true,
			})
			return err
		})
		return e
	}
	epA := mkEndpoint(bizA, "https://a.example.test/hook")
	epB := mkEndpoint(bizB, "https://b.example.test/hook")
	if epA == nil || epB == nil {
		t.Fatal("endpoint create via TXN failed")
	}

	// list-own only.
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		es, err := repo.ListEndpointsTXN(tx, bizA)
		if err != nil {
			return err
		}
		if len(es) != 1 || es[0].ID != epA.ID {
			t.Fatalf("tenant A endpoints = %+v, want only its own", es)
		}
		return nil
	})

	// cross-business endpoint read BLOCKED.
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		e, err := repo.GetEndpointByIDTXN(tx, epA.ID)
		if err != nil {
			return err
		}
		if e != nil {
			t.Fatal("cross-business webhook endpoint visible to tenant B")
		}
		return nil
	})

	// WORKER PROOF: a delivery write through WithTenant(tx, businessID) — the
	// notifier's path — succeeds and is tenant-scoped.
	var delID string
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		d, err := repo.CreateDeliveryTXN(tx, &model.WebhookDelivery{
			WebhookID: &epA.ID, BusinessID: bizA, EventType: "incident.opened",
			Payload: []byte(`{"event":"incident.opened"}`), Status: "pending",
		})
		if err != nil {
			return err
		}
		delID = d.ID
		return nil
	})
	if delID == "" {
		t.Fatal("worker-path delivery create failed under WithTenant")
	}
	msg := ""
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		return repo.MarkDeliveryResultTXN(tx, delID, "delivered", 200, &msg)
	})

	// A's delivery is invisible to B.
	inTenant(t, db, bizB, func(tx *sql.Tx) error {
		ds, err := repo.ListDeliveriesTXN(tx, bizB, 20)
		if err != nil {
			return err
		}
		if len(ds) != 0 {
			t.Fatalf("tenant B sees %d deliveries, want 0 (cross-business invisible)", len(ds))
		}
		return nil
	})
	inTenant(t, db, bizA, func(tx *sql.Tx) error {
		ds, err := repo.ListDeliveriesTXN(tx, bizA, 20)
		if err != nil {
			return err
		}
		if len(ds) != 1 || ds[0].Status != "delivered" {
			t.Fatalf("tenant A deliveries = %+v, want 1 delivered", ds)
		}
		return nil
	})
}
