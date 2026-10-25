package database

import (
	"context"
	"mega-magic/internal/secretbox"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPendingEnrollmentEncryptedRestartAndTenantBinding(t *testing.T) {
	db := requireHarness(t)
	raw, e := os.ReadFile("../../migrations/20261024160000_hub_pending_enrollments.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(string(raw)); e != nil {
		t.Fatal(e)
	}
	biz, foreign := insertBusiness(t, db), insertBusiness(t, db)
	var id, other string
	if e = db.QueryRow(`INSERT INTO devices(business_id,hardware_name)VALUES($1,'pending fixture')RETURNING device_id`, biz).Scan(&id); e != nil {
		t.Fatal(e)
	}
	if e = db.QueryRow(`INSERT INTO devices(business_id,hardware_name)VALUES($1,'foreign fixture')RETURNING device_id`, foreign).Scan(&other); e != nil {
		t.Fatal(e)
	}
	box, _ := secretbox.New(strings.Repeat("ab", 32))
	s := PendingEnrollmentStore{db, box}
	if e = s.Check(); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	var generated atomic.Int32
	var wg sync.WaitGroup
	errors := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := s.GetOrCreate(ctx, biz, id, "router_key", func() (string, error) { generated.Add(1); return "private-fixture", nil }, time.Hour)
			if e == nil && v != "private-fixture" {
				t.Error("unexpected pending value")
			}
			errors <- e
		}()
	}
	wg.Wait()
	close(errors)
	for e := range errors {
		if e != nil {
			t.Fatal(e)
		}
	}
	if generated.Load() != 1 {
		t.Fatal("concurrent requests replaced the private key")
	}
	restarted := PendingEnrollmentStore{db, box}
	v, e := restarted.GetOrCreate(ctx, biz, id, "router_key", nil, 0)
	if e != nil || v != "private-fixture" {
		t.Fatal("restart lost pending state", e)
	}
	var ciphertext string
	if e = db.QueryRow(`SELECT ciphertext FROM hub_pending_enrollments WHERE device_id=$1`, id).Scan(&ciphertext); e != nil || strings.Contains(ciphertext, "private-fixture") {
		t.Fatal("plaintext at rest", e)
	}
	if v, e = s.GetOrCreate(ctx, foreign, id, "router_key", nil, 0); e != nil || v != "" {
		t.Fatal("foreign tenant disclosed state", e)
	}
	// A valid ciphertext copied into another legitimate row must still fail closed.
	if _, e = db.Exec(`INSERT INTO hub_pending_enrollments SELECT $1,$2,purpose,ciphertext,expires_at FROM hub_pending_enrollments WHERE device_id=$3`, foreign, other, id); e != nil {
		t.Fatal(e)
	}
	if _, e = s.GetOrCreate(ctx, foreign, other, "router_key", nil, 0); e == nil {
		t.Fatal("swapped envelope accepted")
	}
	if _, e = db.Exec(`UPDATE hub_pending_enrollments SET expires_at=now()-interval '1 second' WHERE device_id=$1`, id); e != nil {
		t.Fatal(e)
	}
	if v, e = s.GetOrCreate(ctx, biz, id, "router_key", nil, 0); e != nil || v != "" {
		t.Fatal("expired secret returned", e)
	}
	if e = s.Delete(ctx, biz, id); e != nil {
		t.Fatal(e)
	}
	if v, e = s.GetOrCreate(ctx, biz, id, "router_key", nil, 0); e != nil || v != "" {
		t.Fatal("verified installation secret retained", e)
	}
}
