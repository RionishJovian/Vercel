package database

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"mega-magic/internal/model"
	"mega-magic/internal/secretbox"
)

func rotationBox(t *testing.T, fill byte) (*secretbox.Box, string) {
	t.Helper()
	key := hex.EncodeToString([]byte(strings.Repeat(string([]byte{fill}), secretbox.KeyLen)))
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatal(err)
	}
	return box, key
}

func TestDBRotateCredentialKeyIsAtomicVerifiedAndReversibleUntilCommit(t *testing.T) {
	db := requireHarness(t)
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('public.hub_pending_enrollments') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		applyMigrationFile(t, db, "20261024160000_hub_pending_enrollments.sql")
		// Other tests apply this non-idempotent migration themselves; leave no trace.
		t.Cleanup(func() { _, _ = db.Exec(`DROP TABLE IF EXISTS hub_pending_enrollments`) })
	}
	ctx := context.Background()
	oldBox, _ := rotationBox(t, 'o')
	newBox, _ := rotationBox(t, 'n')
	thirdBox, _ := rotationBox(t, 't')
	const fp = "SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	bizA, bizB := insertBusiness(t, db), insertBusiness(t, db)
	devA := insertDeviceRow(t, db, bizA, nil, "rot A", "MAIN_GATEWAY")
	devB := insertDeviceRow(t, db, bizB, nil, "rot B", "MAIN_GATEWAY")
	srcOld := NewDeviceCredentialSource(db, oldBox)
	if err := srcOld.SetDeviceCredentials(ctx, bizA, devA, "10.77.0.8", "mm-a", "password", "secret-A", 22, fp); err != nil {
		t.Fatal(err)
	}
	if err := srcOld.SetDeviceCredentials(ctx, bizB, devB, "10.77.0.9", "mm-b", "key", "-----BEGIN KEY-----\nsecret-B\n-----END KEY-----", 22, fp); err != nil {
		t.Fatal(err)
	}
	live, _ := oldBox.Seal("pending-live")
	dead, _ := oldBox.Seal("pending-expired")
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO hub_pending_enrollments(business_id,device_id,purpose,ciphertext,expires_at) VALUES($1,$2,'router_key',$3,$4)`, bizA, devA, live, time.Now().Add(time.Hour))
	mustExec(`INSERT INTO hub_pending_enrollments(business_id,device_id,purpose,ciphertext,expires_at) VALUES($1,$2,'management',$3,$4)`, bizA, devA, dead, time.Now().Add(-time.Hour))

	// Integration secrets sealed under the old key (and one legacy plaintext) must follow the rotation.
	SetFieldBox(oldBox)
	t.Cleanup(func() {
		SetFieldBox(nil)
		_, _ = db.Exec(`DELETE FROM business_telegram_bots WHERE business_id IN ($1,$2)`, bizA, bizB)
		_, _ = db.Exec(`DELETE FROM webhook_endpoints WHERE business_id IN ($1,$2)`, bizA, bizB)
	})
	if err := NewBusinessTelegramBotRepository(db).Upsert(&model.BusinessTelegramBot{BusinessID: bizA, BotToken: "111:rot-token", TargetChatID: 1, ApproverUserID: 2}); err != nil {
		t.Fatal(err)
	}
	hookSecret := "rot-hook-secret"
	hookRepo := NewWebhookRepository(db)
	hook, err := hookRepo.CreateEndpoint(&model.WebhookEndpoint{BusinessID: bizB, URL: "https://example.com/h", Secret: &hookSecret, EventTypes: []string{"x"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	mustExec(`UPDATE webhook_endpoints SET secret='legacy-plain' WHERE id=$1`, hook.ID)

	sealedOf := func(dev string) string {
		var s string
		if err := db.QueryRow(`SELECT ssh_secret_ciphertext FROM devices WHERE device_id=$1`, dev).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := sealedOf(devA)

	// A faithful dry run: counts and verifies everything, changes nothing.
	r, err := db.RotateCredentialKey(ctx, oldBox, newBox, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.Applied || r.DeviceSecrets < 2 || r.PendingEnrollments < 1 || r.ExpiredEnrollments < 1 || r.VerifiedUnderNewKey != r.DeviceSecrets+r.PendingEnrollments+r.BotTokens+r.WebhookSecrets {
		t.Fatalf("dry-run report = %+v", r)
	}
	if sealedOf(devA) != before {
		t.Fatal("dry run changed stored data")
	}

	// One value the current key cannot open must abort everything.
	third, _ := thirdBox.Seal("someone else's")
	devC := insertDeviceRow(t, db, bizA, nil, "rot C", "MAIN_GATEWAY")
	mustExec(`UPDATE devices SET ssh_username='x', ssh_secret_kind='password', ssh_secret_ciphertext=$1, ssh_host_key_fingerprint=$2, management_ip='10.77.0.10' WHERE device_id=$3`, third, fp, devC)
	if _, err := db.RotateCredentialKey(ctx, oldBox, newBox, true, time.Now()); err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("an unreadable value must abort the rotation, got %v", err)
	}
	if sealedOf(devA) != before {
		t.Fatal("a failed rotation partially changed data")
	}
	mustExec(`UPDATE devices SET ssh_secret_ciphertext=NULL, ssh_username=NULL, ssh_secret_kind=NULL, ssh_host_key_fingerprint=NULL, management_ip=NULL WHERE device_id=$1`, devC)

	// A new key identical to the current one is refused.
	if _, err := db.RotateCredentialKey(ctx, oldBox, oldBox, true, time.Now()); err == nil {
		t.Fatal("rotating to the same key must be refused")
	}

	// The real rotation.
	r, err = db.RotateCredentialKey(ctx, oldBox, newBox, true, time.Now())
	if err != nil || !r.Applied {
		t.Fatalf("apply failed: %+v %v", r, err)
	}
	if _, err := oldBox.Open(sealedOf(devA)); err == nil {
		t.Fatal("the retired key still opens stored data")
	}
	for dev, want := range map[string]string{devA: "secret-A", devB: "-----BEGIN KEY-----\nsecret-B\n-----END KEY-----"} {
		got, err := newBox.Open(sealedOf(dev))
		if err != nil || got != want {
			t.Fatalf("device %s secret changed or unreadable under the new key", dev)
		}
	}
	// What the application actually reads must still be the same login.
	target, err := NewDeviceCredentialSource(db, newBox).TargetForDevice(ctx, bizA, devA)
	if err != nil || target.Secret != "secret-A" || target.User != "mm-a" {
		t.Fatalf("application cannot read the rotated credential: %v", err)
	}
	var ct string
	if err := db.QueryRow(`SELECT ciphertext FROM hub_pending_enrollments WHERE device_id=$1 AND purpose='router_key'`, devA).Scan(&ct); err != nil {
		t.Fatal(err)
	}
	if p, err := newBox.Open(ct); err != nil || p != "pending-live" {
		t.Fatal("live pending enrollment was not preserved under the new key")
	}
	SetFieldBox(newBox)
	if b, err := NewBusinessTelegramBotRepository(db).GetByBusiness(bizA); err != nil || b.BotToken != "111:rot-token" {
		t.Fatalf("bot token after rotation = %+v, %v", b, err)
	}
	if h, err := hookRepo.GetEndpointByID(hook.ID); err != nil || *h.Secret != "legacy-plain" {
		t.Fatalf("webhook secret after rotation = %+v, %v", h, err)
	}
	var left int
	_ = db.QueryRow(`SELECT count(*) FROM hub_pending_enrollments WHERE device_id=$1 AND purpose='management'`, devA).Scan(&left)
	if left != 0 {
		t.Fatal("expired pending enrollment should have been deleted")
	}
}
