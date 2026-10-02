package bridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"mega-magic/internal/database"
	"mega-magic/internal/media/jellyfin"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Explicit, disposable LOCAL database only. Never falls back to app config or
// production secrets. The database must already have the authenticated role.
func mediaTestStore(t *testing.T) (*Postgres, *database.DB) {
	t.Helper()
	raw := os.Getenv("MM_MEDIA_TEST_DSN")
	if raw == "" {
		t.Skip("MM_MEDIA_TEST_DSN not configured; PostgreSQL integration unverified")
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "postgres" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.Path != "/mm_media_test" {
		t.Fatal("use a literal loopback URL for the disposable mm_media_test database")
	}
	secret, _ := Secret()
	schema := "mm_media_test_" + secret[:16]
	admin, e := sql.Open("postgres", raw)
	if e != nil {
		t.Fatal("test database connection failed")
	}
	defer admin.Close()
	if _, e = admin.Exec(`CREATE SCHEMA ` + schema); e != nil {
		t.Fatal("cannot create disposable test schema")
	}
	t.Cleanup(func() {
		cleanup, _ := sql.Open("postgres", raw)
		if cleanup != nil {
			defer cleanup.Close()
			if _, e := cleanup.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); e != nil {
				t.Error("could not clean disposable media schema")
			}
		}
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	pool, e := sql.Open("postgres", u.String())
	if e != nil {
		t.Fatal("test connection failed")
	}
	t.Cleanup(func() { pool.Close() })
	db := &database.DB{DB: pool}
	_, e = pool.Exec(`CREATE TABLE businesses(business_id uuid PRIMARY KEY,is_active boolean NOT NULL DEFAULT true);
 CREATE TABLE users(user_id uuid PRIMARY KEY,business_id uuid NOT NULL REFERENCES businesses(business_id),is_active boolean NOT NULL DEFAULT true,user_role text NOT NULL DEFAULT 'OWNER');
 GRANT USAGE ON SCHEMA ` + schema + ` TO authenticated; GRANT SELECT ON businesses,users TO authenticated;`)
	if e != nil {
		t.Fatal("test fixtures failed; authenticated role must exist")
	}
	migration, e := os.ReadFile("../../../migrations/20261001120000_magic_media_connector.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(string(migration)); e != nil {
		t.Fatal(e)
	}
	dashboardMigration, e := os.ReadFile("../../../migrations/20261001140000_magic_media_dashboard.sql")
	if e != nil {
		t.Fatal(e)
	}
	// Keep the production-qualified ALTER TABLE inside this test's disposable
	// schema; never create public fixtures or change another test's objects.
	if _, e = pool.Exec(strings.ReplaceAll(string(dashboardMigration), "public.magic_media_jobs", schema+".magic_media_jobs")); e != nil {
		t.Fatal(e)
	}
	return NewPostgres(db), db
}
func TestPostgresPairingQueueIsolation(t *testing.T) {
	store, db := mediaTestStore(t)
	ctx := context.Background()
	a := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	b := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	actor := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	id := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	if _, e := db.Exec(`INSERT INTO businesses(business_id)VALUES($1),($2);`, a, b); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`INSERT INTO users(user_id,business_id)VALUES($1,$2)`, actor, a); e != nil {
		t.Fatal(e)
	}
	code, _ := Secret()
	pair, _ := Hash(code)
	if e := store.Pair(ctx, a, pair); e != nil {
		t.Fatal(e)
	}
	token, _ := Secret()
	hash, _ := Hash(token)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if store.Redeem(ctx, pair, hash, "Test PC") == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("single-use pairing redeemed %d times", wins.Load())
	}
	if e := store.Enqueue(ctx, a, actor, Job{ID: id, Operation: "libraries", Payload: json.RawMessage(`{}`)}); e != nil {
		t.Fatal(e)
	}
	if _, e := store.GetJob(ctx, b, id); e == nil {
		t.Fatal("cross-tenant job visible")
	}
	wins.Store(0)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, e := store.Poll(ctx, hash)
			if e != nil {
				t.Error(e)
			}
			if j != nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("expected exactly one job claim, got %d", wins.Load())
	}
	if e := store.Complete(ctx, hash, id, Completion{}); e != nil {
		t.Fatal(e)
	}
	j, e := store.GetJob(ctx, a, id)
	if e != nil || j.State != "succeeded" {
		t.Fatalf("completion failed: %+v %v", j, e)
	}
	if e := store.Complete(ctx, hash, id, Completion{}); e == nil {
		t.Fatal("accepted replayed completion")
	}
	if e := store.Revoke(ctx, a); e != nil {
		t.Fatal(e)
	}
	if _, e := store.Poll(ctx, hash); e == nil {
		t.Fatal("revoked connector allowed")
	}
	// Check raw RLS as well as application-level WHERE clauses.
	e = store.tenant(ctx, b, func(tx *sql.Tx) error {
		var count int
		e := tx.QueryRow(`SELECT count(*) FROM magic_media_jobs`).Scan(&count)
		if e == nil && count != 0 {
			t.Fatal("raw cross-tenant RLS leak")
		}
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
}
func TestPostgresExpiredAndSuspended(t *testing.T) {
	store, db := mediaTestStore(t)
	ctx := context.Background()
	bid := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	code, _ := Secret()
	pair, _ := Hash(code)
	token, _ := Secret()
	hash, _ := Hash(token)
	if _, e := db.Exec(`INSERT INTO businesses(business_id)VALUES($1)`, bid); e != nil {
		t.Fatal(e)
	}
	if e := store.Pair(ctx, bid, pair); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`UPDATE magic_media_connections SET pairing_expires_at=now()-interval '1 second'`); e != nil {
		t.Fatal(e)
	}
	if e := store.Redeem(ctx, pair, hash, "PC"); e == nil {
		t.Fatal("expired pairing allowed")
	}
	if e := store.Pair(ctx, bid, pair); e != nil {
		t.Fatal(e)
	}
	if e := store.Redeem(ctx, pair, hash, "PC"); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`UPDATE businesses SET is_active=false`); e != nil {
		t.Fatal(e)
	}
	if _, e := store.Poll(ctx, hash); e == nil {
		t.Fatal("suspended business allowed")
	}
}

