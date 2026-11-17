package database

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"mega-magic/internal/secretbox"
)

var hubEnrollmentMigrationOnce sync.Once

// hubEnrollmentDB returns the throwaway harness database with this wave's
// migration applied. It needs MM_TEST_DB_URL like the other integration tests.
func hubEnrollmentDB(t *testing.T) *DB {
	t.Helper()
	db := requireHarness(t)
	hubEnrollmentMigrationOnce.Do(func() {
		// The harness schema predates the pending-enrolment secrets table.
		var have bool
		if err := db.QueryRow(`SELECT to_regclass('public.hub_pending_enrollments') IS NOT NULL`).Scan(&have); err != nil {
			t.Fatal(err)
		}
		if !have {
			pending, err := os.ReadFile("../../migrations/20261024160000_hub_pending_enrollments.sql")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(string(pending)); err != nil {
				t.Fatalf("apply pending enrollments migration: %v", err)
			}
		}
		sqlBytes, err := os.ReadFile("../../db/migrations/20261114120000_hub_callback_enrollment.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(sqlBytes)); err != nil {
			t.Fatalf("apply hub callback enrollment migration: %v", err)
		}
	})
	return db
}

func newHubTestDevice(t *testing.T, db *DB, biz string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`INSERT INTO devices(business_id,hardware_name) VALUES($1,'Hub enrollment fixture') RETURNING device_id::text`, biz).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newUUID(t *testing.T, db *DB) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func peerKey(c byte) string {
	return strings.Repeat(string(rune('A'+c)), 43) + "="
}

func TestCallbackTokenIsSingleUseAndStoredHashed(t *testing.T) {
	db := hubEnrollmentDB(t)
	ctx := context.Background()
	biz := insertBusiness(t, db)
	device := newHubTestDevice(t, db, biz)
	actor := newUUID(t, db)
	store := HubCallbackTokenStore{DB: db}

	token, expires, err := store.Issue(ctx, biz, device, actor, 15*time.Minute)
	if err != nil || len(token) != 43 || !expires.After(time.Now()) {
		t.Fatalf("issue: token=%d err=%v", len(token), err)
	}
	var stored []byte
	if err := db.QueryRow(`SELECT token_hash FROM hub_callback_tokens WHERE business_id=$1 AND device_id=$2`, biz, device).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 32 || string(stored) == token || strings.Contains(string(stored), token) {
		t.Fatal("only a hash of the token may be stored")
	}

	gotBiz, gotDevice, err := store.Consume(ctx, token)
	if err != nil || gotBiz != biz || gotDevice != device {
		t.Fatalf("consume: %v %s %s", err, gotBiz, gotDevice)
	}
	if _, _, err := store.Consume(ctx, token); !errors.Is(err, ErrCallbackTokenInvalid) {
		t.Fatalf("a spent token must not work again: %v", err)
	}
}

func TestCallbackTokenReissueExpiryAndRaces(t *testing.T) {
	db := hubEnrollmentDB(t)
	ctx := context.Background()
	biz := insertBusiness(t, db)
	device := newHubTestDevice(t, db, biz)
	actor := newUUID(t, db)
	store := HubCallbackTokenStore{DB: db}

	first, _, _ := store.Issue(ctx, biz, device, actor, 15*time.Minute)
	second, _, err := store.Issue(ctx, biz, device, actor, 15*time.Minute)
	if err != nil || first == second {
		t.Fatalf("reissue: %v", err)
	}
	if _, _, err := store.Consume(ctx, first); !errors.Is(err, ErrCallbackTokenInvalid) {
		t.Fatal("a newer token must replace the older one")
	}

	// Twenty callers racing with the same live token: exactly one wins.
	var wins, losses int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := store.Consume(ctx, second)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				wins++
			} else if errors.Is(err, ErrCallbackTokenInvalid) {
				losses++
			}
		}()
	}
	wg.Wait()
	if wins != 1 || losses != 19 {
		t.Fatalf("single use violated: wins=%d losses=%d", wins, losses)
	}

	expiring, _, _ := store.Issue(ctx, biz, device, actor, 15*time.Minute)
	if _, err := db.Exec(`UPDATE hub_callback_tokens SET expires_at=clock_timestamp()-interval '1 second' WHERE business_id=$1 AND device_id=$2`, biz, device); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Consume(ctx, expiring); !errors.Is(err, ErrCallbackTokenInvalid) {
		t.Fatal("an expired token must not work")
	}
	for _, bad := range []string{"", "short", strings.Repeat("A", 43), strings.Repeat("A", 44)} {
		if _, _, err := store.Consume(ctx, bad); !errors.Is(err, ErrCallbackTokenInvalid) {
			t.Fatalf("token %q must be refused", bad)
		}
	}
	if _, _, err := store.Issue(ctx, biz, device, actor, 2*time.Hour); err == nil {
		t.Fatal("a token lifetime above an hour must be refused")
	}
	if _, _, err := store.Issue(ctx, "not-a-uuid", device, actor, time.Minute); err == nil {
		t.Fatal("invalid identity must be refused")
	}
}

