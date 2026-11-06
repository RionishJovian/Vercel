package discoverystore

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"mega-magic/internal/database"
)

// Integration tests need a throwaway PostgreSQL: MM_TEST_DB_URL=postgres://user@host:port/postgres?sslmode=disable
// They load db/baseline.sql and the discovery migrations into a new database, then connect as the app's restricted
// runtime role, so row security is enforced the way it is in production.
var (
	pool    *database.DB
	other   string // second business
	mine    string // first business
	setupOK bool
)

func TestMain(m *testing.M) {
	base := os.Getenv("MM_TEST_DB_URL")
	var cleanup func()
	if base != "" {
		var err error
		pool, cleanup, err = setup(base)
		if err != nil {
			fmt.Fprintln(os.Stderr, "integration setup failed:", err)
			os.Exit(1)
		}
		setupOK = true
	}
	code := m.Run()
	if cleanup != nil {
		cleanup()
	}
	os.Exit(code)
}

func setup(base string) (*database.DB, func(), error) {
	admin, err := sql.Open("postgres", base)
	if err != nil {
		return nil, nil, err
	}
	name := fmt.Sprintf("mm_discovery_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE DATABASE " + name); err != nil {
		return nil, nil, err
	}
	u, _ := url.Parse(base)
	u.Path = "/" + name
	owner, err := sql.Open("postgres", u.String())
	if err != nil {
		return nil, nil, err
	}
	owner.SetMaxOpenConns(1) // keep one session so SET search_path below applies to every file
	run := func(path string) error {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := owner.Exec(string(b)); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		return nil
	}
	if err := run("../../db/baseline.sql"); err != nil {
		return nil, nil, err
	}
	if _, err := owner.Exec(`SET search_path = public`); err != nil { // the baseline clears it; the migration runner sets it per file
		return nil, nil, err
	}
	// Only this feature's migration is applied on top of the baseline: other migration files assume tables from the
	// older chain, and this one needs just businesses and sites.
	files, _ := filepath.Glob("../../db/migrations/*_discovery_*.sql")
	sort.Strings(files)
	for _, f := range files {
		if err := run(f); err != nil {
			return nil, nil, err
		}
	}
	if err := run("../../deploy/runtime-db-role.sql"); err != nil {
		return nil, nil, err
	}
	if _, err := owner.Exec(`ALTER ROLE mega_magic_runtime LOGIN PASSWORD 'testpw'`); err != nil {
		return nil, nil, err
	}
	for i, b := range []*string{&mine, &other} {
		if err := owner.QueryRow(`INSERT INTO businesses(company_name,owner_email) VALUES($1,$2) RETURNING business_id`, fmt.Sprintf("Shop %d", i), fmt.Sprintf("o%d@example.test", i)).Scan(b); err != nil {
			return nil, nil, err
		}
	}
	u.User = url.UserPassword("mega_magic_runtime", "testpw")
	app, err := sql.Open("postgres", u.String())
	if err != nil {
		return nil, nil, err
	}
	return &database.DB{DB: app}, func() {
		app.Close()
		owner.Close()
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
		admin.Close()
	}, nil
}

func skip(t *testing.T) *Store {
	if !setupOK {
		t.Skip("set MM_TEST_DB_URL to run the discovery integration tests")
	}
	return New(pool)
}

// reset removes every session so tests do not depend on each other.
func reset(t *testing.T) {
	if _, err := pool.Exec(`DELETE FROM discovery_sessions`); err != nil {
		t.Fatal(err)
	}
}

const user = "11111111-1111-4111-8111-111111111111"