func TestPostgresActorExpiryAndReplacement(t *testing.T) {
	store, db := mediaTestStore(t)
	ctx := context.Background()
	bid := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	other := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	actor := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	id := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	if _, e := db.Exec(`INSERT INTO businesses(business_id) VALUES($1),($2)`, bid, other); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`INSERT INTO users(user_id,business_id) VALUES($1,$2)`, actor, bid); e != nil {
		t.Fatal(e)
	}
	pairCode, _ := Secret()
	pair, _ := Hash(pairCode)
	token, _ := Secret()
	hash, _ := Hash(token)
	if e := store.Pair(ctx, bid, pair); e != nil {
		t.Fatal(e)
	}
	if e := store.Redeem(ctx, pair, hash, "PC"); e != nil {
		t.Fatal(e)
	}
	if e := store.Enqueue(ctx, bid, actor, Job{ID: id, Operation: "libraries", Payload: json.RawMessage(`{}`)}); e != nil {
		t.Fatal(e)
	}
	for _, update := range []string{
		`UPDATE users SET is_active=false`,
		`UPDATE users SET is_active=true,user_role='VIEWER'`,
		`UPDATE users SET user_role='OWNER',business_id='bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'`,
	} {
		if _, e := db.Exec(update); e != nil {
			t.Fatal(e)
		}
		if j, e := store.Poll(ctx, hash); e != nil || j != nil {
			t.Fatalf("ineligible actor's job delivered: %v", e)
		}
	}
	if _, e := db.Exec(`UPDATE users SET business_id=$1`, bid); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`UPDATE magic_media_jobs SET expires_at=now()-interval '1 second'`); e != nil {
		t.Fatal(e)
	}
	if j, e := store.Poll(ctx, hash); e != nil || j != nil {
		t.Fatalf("expired job delivered: %v", e)
	}
	if j, e := store.GetJob(ctx, bid, id); e != nil || j.State != "expired" {
		t.Fatalf("expired job status incorrect: %v", e)
	}
	// Replacement invalidates the previous connector, even with a queued job.
	if _, e := db.Exec(`UPDATE magic_media_jobs SET expires_at=now()+interval '2 minutes'`); e != nil {
		t.Fatal(e)
	}
	newCode, _ := Secret()
	newPair, _ := Hash(newCode)
	newToken, _ := Secret()
	newHash, _ := Hash(newToken)
	if e := store.Pair(ctx, bid, newPair); e != nil {
		t.Fatal(e)
	}
	if e := store.Redeem(ctx, newPair, newHash, "Replacement PC"); e != nil {
		t.Fatal(e)
	}
	if _, e := store.Poll(ctx, hash); e == nil {
		t.Fatal("old connector survived replacement")
	}
	if j, e := store.GetJob(ctx, bid, id); e != nil || j.State != "cancelled" {
		t.Fatalf("replacement did not cancel pending job: %v", e)
	}
	if e := store.tenant(ctx, other, func(tx *sql.Tx) error {
		_, e := tx.Exec(`INSERT INTO magic_media_connections(business_id) VALUES($1)`, bid)
		return e
	}); e == nil {
		t.Fatal("cross-tenant connection insert allowed")
	}
	if _, e := db.Exec(`UPDATE magic_media_connections SET token_expires_at=now()-interval '1 second'`); e != nil {
		t.Fatal(e)
	}
	if _, e := store.Poll(ctx, newHash); e == nil {
		t.Fatal("expired connector token allowed")
	}
}