func TestPeerStateRefusesDuplicateTunnelAddresses(t *testing.T) {
	db := hubEnrollmentDB(t)
	ctx := context.Background()
	biz := insertBusiness(t, db)
	a, b := newHubTestDevice(t, db, biz), newHubTestDevice(t, db, biz)
	store := HubPeerStateStore{DB: db}
	endpoint := "hub-" + randHex(4) + ".example.test:51820"

	stateA := HubPeerState{BusinessID: biz, DeviceID: a, PeerID: "peer-a", RouterPublicKey: peerKey(1), TunnelAddress: "10.77.0.2/32", Endpoint: endpoint}
	if err := store.Record(ctx, stateA); err != nil {
		t.Fatal(err)
	}
	// Recording the same router again (a retried enrolment) is idempotent.
	if err := store.Record(ctx, stateA); err != nil {
		t.Fatalf("retry must be idempotent: %v", err)
	}
	// A different router may not take the same /32 or the same key on this Hub.
	dupAddress := HubPeerState{BusinessID: biz, DeviceID: b, PeerID: "peer-b", RouterPublicKey: peerKey(2), TunnelAddress: "10.77.0.2/32", Endpoint: endpoint}
	if err := store.Record(ctx, dupAddress); !errors.Is(err, ErrTunnelAddressInUse) {
		t.Fatalf("duplicate /32 must be refused: %v", err)
	}
	dupKey := HubPeerState{BusinessID: biz, DeviceID: b, PeerID: "peer-b", RouterPublicKey: peerKey(1), TunnelAddress: "10.77.0.3/32", Endpoint: endpoint}
	if err := store.Record(ctx, dupKey); !errors.Is(err, ErrTunnelAddressInUse) {
		t.Fatalf("duplicate router key must be refused: %v", err)
	}
	// The same address on a different Hub endpoint is a different pool.
	otherHub := dupAddress
	otherHub.Endpoint = "other-" + randHex(4) + ".example.test:51820"
	if err := store.Record(ctx, otherHub); err != nil {
		t.Fatalf("a different Hub may reuse the address: %v", err)
	}
	// Shapes the database refuses outright.
	for _, bad := range []HubPeerState{
		{BusinessID: biz, DeviceID: a, PeerID: "p", RouterPublicKey: "short", TunnelAddress: "10.77.0.9/32", Endpoint: endpoint},
		{BusinessID: biz, DeviceID: a, PeerID: "p", RouterPublicKey: peerKey(3), TunnelAddress: "10.77.0.9/24", Endpoint: endpoint},
	} {
		if err := store.Record(ctx, bad); err == nil {
			t.Fatalf("malformed peer state accepted: %+v", bad)
		}
	}
}

