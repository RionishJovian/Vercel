package auth

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/config"
	"mega-magic/internal/database"
)

// DB integration test for RequireActiveTenant — the control that makes a
// suspended tenant (businesses.is_active = false) stop working.
//
// It exists because suspension used to be write-only: the platform's suspend
// action set is_active=false and NOTHING read it, so a "suspended" tenant kept
// logging in and kept using every endpoint. That is worse than having no
// control, because it reports success while changing nothing.
//
// Throwaway database per run, dropped afterwards; the dev database is never
// touched. Gating is the repo's existing convention: activated by MM_TEST_DB_URL
// (a postgres:// URL), SKIPPED when unset so `go test ./...` stays green.
//
//	MM_TEST_DB_URL='postgres://mega_admin:pw@127.0.0.1:5433/postgres?sslmode=disable' go test ./internal/auth/
func TestRequireActiveTenant_suspendedTenantIsRefused(t *testing.T) {
	base := os.Getenv("MM_TEST_DB_URL")
	if base == "" {
		t.Skip("DB integration harness skipped: set MM_TEST_DB_URL=postgres://user:***@host:port/db?sslmode=disable to run against a throwaway DB")
	}

	admin, cfg, name, ok := newThrowaway(t, base)
	if !ok {
		return // already skipped: harness unreachable
	}
	defer func() {
		// Close the pool BEFORE dropping: DROP DATABASE fails while any connection
		// is open, and the failure is easy to swallow. Leaving this out is exactly
		// how this test leaked a database on its first run — so a failed cleanup is
		// reported as a test error rather than ignored.
		_ = cfg.Close()
		if _, err := admin.Exec(`DROP DATABASE IF EXISTS "` + name + `"`); err != nil {
			t.Errorf("could not drop throwaway database %s (it has been left behind): %v", name, err)
		}
		_ = admin.Close()
	}()

	// init.sql seeds businesses, so the test needs no fixture inserts.
	var businessID string
	if err := cfg.QueryRow(`SELECT business_id FROM businesses ORDER BY company_name LIMIT 1`).Scan(&businessID); err != nil {
		t.Fatalf("no seeded business to test with: %v", err)
	}

	// The middleware is intentionally exercised through a real gin chain, with the
	// same context keys Middleware sets, so the wiring is covered too.
	run := func(ctxBusiness string) int {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(ContextKeyBusinessID, ctxBusiness)
			c.Next()
		})
		r.Use(RequireActiveTenant(cfg))
		r.GET("/thing", func(c *gin.Context) { c.Status(http.StatusOK) })

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/thing", nil))
		return w.Code
	}

	setActive := func(v bool) {
		t.Helper()
		if _, err := cfg.Exec(`UPDATE businesses SET is_active = $1 WHERE business_id = $2`, v, businessID); err != nil {
			t.Fatalf("could not set is_active=%v: %v", v, err)
		}
	}

	// 1. An active tenant is served.
	setActive(true)
	if got := run(businessID); got != http.StatusOK {
		t.Fatalf("active tenant = %d, want 200", got)
	}

	// 2. Suspended: refused with 403 (not 401 — the token was valid; the account is
	//    switched off, and saying so avoids a user retyping a working password).
	setActive(false)
	if got := run(businessID); got != http.StatusForbidden {
		t.Fatalf("suspended tenant = %d, want 403", got)
	}

	// 3. Unsuspending restores access — the state must not be sticky.
	setActive(true)
	if got := run(businessID); got != http.StatusOK {
		t.Fatalf("re-activated tenant = %d, want 200", got)
	}

	// 4. No tenant in context (platform routes, pre-tenant bootstrap) passes through:
	//    those legitimately have no business to check.
	if got := run(""); got != http.StatusOK {
		t.Fatalf("no tenant context = %d, want 200 (must not break platform routes)", got)
	}

	// 5. An unknown/absent business row must FAIL CLOSED. Failing open would let a
	//    deleted or unreadable tenant through, which is the dangerous direction.
	if got := run("11111111-2222-3333-4444-555555555555"); got != http.StatusForbidden {
		t.Fatalf("unknown tenant = %d, want 403 (must fail closed)", got)
	}
}

// newThrowaway creates a disposable database, loads database/init.sql into it,
// and returns an admin handle plus a pool bound to the new database.
func newThrowaway(t *testing.T, base string) (*sql.DB, *database.DB, string, bool) {
	t.Helper()

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("bad MM_TEST_DB_URL: %v", err)
	}
	pw, _ := u.User.Password()
	port := 5432
	if p := u.Port(); p != "" {
		if port, err = strconv.Atoi(p); err != nil {
			t.Fatalf("bad port in MM_TEST_DB_URL: %v", err)
		}
	}

	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Skipf("DB integration harness skipped: cannot open %s: %v", base, err)
	}
	ctx, cancel := timeoutCtx()
	defer cancel()
	if err := admin.PingContext(ctx); err != nil {
		_ = admin.Close()
		t.Skipf("DB integration harness skipped: cannot reach the server: %v", err)
	}

	name := fmt.Sprintf("mega_magic_authtest_%d", time.Now().UnixNano()%100000000)
	if _, err := admin.Exec(`CREATE DATABASE "` + name + `"`); err != nil {
		_ = admin.Close()
		t.Fatalf("could not create throwaway database: %v", err)
	}

	cfg := config.DatabaseConfig{
		Host: u.Hostname(), Port: port, User: u.User.Username(), Password: pw,
		Name: name, SSLMode: sslModeOf(u), MaxOpenConns: 4, MaxIdleConns: 2,
		ConnMaxLifetime: time.Minute,
	}
	pool, err := database.New(cfg)
	if err != nil {
		_, _ = admin.Exec(`DROP DATABASE IF EXISTS "` + name + `"`)
		_ = admin.Close()
		t.Fatalf("could not connect to throwaway database: %v", err)
	}

	schema, err := os.ReadFile("../../database/init.sql")
	if err != nil {
		t.Fatalf("cannot read database/init.sql: %v", err)
	}
	if _, err := pool.Exec(string(schema)); err != nil {
		t.Fatalf("init.sql failed to load into the throwaway database: %v", err)
	}

	return admin, pool, name, true
}

func timeoutCtx() (ctx context.Context, cancel context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func sslModeOf(u *url.URL) string {
	if v := u.Query().Get("sslmode"); v != "" {
		return v
	}
	if strings.Contains(u.RawQuery, "sslmode") {
		return "disable"
	}
	return "disable"
}
