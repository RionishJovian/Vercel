package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"mega-magic/internal/model"
)

func TestPublicWebhookResponseOmitsSigningSecret(t *testing.T) {
	secret := "sensitive-webhook-signing-secret"
	original := &model.WebhookEndpoint{
		ID: "hook-id", BusinessID: "business-id",
		URL: "https://example.org/hook", Secret: &secret,
		EventTypes: []string{"incident.opened"}, Enabled: true,
	}
	out := publicWebhook(original)
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "\"has_secret\":true") {
		t.Fatalf("expected presence indicator: %s", text)
	}
	if strings.Contains(text, secret) || strings.Contains(text, "\"secret\"") {
		t.Fatalf("webhook signing secret leaked to API response: %s", text)
	}
	if !strings.Contains(text, "\"id\":\"hook-id\"") {
		t.Fatalf("webhook identity lost: %s", text)
	}
	if original.Secret == nil || *original.Secret != secret {
		t.Fatal("redaction changed the original secret required for server-side signing")
	}
	// The response itself must not retain the secret pointer.
	if out.WebhookEndpoint.Secret != nil {
		t.Fatal("response still references a secret")
	}
}

func TestPublicWebhookResponseWithoutSecret(t *testing.T) {
	out := publicWebhook(&model.WebhookEndpoint{ID: "empty"})
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "\"secret\"") || !strings.Contains(string(b), "\"has_secret\":false") {
		t.Fatalf("invalid public webhook response: %s", string(b))
	}
}
