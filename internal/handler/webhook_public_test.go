package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"mega-magic/internal/model"
)

func TestWebhookSigningSecretNeverAppearsInAPIResponse(t *testing.T) {
	const secret = "test-webhook-signing-secret-MUST-NOT-LEAK"
	ep := &model.WebhookEndpoint{
		ID:         "endpoint-1",
		BusinessID: "business-1",
		URL:        "https://example.com/hook",
		Secret:     &secret,
		Enabled:    true,
		EventTypes: []string{"device.offline"},
	}

	public := redactWebhook(ep)
	data, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) || strings.Contains(string(data), "\"secret\"") {
		t.Fatalf("API response disclosed webhook signing secret: %s", data)
	}
	if !strings.Contains(string(data), "\"has_secret\":true") {
		t.Fatalf("API response must disclose presence without the secret: %s", data)
	}
	if !strings.Contains(string(data), "\"url\":\"https://example.com/hook\"") {
		t.Fatalf("public endpoint metadata was lost: %s", data)
	}
	if ep.Secret == nil || *ep.Secret != secret {
		t.Fatal("redaction must not mutate the repository entity used for signing")
	}

	missing, err := json.Marshal(redactWebhook(&model.WebhookEndpoint{ID: "none"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(missing), "\"has_secret\":false") || strings.Contains(string(missing), "\"secret\":") {
		t.Fatalf("missing secret must have explicit false state, without a value: %s", missing)
	}
}