func TestPostgresDashboardCompletion(t *testing.T) {
	store, db := mediaTestStore(t)
	ctx := context.Background()
	bid := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	actor := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	id := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	if _, e := db.Exec(`INSERT INTO businesses(business_id) VALUES($1)`, bid); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`INSERT INTO users(user_id,business_id) VALUES($1,$2)`, actor, bid); e != nil {
		t.Fatal(e)
	}
	code, _ := Secret()
	pair, _ := Hash(code)
	token, _ := Secret()
	hash, _ := Hash(token)
	if e := store.Pair(ctx, bid, pair); e != nil {
		t.Fatal(e)
	}
	if e := store.Redeem(ctx, pair, hash, "PC"); e != nil {
		t.Fatal(e)
	}
	if e := store.Enqueue(ctx, bid, actor, Job{ID: id, Operation: "dashboard", Payload: json.RawMessage(`{}`)}); e != nil {
		t.Fatal(e)
	}
	if j, e := store.Poll(ctx, hash); e != nil || j == nil || j.Operation != "dashboard" {
		t.Fatalf("dashboard claim failed: %v", e)
	}
	if e := store.Complete(ctx, hash, id, Completion{}); e == nil {
		t.Fatal("accepted missing dashboard")
	}
	zero := int64(0)
	d := jellyfin.Dashboard{CollectedAt: time.Now(), Counts: &jellyfin.Counts{Movies: &zero}}
	if e := store.Complete(ctx, hash, id, Completion{Dashboard: &d, Libraries: []jellyfin.Library{{Name: "unrelated"}}}); e != nil {
		t.Fatal(e)
	}
	j, e := store.GetJob(ctx, bid, id)
	if e != nil {
		t.Fatal(e)
	}
	var result Completion
	if e := json.Unmarshal(j.Result, &result); e != nil {
		t.Fatal(e)
	}
	if j.State != "succeeded" || result.Dashboard == nil || result.Dashboard.Counts.Movies == nil || *result.Dashboard.Counts.Movies != 0 || result.Libraries != nil {
		t.Fatal("invalid or unrelated dashboard result persisted")
	}
}
