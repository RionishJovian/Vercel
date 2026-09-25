package database

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
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
	if err := loadSchema(db, string(sqlBytes)); err != nil {
		harnessFatal = fmt.Errorf("load schema into %s: %w", harnessName, err)
		return
	}
}

// loadSchema applies database/init.sql statement-by-statement.
//
// init.sql has an ORDERING DEFECT: its `CREATE POLICY ... TO authenticated`
// statements (≈L288-340) run before `CREATE ROLE authenticated` (L421), so a
// single-batch load aborts with 42704 (undefined role). We therefore:
//   - split into statements (dollar-quote/quote/comment aware), and
//   - skip cluster-global role DDL (CREATE/DROP/ALTER ROLE) so a test never
//     mutates the shared cluster, and
//   - tolerate "undefined role" (42704) / "duplicate role" (42710) for the
//     policy + GRANT statements that need that role.
//
// Consequence: RLS policies/grants are not applied in the throwaway DB. That is
// irrelevant to the money-path SQL, which runs as the table owner (mega_admin
// bypasses RLS). Any OTHER error is fatal.
func loadSchema(db *DB, text string) error {
	for _, stmt := range splitSQL(text) {
		s := strings.TrimSpace(stmt)
		if s == "" {
			continue
		}
		up := strings.ToUpper(s)
		if strings.HasPrefix(up, "CREATE ROLE") || strings.HasPrefix(up, "DROP ROLE") || strings.HasPrefix(up, "ALTER ROLE") {
			continue
		}
		if _, err := db.Exec(s); err != nil {
			if isUndefinedRoleErr(err) || isDuplicateRoleErr(err) {
				continue
			}
			return fmt.Errorf("statement %.60q: %w", s, err)
		}
	}
	return nil
}

func isUndefinedRoleErr(err error) bool {
	var pgErr *pq.Error
	return errors.As(err, &pgErr) && pgErr.Code == "42704"
}

func isDuplicateRoleErr(err error) bool {
	var pgErr *pq.Error
	return errors.As(err, &pgErr) && pgErr.Code == "42710"
}

// splitSQL splits SQL text on top-level semicolons, ignoring semicolons inside
// single-quoted strings, dollar-quoted bodies ($$...$$ / $tag$...$tag$), and
// comments.
func splitSQL(text string) []string {
	var out []string
	var b strings.Builder
	i, n := 0, len(text)
	for i < n {
		c := text[i]
		switch {
		case c == '-' && i+1 < n && text[i+1] == '-':
			j := strings.IndexByte(text[i:], '\n')
			if j < 0 {
				b.WriteString(text[i:])
				i = n
				continue
			}
			b.WriteString(text[i : i+j+1])
			i += j + 1
		case c == '/' && i+1 < n && text[i+1] == '*':
			j := strings.Index(text[i+2:], "*/")
			if j < 0 {
				b.WriteString(text[i:])
				i = n
				continue
			}
			b.WriteString(text[i : i+2+j+2])
			i += 2 + j + 2
		case c == '\'':
			b.WriteByte(c)
			i++
			for i < n {
				b.WriteByte(text[i])
				if text[i] == '\'' {
					if i+1 < n && text[i+1] == '\'' {
						b.WriteByte(text[i+1])
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case c == '$':
			if tag, ok := dollarTag(text[i:]); ok {
				end := strings.Index(text[i+len(tag):], tag)
				if end < 0 {
					b.WriteString(text[i:])
					i = n
					continue
				}
				b.WriteString(text[i : i+len(tag)+end+len(tag)])
				i += len(tag) + end + len(tag)
				continue
			}
			b.WriteByte(c)
			i++
		case c == ';':
			out = append(out, b.String())
			b.Reset()
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	if strings.TrimSpace(b.String()) != "" {
		out = append(out, b.String())
	}
	return out
}

func dollarTag(s string) (string, bool) {
	if len(s) >= 2 && s[1] == '$' {
		return "$$", true
	}
	j := 1
	for j < len(s) && (s[j] == '_' || (s[j] >= 'a' && s[j] <= 'z') || (s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= '0' && s[j] <= '9')) {
		j++
	}
	if j > 1 && j < len(s) && s[j] == '$' {
		return s[:j+1], true
	}
	return "", false
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
