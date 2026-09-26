package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mega-magic/internal/model"
)

func orEnvelope(content string) string {
	env := map[string]any{
		"choices": []map[string]any{
			{"message": map[string]string{"role": "assistant", "content": content}},
		},
	}
	b, _ := json.Marshal(env)
	return string(b)
}

func orSample() model.TelemetrySample {
	return model.TelemetrySample{BusinessID: "biz-1", SubjectKind: "router", SubjectID: "sub-1"}
}

func TestOpenRouterClient_parseFinding(t *testing.T) {
	const key = "test-key-abc"
	probed := `{"rule":"high_cpu_and_latency","severity":"critical","summary":"cpu and latency both high","confidence":0.95}`

	var gotAuth string
	var gotBody openRouterRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		_, _ = w.Write([]byte(orEnvelope(probed)))
	}))
	defer srv.Close()

	c := NewOpenRouterClient(srv.URL, key, "test/model")
	f, err := c.Analyze(context.Background(), orSample())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if f == nil {
		t.Fatal("expected a finding")
	}
	if f.Rule != "high_cpu_and_latency" || f.Severity != "critical" || f.Summary != "cpu and latency both high" || f.Confidence != 0.95 {
		t.Fatalf("finding fields wrong: %+v", f)
	}
	if gotAuth != "Bearer "+key {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody.ResponseFormat["type"] != "json_object" {
		t.Fatalf("response_format = %v, want json_object (mandatory)", gotBody.ResponseFormat)
	}
	if gotBody.Model != "test/model" || gotBody.MaxTokens != 250 {
		t.Fatalf("model/max_tokens wrong: %q/%d", gotBody.Model, gotBody.MaxTokens)
	}
}

func TestOpenRouterClient_healthyEmptySeverity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(orEnvelope(`{"rule":"","severity":"","summary":"all good","confidence":1}`)))
	}))
	defer srv.Close()

	c := NewOpenRouterClient(srv.URL, "k", "")
	f, err := c.Analyze(context.Background(), orSample())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if f != nil {
		t.Fatalf("empty severity => nil finding, got %+v", f)
	}
}

func TestOpenRouterClient_statusError(t *testing.T) {
	const key = "super-secret-key"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewOpenRouterClient(srv.URL, key, "")
	_, err := c.Analyze(context.Background(), orSample())
	if err == nil {
		t.Fatal("500 must surface an error (so the chain can fall back)")
	}
	// The API key must never leak into error text.
	if strings.Contains(err.Error(), key) {
		t.Fatalf("error text leaked the API key: %v", err)
	}
}

func TestOpenRouterClient_unparseableProse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(orEnvelope("The network looks a bit slow today, nothing alarming.")))
	}))
	defer srv.Close()

	c := NewOpenRouterClient(srv.URL, "k", "")
	f, err := c.Analyze(context.Background(), orSample())
	if err == nil {
		t.Fatalf("prose must error, not yield a zero finding (got %+v)", f)
	}
	if f != nil {
		t.Fatalf("prose must not yield a finding, got %+v", f)
	}
}

func TestOpenRouterClient_unreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // now unreachable

	c := NewOpenRouterClient(url, "k", "")
	if _, err := c.Analyze(context.Background(), orSample()); err == nil {
		t.Fatal("unreachable transport must surface an error")
	}
}

func TestNewClient_modeOpenrouter(t *testing.T) {
	c := NewClient(Config{Mode: "openrouter", OpenRouterURL: "https://example.test", OpenRouterAPIKey: "k"})
	if _, ok := c.(*OpenRouterClient); !ok {
		t.Fatalf("mode openrouter => *OpenRouterClient, got %T", c)
	}
}

func TestNewClient_modeDefaultIsChain(t *testing.T) {
	c := NewClient(Config{Mode: "auto"})
	if _, ok := c.(*ChainClient); !ok {
		t.Fatalf("mode auto => *ChainClient, got %T", c)
	}
	// auto must NOT be an OpenRouter client.
	if _, ok := c.(*OpenRouterClient); ok {
		t.Fatal("auto must never return an OpenRouter client")
	}
}

func TestNewClient_defaultModelApplied(t *testing.T) {
	c := NewOpenRouterClient("", "", "")
	if c.model != defaultOpenRouterModel {
		t.Fatalf("model = %q, want default %q", c.model, defaultOpenRouterModel)
	}
	if c.url != defaultOpenRouterURL {
		t.Fatalf("url = %q, want default %q", c.url, defaultOpenRouterURL)
	}
}
