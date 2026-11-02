package database

import (
	"context"
	"strings"
	"testing"

	"mega-magic/internal/model"
)

func TestIntegrationSecretsAreSealedAtRestAndReadBackInPlaintext(t *testing.T) {
	db := requireHarness(t)
	ctx := context.Background()
	box, _ := rotationBox(t, 'f')
	SetFieldBox(box)
	t.Cleanup(func() { SetFieldBox(nil) })

	biz := insertBusiness(t, db)
	// Other tests in this package share the database and read these tables with no key: leave nothing sealed behind.
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM business_telegram_bots WHERE business_id=$1`, biz)
		_, _ = db.Exec(`DELETE FROM webhook_endpoints WHERE business_id=$1`, biz)
	})
	bots := NewBusinessTelegramBotRepository(db)
	hooks := NewWebhookRepository(db)

	const token = "123456:ABC-plain-bot-token"
	if err := bots.Upsert(&model.BusinessTelegramBot{BusinessID: biz, BotToken: token, TargetChatID: 1, ApproverUserID: 2, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	secret := "whsec_plain"
	ep, err := hooks.CreateEndpoint(&model.WebhookEndpoint{BusinessID: biz, URL: "https://example.com/hook", Secret: &secret, EventTypes: []string{"incident.opened"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if ep.Secret == nil || *ep.Secret != secret {
		t.Fatalf("create returned %v, want the plaintext secret", ep.Secret)
	}

	var rawToken, rawSecret string
	if err := db.QueryRow(`SELECT bot_token FROM business_telegram_bots WHERE business_id=$1`, biz).Scan(&rawToken); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT secret FROM webhook_endpoints WHERE id=$1`, ep.ID).Scan(&rawSecret); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{rawToken, rawSecret} {
		if !strings.HasPrefix(raw, sealedPrefix) || strings.Contains(raw, "plain") {
			t.Fatalf("stored value is not sealed: %q", raw)
		}
	}

	got, err := bots.GetByBusiness(biz)
	if err != nil || got == nil || got.BotToken != token {
		t.Fatalf("bot read back = %+v, %v", got, err)
	}
	back, err := hooks.GetEndpointByID(ep.ID)
	if err != nil || back == nil || back.Secret == nil || *back.Secret != secret {
		t.Fatalf("webhook read back = %+v, %v", back, err)
	}
	list, err := hooks.ListEnabledForEvent(biz, "incident.opened")
	if err != nil || len(list) != 1 || *list[0].Secret != secret {
		t.Fatalf("enabled list = %+v, %v", list, err)
	}

	// Updating with the already-read plaintext seals again, never double-seals.
	if err := hooks.UpdateEndpoint(back); err != nil {
		t.Fatal(err)
	}
	if again, _ := hooks.GetEndpointByID(ep.ID); again == nil || *again.Secret != secret {
		t.Fatal("secret changed after update")
	}

	// A sealed value is unreadable without the key: an error, never the ciphertext.
	SetFieldBox(nil)
	if _, err := bots.GetByBusiness(biz); err == nil {
		t.Fatal("sealed bot token opened with no key")
	}
	if _, err := hooks.GetEndpointByID(ep.ID); err == nil {
		t.Fatal("sealed webhook secret opened with no key")
	}
	SetFieldBox(box)

	// Legacy plaintext rows still read, and the start-up pass seals them (idempotently).
	if _, err := db.Exec(`UPDATE business_telegram_bots SET bot_token=$1 WHERE business_id=$2`, token, biz); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE webhook_endpoints SET secret=$1 WHERE id=$2`, secret, ep.ID); err != nil {
		t.Fatal(err)
	}
	if g, _ := bots.GetByBusiness(biz); g == nil || g.BotToken != token {
		t.Fatal("legacy plaintext token unreadable")
	}
	nb, nh, err := EncryptLegacyFieldSecrets(ctx, db)
	if err != nil || nb < 1 || nh < 1 {
		t.Fatalf("sealed %d bots, %d hooks, err %v", nb, nh, err)
	}
	if nb, nh, _ = EncryptLegacyFieldSecrets(ctx, db); nb != 0 || nh != 0 {
		t.Fatalf("second pass sealed %d, %d", nb, nh)
	}
	if g, _ := bots.GetByBusiness(biz); g == nil || g.BotToken != token {
		t.Fatal("token unreadable after sealing")
	}
}
