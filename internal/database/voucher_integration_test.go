package database

import (
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
	harnessDB    *DB    // throwaway DB under test
	harnessAdmin *DB    // admin connection (creates/drops the throwaway)
	harnessName  string // throwaway database name
	harnessSkip  error  // connect-level problem => SKIP
	harnessFatal error  // setup problem after connect => FAIL
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