func newPaired(t *testing.T, s *Store, bid string) (Created, Paired) {
	c, err := s.Create(context.Background(), bid, user, "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Pair(context.Background(), c.PairingCode)
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	return c, p
}

func obs(key string) ObservationIn {
	return ObservationIn{DeviceKey: key, MACs: []string{"48:A9:8A:00:00:06"}, Identity: "hAP", Board: "C52iG-5HaxD2HaxD", Version: "7.25", ClaimedIP: "192.168.88.1", SourceIP: "192.168.88.1", HeardCount: 1}
}

func TestSessionLifecycleAndOneLivePerBusiness(t *testing.T) {
	s := skip(t)
	reset(t)
	ctx := context.Background()
	c, err := s.Create(ctx, mine, user, "")
	if err != nil || len(c.PairingCode) != 15 || c.PairingCode[6] != '-' {
		t.Fatalf("create: %+v %v", c, err)
	}
	if _, err := s.Create(ctx, mine, user, ""); err != ErrLiveSession {
		t.Fatalf("a second live session for the same business must be refused: %v", err)
	}
	if _, err := s.Create(ctx, other, user, ""); err != nil {
		t.Fatalf("another business is independent: %v", err)
	}
	v, err := s.Get(ctx, mine, c.SessionID)
	if err != nil || v.State != "open" || len(v.Candidates) != 0 {
		t.Fatalf("get: %+v %v", v, err)
	}
	if err := s.Cancel(ctx, mine, c.SessionID); err != nil {
		t.Fatal(err)
	}
	if v, _ = s.Get(ctx, mine, c.SessionID); v.State != "cancelled" {
		t.Fatalf("state after cancel: %s", v.State)
	}
	if _, err := s.Pair(ctx, c.PairingCode); err != ErrUnavailable {
		t.Fatalf("a cancelled session must not pair: %v", err)
	}
	if _, err := s.Create(ctx, mine, user, ""); err != nil {
		t.Fatalf("a new session after cancel: %v", err)
	}
}

func TestTenantIsolation(t *testing.T) {
	s := skip(t)
	reset(t)
	ctx := context.Background()
	c, p := newPaired(t, s, mine)
	if _, _, err := s.AddObservations(ctx, p.Token, p.SessionID, []ObservationIn{obs("sw:0123456789abcdef")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, other, c.SessionID); err != ErrUnavailable {
		t.Fatalf("another business must not read the session: %v", err)
	}
	if err := s.Cancel(ctx, other, c.SessionID); err != ErrUnavailable {
		t.Fatalf("another business must not cancel the session: %v", err)
	}
	if _, err := s.Create(ctx, other, user, "22222222-2222-4222-8222-222222222222"); err != ErrBadSite {
		t.Fatalf("a site that does not exist for this business must be refused: %v", err)
	}
	// Row security itself: as the tenant role with the other business set, the rows are invisible.
	tx, _ := pool.Begin()
	defer tx.Rollback()
	var n int
	_ = database.WithTenant(tx, other, func() error {
		return tx.QueryRow(`SELECT (SELECT count(*) FROM discovery_sessions)+(SELECT count(*) FROM discovery_observations)`).Scan(&n)
	})
	if n != 0 {
		t.Fatalf("row security leaked %d rows to another business", n)
	}
}

func TestPairingIsSingleUseAndGuessingLocks(t *testing.T) {
	s := skip(t)
	reset(t)
	ctx := context.Background()
	c, err := s.Create(ctx, mine, user, "")
	if err != nil {
		t.Fatal(err)
	}
	wrong := c.PairingCode[:7] + strings.Repeat("A", 8)
	if wrong == c.PairingCode {
		wrong = c.PairingCode[:7] + strings.Repeat("B", 8)
	}
	for i := 0; i < MaxPairAttempts; i++ {
		if _, err := s.Pair(ctx, wrong); err != ErrUnavailable {
			t.Fatalf("a wrong secret must fail: %v", err)
		}
	}
	if _, err := s.Pair(ctx, c.PairingCode); err != ErrUnavailable {
		t.Fatalf("after too many wrong guesses even the right code must fail: %v", err)
	}
	if v, _ := s.Get(ctx, mine, c.SessionID); v.State != "closed" {
		t.Fatalf("the session must close after repeated guessing: %s", v.State)
	}

	reset(t)
	c, _ = s.Create(ctx, mine, user, "")
	if _, err := s.Pair(ctx, strings.ToLower(c.PairingCode)); err != nil {
		t.Fatalf("pairing is case-insensitive: %v", err)
	}
	if _, err := s.Pair(ctx, c.PairingCode); err != ErrUnavailable {
		t.Fatalf("a used code must not work again (replay): %v", err)
	}
	for _, bad := range []string{"", "ABC", "AAAAAA-AAAAAAAA-X", "IIIIII-OOOOOOOO", "AAAAAA_BBBBBBBB"} {
		if _, err := s.Pair(ctx, bad); err != ErrUnavailable {
			t.Fatalf("malformed code %q must fail: %v", bad, err)
		}
	}
}

func TestExpiredPairingAndSessionAreRefused(t *testing.T) {
	s := skip(t)
	reset(t)
	ctx := context.Background()
	c, _ := s.Create(ctx, mine, user, "")
	if _, err := pool.Exec(`UPDATE discovery_sessions SET pair_expires_at=now()-interval '1 second' WHERE session_id=$1`, c.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pair(ctx, c.PairingCode); err != ErrUnavailable {
		t.Fatalf("an expired code must fail: %v", err)
	}
	if v, _ := s.Get(ctx, mine, c.SessionID); v.State != "open" || v.PairExpiresAt == nil || v.PairExpiresAt.After(time.Now()) {
		t.Fatalf("an open session whose code ran out stays open but shows the past expiry: %+v", v)
	}
	if _, err := pool.Exec(`UPDATE discovery_sessions SET created_at=now()-interval '10 minutes', expires_at=now()-interval '1 second' WHERE session_id=$1`, c.SessionID); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Get(ctx, mine, c.SessionID); v.State != "expired" {
		t.Fatalf("an open session past its end must read as expired: %s", v.State)
	}

	reset(t)
	_, p := newPaired(t, s, mine)
	if _, err := pool.Exec(`UPDATE discovery_sessions SET created_at=now()-interval '10 minutes', expires_at=now()-interval '1 second' WHERE session_id=$1`, p.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddObservations(context.Background(), p.Token, p.SessionID, []ObservationIn{obs("sw:0123456789abcdef")}); err != ErrUnavailable {
		t.Fatalf("an expired session must not accept uploads: %v", err)
	}
	if _, err := s.Create(ctx, mine, user, ""); err != nil {
		t.Fatalf("an expired session must not block a new one: %v", err)
	}
}

func TestUploadsAreScopedBoundedAndMerged(t *testing.T) {
	s := skip(t)
	reset(t)
	ctx := context.Background()
	_, p := newPaired(t, s, mine)
	if _, _, err := s.AddObservations(ctx, strings.Repeat("0", 64), p.SessionID, []ObservationIn{obs("sw:0123456789abcdef")}); err != ErrUnavailable {
		t.Fatalf("an unknown token must fail: %v", err)
	}
	if _, _, err := s.AddObservations(ctx, p.Token, "33333333-3333-4333-8333-333333333333", []ObservationIn{obs("sw:0123456789abcdef")}); err != ErrUnavailable {
		t.Fatalf("a token for another session id must fail: %v", err)
	}
	a, d, err := s.AddObservations(ctx, p.Token, p.SessionID, []ObservationIn{obs("sw:0123456789abcdef")})
	if err != nil || a != 1 || d != 0 {
		t.Fatalf("first upload: %d %d %v", a, d, err)
	}
	if _, _, err := s.AddObservations(ctx, p.Token, p.SessionID, []ObservationIn{obs("sw:0123456789abcdef")}); err != ErrTooFast {
		t.Fatalf("uploads faster than the minimum gap must be refused: %v", err)
	}
	time.Sleep(MinUploadGap + 200*time.Millisecond)
	second := obs("sw:0123456789abcdef")
	second.MACs, second.HeardCount, second.IPChanged = []string{"48:A9:8A:00:00:07"}, 3, true
	if _, _, err := s.AddObservations(ctx, p.Token, p.SessionID, []ObservationIn{second}); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Get(ctx, mine, p.SessionID)
	if len(v.Candidates) != 1 || len(v.Candidates[0].MACs) != 2 || v.Candidates[0].HeardCount != 3 || !v.Candidates[0].IPChanged || v.Candidates[0].Status != "nearby_unverified" {
		t.Fatalf("one device heard twice must merge into one unverified candidate with both MACs: %+v", v.Candidates)
	}

	// The candidate limit holds even if a helper floods forged devices.
	reset(t)
	_, p = newPaired(t, s, mine)
	var flood []ObservationIn
	for i := 0; i < MaxObservations+6; i++ {
		flood = append(flood, obs(fmt.Sprintf("sw:%016x", i)))
	}
	a, d, err = s.AddObservations(ctx, p.Token, p.SessionID, flood)
	if err != nil || a != MaxObservations || d != 6 {
		t.Fatalf("flood must be bounded: accepted %d dropped %d err %v", a, d, err)
	}
}

func TestCancelRevokesTheToken(t *testing.T) {
	s := skip(t)
	reset(t)
	ctx := context.Background()
	c, p := newPaired(t, s, mine)
	if err := s.Cancel(ctx, mine, c.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddObservations(ctx, p.Token, p.SessionID, []ObservationIn{obs("sw:0123456789abcdef")}); err != ErrUnavailable {
		t.Fatalf("a cancelled session's token must stop working: %v", err)
	}
}

func TestSweepDeletesOldSessionsAndTheirObservations(t *testing.T) {
	s := skip(t)
	reset(t)
	ctx := context.Background()
	c, p := newPaired(t, s, mine)
	_, _, _ = s.AddObservations(ctx, p.Token, p.SessionID, []ObservationIn{obs("sw:0123456789abcdef")})
	if _, err := pool.Exec(`UPDATE discovery_sessions SET state='closed', ended_at=now()-interval '2 hours' WHERE session_id=$1`, c.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := s.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = pool.QueryRow(`SELECT (SELECT count(*) FROM discovery_sessions)+(SELECT count(*) FROM discovery_observations)`).Scan(&n)
	if n != 0 {
		t.Fatalf("ended sessions and their observations must be deleted after the retention time, %d rows left", n)
	}
}
