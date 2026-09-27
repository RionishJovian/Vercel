package database

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"mega-magic/internal/provisioner/routeros"
	"mega-magic/internal/secretbox"
)

// This is the end-to-end proof that the per-device transport can actually be
// resolved from real rows: credentials are written (encrypted), read back, and
// a device WITHOUT credentials is refused.
//
// It needs a live database and is therefore opt-in, so the default `go test
// ./...` run stays green without one:
//
//	MM_TEST_DSN='host=... port=5433 user=... password=... dbname=... sslmode=disable' \
//	    go test ./internal/database/ -run DeviceCredentialSource -v
func openTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("MM_TEST_DSN")
	if dsn == "" {
		t.Skip("MM_TEST_DSN not set; skipping live-database credential test")
	}
	raw, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := raw.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	return &DB{raw}
}

// pickTestDevice returns a (business_id, device_id) pair that currently has no
// SSH target, so the test owns exactly the rows it touches.
func pickTestDevice(t *testing.T, db *DB) (businessID, deviceID string) {
	t.Helper()
	err := db.QueryRow(`
		SELECT business_id::text, device_id::text
		  FROM devices
		 WHERE ssh_username IS NULL
		 ORDER BY device_id
		 LIMIT 1`).Scan(&businessID, &deviceID)
	if errors.Is(err, sql.ErrNoRows) {
		t.Skip("no unconfigured device available to test with")
	}
	if err != nil {
		t.Fatalf("pick device: %v", err)
	}
	return businessID, deviceID
}

func TestDeviceCredentialSourceRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	key, err := secretbox.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatal(err)
	}
	src := NewDeviceCredentialSource(db, box)

	businessID, deviceID := pickTestDevice(t, db)
	const (
		testAddr   = "10.99.99.42"
		testUser   = "mmtest"
		testSecret = "s3cret-do-not-log-me"
	)

	t.Cleanup(func() {
		_, _ = db.Exec(`UPDATE devices
		                   SET ssh_username = NULL, ssh_secret_ciphertext = NULL,
		                       ssh_secret_kind = NULL, ssh_host_key_fingerprint = NULL,
		                       management_ip = NULL, ssh_port = 22
		                 WHERE device_id = $1`, deviceID)
	})

	// 1. Before writing: the device must NOT resolve. This is the "unconfigured
	//    fleet" state, and it must be an honest refusal, not an empty target.
	if _, err := src.TargetForDevice(ctx, businessID, deviceID); !errors.Is(err, routeros.ErrNoTarget) {
		t.Fatalf("unconfigured device resolved: err=%v, want ErrNoTarget", err)
	}

	// 2. Write an encrypted credential.
	if err := src.SetDeviceCredentials(ctx, businessID, deviceID, testAddr, testUser, "password", testSecret, 2222, "SHA256:abc"); err != nil {
		t.Fatalf("SetDeviceCredentials: %v", err)
	}

	// 3. THE POINT OF THE CIPHER: what landed on disk must not be the secret.
	var atRest string
	if err := db.QueryRow(`SELECT ssh_secret_ciphertext FROM devices WHERE device_id = $1`, deviceID).Scan(&atRest); err != nil {
		t.Fatalf("read ciphertext: %v", err)
	}
	if atRest == "" {
		t.Fatal("ciphertext is empty — nothing was stored")
	}
	if strings.Contains(atRest, testSecret) {
		t.Fatal("the router password is stored in PLAINTEXT in the database")
	}
	if atRest == testSecret {
		t.Fatal("stored value equals the plaintext secret")
	}

	// 4. Resolve it back.
	target, err := src.TargetForDevice(ctx, businessID, deviceID)
	if err != nil {
		t.Fatalf("configured device did not resolve: %v", err)
	}
	if target.Addr != "10.99.99.42:2222" {
		t.Errorf("Addr = %q, want 10.99.99.42:2222", target.Addr)
	}
	if target.User != testUser {
		t.Errorf("User = %q, want %q", target.User, testUser)
	}
	if target.Secret != testSecret {
		t.Errorf("Secret did not round-trip (len %d)", len(target.Secret))
	}
	if target.Kind != "password" {
		t.Errorf("Kind = %q", target.Kind)
	}

	// 5. TENANT SCOPING: the SAME device id under a different tenant must NOT
	//    resolve. mega_admin is superuser+bypassrls, so RLS does not save us
	//    here — the business_id predicate is what does the work.
	wrongTenant := "00000000-0000-4000-8000-000000000000"
	if _, err := src.TargetForDevice(ctx, wrongTenant, deviceID); !errors.Is(err, routeros.ErrNoTarget) {
		t.Fatalf("cross-tenant lookup resolved! err=%v, want ErrNoTarget", err)
	}
}

// A wrong encryption key must make the device unresolvable rather than
// producing an empty password. This is the fail-closed property at the DB layer.
func TestDeviceCredentialSourceWrongKeyFailsClosed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	keyA, _ := secretbox.GenerateKey()
	boxA, _ := secretbox.New(keyA)
	// Note: NOT a db-backed object from keyA — we never log the secret.
	srcA := NewDeviceCredentialSource(db, boxA)

	businessID, deviceID := pickTestDevice(t, db)
	t.Cleanup(func() {
		_, _ = db.Exec(`UPDATE devices
		                   SET ssh_username = NULL, ssh_secret_ciphertext = NULL,
		                       ssh_secret_kind = NULL, management_ip = NULL, ssh_port = 22
		                 WHERE device_id = $1`, deviceID)
	})

	if err := srcA.SetDeviceCredentials(ctx, businessID, deviceID, "10.1.1.1", "u", "password", "secret", 22, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// A DIFFERENT key must not open it.
	keyB, _ := secretbox.GenerateKey()
	boxB, _ := secretbox.New(keyB)
	srcB := NewDeviceCredentialSource(db, boxB)

	if _, err := srcB.TargetForDevice(ctx, businessID, deviceID); !errors.Is(err, routeros.ErrNoTarget) {
		t.Fatalf("wrong key resolved the device: err=%v, want ErrNoTarget", err)
	}

	// And no key at all must refuse too — never "password is empty".
	srcNil := NewDeviceCredentialSource(db, nil)
	if _, err := srcNil.TargetForDevice(ctx, businessID, deviceID); !errors.Is(err, routeros.ErrNoTarget) {
		t.Fatalf("nil cipher resolved the device: err=%v, want ErrNoTarget", err)
	}
}