func TestPeerStateConcurrentEnrolmentsNeverShareAnAddress(t *testing.T) {
	db := hubEnrollmentDB(t)
	ctx := context.Background()
	biz := insertBusiness(t, db)
	store := HubPeerStateStore{DB: db}
	endpoint := "race-" + randHex(4) + ".example.test:51820"
	const routers = 12
	var wins, refused int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < routers; i++ {
		device := newHubTestDevice(t, db, biz)
		key := peerKey(byte(i + 4))
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := store.Record(ctx, HubPeerState{BusinessID: biz, DeviceID: device, PeerID: "peer-" + device[:8], RouterPublicKey: key, TunnelAddress: "10.88.0.2/32", Endpoint: endpoint})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				wins++
			case errors.Is(err, ErrTunnelAddressInUse):
				refused++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins != 1 || refused != routers-1 {
		t.Fatalf("one /32 must belong to exactly one router: wins=%d refused=%d", wins, refused)
	}
}

func TestPeerStateHandshakeMovesForwardOnlyAndResetsOnKeyChange(t *testing.T) {
	db := hubEnrollmentDB(t)
	ctx := context.Background()
	biz := insertBusiness(t, db)
	device := newHubTestDevice(t, db, biz)
	store := HubPeerStateStore{DB: db}
	endpoint := "hs-" + randHex(4) + ".example.test:51820"
	st := HubPeerState{BusinessID: biz, DeviceID: device, PeerID: "peer-x", RouterPublicKey: peerKey(20), TunnelAddress: "10.99.0.2/32", Endpoint: endpoint}
	if err := store.Record(ctx, st); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(ctx, biz, device); got == nil || got.LastHandshakeAt != nil {
		t.Fatal("a new allocation has no handshake")
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.TouchHandshake(ctx, biz, device, now); err != nil {
		t.Fatal(err)
	}
	if err := store.TouchHandshake(ctx, biz, device, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(ctx, biz, device)
	if got == nil || got.LastHandshakeAt == nil || !got.LastHandshakeAt.Equal(now) {
		t.Fatalf("handshake time must not move backwards: %+v", got)
	}
	// Re-recording with the same key keeps it; a replaced key forgets it.
	_ = store.Record(ctx, st)
	if got, _ = store.Get(ctx, biz, device); got.LastHandshakeAt == nil {
		t.Fatal("same key keeps the handshake")
	}
	st.RouterPublicKey = peerKey(21)
	if err := store.Record(ctx, st); err != nil {
		t.Fatal(err)
	}
	if got, _ = store.Get(ctx, biz, device); got.LastHandshakeAt != nil {
		t.Fatal("a new router key must not inherit the old handshake")
	}
	if missing, err := store.Get(ctx, biz, newHubTestDevice(t, db, biz)); err != nil || missing != nil {
		t.Fatal("unknown router has no state")
	}
	if err := store.Delete(ctx, biz, device); err != nil {
		t.Fatal(err)
	}
	if got, _ = store.Get(ctx, biz, device); got != nil {
		t.Fatal("delete forgets the allocation")
	}
}

func TestHubEnrollmentTablesAreServiceOnlyAndCascade(t *testing.T) {
	db := hubEnrollmentDB(t)
	biz := insertBusiness(t, db)
	device := newHubTestDevice(t, db, biz)
	actor := newUUID(t, db)
	if _, _, err := (HubCallbackTokenStore{DB: db}).Issue(context.Background(), biz, device, actor, time.Minute); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"hub_callback_tokens", "hub_peer_state"} {
		var forced, tenantAccess bool
		if err := db.QueryRow(`SELECT relrowsecurity AND relforcerowsecurity, has_table_privilege('authenticated',oid,'SELECT,INSERT,UPDATE,DELETE') FROM pg_class WHERE oid=to_regclass('public.'||$1)`, table).Scan(&forced, &tenantAccess); err != nil {
			t.Fatal(err)
		}
		if !forced || tenantAccess {
			t.Fatalf("%s must force row security and be closed to the tenant role", table)
		}
	}
	if _, err := db.Exec(`DELETE FROM devices WHERE device_id=$1`, device); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.QueryRow(`SELECT count(*) FROM hub_callback_tokens WHERE device_id=$1`, device).Scan(&left); err != nil || left != 0 {
		t.Fatal("tokens must go with the router")
	}
}

func TestHubEnrollmentTablesPassTheStartupCheck(t *testing.T) {
	db := hubEnrollmentDB(t)
	if err := CheckHubEnrollmentTables(db); err != nil {
		t.Fatalf("migrated database must pass: %v", err)
	}
	if _, err := db.Exec(`GRANT SELECT ON hub_peer_state TO authenticated`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`REVOKE ALL ON hub_peer_state FROM authenticated`)
	if err := CheckHubEnrollmentTables(db); err == nil {
		t.Fatal("tenant access to the peer state must stop startup")
	}
}

func TestPendingEnrollmentRewriteScrubsASecretButKeepsTheRow(t *testing.T) {
	db := hubEnrollmentDB(t)
	ctx := context.Background()
	key, err := secretbox.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatal(err)
	}
	store := PendingEnrollmentStore{DB: db, Box: box}
	biz := insertBusiness(t, db)
	device := newHubTestDevice(t, db, biz)

	// No row yet: nothing to scrub, and that is not an error.
	if err := store.Rewrite(ctx, biz, device, "management", func(string) (string, error) { return "x", nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOrCreate(ctx, biz, device, "management", func() (string, error) { return "tunnel private-key=SECRET end", nil }, time.Hour); err != nil {
		t.Fatal(err)
	}
	var before time.Time
	if err := db.QueryRow(`SELECT expires_at FROM hub_pending_enrollments WHERE business_id=$1 AND device_id=$2 AND purpose='management'`, biz, device).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := store.Rewrite(ctx, biz, device, "management", func(v string) (string, error) {
		return strings.Replace(v, "SECRET", "removed", 1), nil
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetOrCreate(ctx, biz, device, "management", nil, 0)
	if err != nil || got != "tunnel private-key=removed end" {
		t.Fatalf("rewritten value must read back through the normal path: %q %v", got, err)
	}
	var after time.Time
	var cipher string
	if err := db.QueryRow(`SELECT expires_at,ciphertext FROM hub_pending_enrollments WHERE business_id=$1 AND device_id=$2 AND purpose='management'`, biz, device).Scan(&after, &cipher); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Fatal("the expiry must not move")
	}
	if strings.Contains(cipher, "SECRET") || strings.Contains(cipher, "removed") {
		t.Fatal("the stored value must stay encrypted")
	}
	// A failing transform leaves the stored value as it was.
	if err := store.Rewrite(ctx, biz, device, "management", func(string) (string, error) { return "", errors.New("boom") }); err == nil {
		t.Fatal("a transform error must surface")
	}
	if got, _ = store.GetOrCreate(ctx, biz, device, "management", nil, 0); got != "tunnel private-key=removed end" {
		t.Fatal("a failed rewrite must not change the row")
	}
	// An expired row is left alone.
	if _, err := db.Exec(`UPDATE hub_pending_enrollments SET expires_at=clock_timestamp()-interval '1 minute' WHERE business_id=$1 AND device_id=$2`, biz, device); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := store.Rewrite(ctx, biz, device, "management", func(v string) (string, error) { called = true; return v, nil }); err != nil || called {
		t.Fatal("an expired row must not be rewritten")
	}
	for _, bad := range []string{"", "other"} {
		if err := store.Rewrite(ctx, biz, device, bad, func(v string) (string, error) { return v, nil }); err == nil {
			t.Fatalf("purpose %q must be refused", bad)
		}
	}
	// The router key row can be deleted on its own.
	if _, err := store.GetOrCreate(ctx, biz, device, "router_key", func() (string, error) { return "k", nil }, RouterKeyLifetime); err != nil {
		t.Fatal(err)
	}
	if err := store.DeletePurpose(ctx, biz, device, "router_key"); err != nil {
		t.Fatal(err)
	}
	if v, _ := store.GetOrCreate(ctx, biz, device, "router_key", nil, 0); v != "" {
		t.Fatal("the router key must be gone")
	}
}

func TestRouterKeyLifetimeIsShortAndWithinTheStoreCap(t *testing.T) {
	if RouterKeyLifetime > time.Hour || RouterKeyLifetime <= 0 || RouterKeyLifetime > PendingEnrollmentLifetime {
		t.Fatalf("a server-held router key may live at most an hour, got %v", RouterKeyLifetime)
	}
}
