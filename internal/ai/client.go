package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mega-magic/internal/model"
)

// Finding is the analysis of one telemetry sample. An empty Severity means the
// sample is healthy and no finding is emitted.
type Finding struct {
	Rule       string  `json:"rule"`       // e.g. "latency_spike" or a model label
	Severity   string  `json:"severity"`   // "info"|"warn"|"critical"|"" (empty = healthy)
	Summary    string  `json:"summary"`    //
	Confidence float64 `json:"confidence"` //
}

// ModelClient is the pluggable model contract. Analyze classifies one sample and
// returns a nil Finding when the sample is healthy.
type ModelClient interface {
	Analyze(ctx context.Context, sample model.TelemetrySample) (*Finding, error)
}

// ---------------------------------------------------------------------------
// RuleFallbackClient — deterministic, no network. Also the safety net when the
// model transport is unreachable (mode "auto"/"fallback").
// ---------------------------------------------------------------------------

type RuleFallbackClient struct{}

func NewRuleFallbackClient() *RuleFallbackClient { return &RuleFallbackClient{} }

func (c *RuleFallbackClient) Analyze(_ context.Context, sample model.TelemetrySample) (*Finding, error) {
	if sample.LatencyMs != nil && *sample.LatencyMs > 400 {
		return &Finding{
			Rule:       "latency_spike",
			Severity:   "critical",
			Summary:    fmt.Sprintf("latency %dms exceeds 400ms threshold", *sample.LatencyMs),
			Confidence: 1.0,
		}, nil
	}
	if sample.CPUPercent != nil && *sample.CPUPercent > 85 {
		return &Finding{
			Rule:       "cpu_high",
			Severity:   "warn",
			Summary:    fmt.Sprintf("cpu %.1f%% exceeds 85%% threshold", *sample.CPUPercent),
			Confidence: 1.0,
		}, nil
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// OllamaClient — talks to a local Ollama over plain HTTP (stdlib only).
// ---------------------------------------------------------------------------

// defaultOllamaModel is used when the request is issued; a per-plan model is a
// later refinement.
const defaultOllamaModel = "llama3.2"

type OllamaClient struct {
	url   string
	model string
	http  *http.Client
}

func NewOllamaClient(url string) *OllamaClient {
	if strings.TrimSpace(url) == "" {
		url = "http://127.0.0.1:11434"
	}
	return &OllamaClient{
		url:   strings.TrimRight(url, "/"),
		model: defaultOllamaModel,
		http:  &http.Client{Timeout: 20 * time.Second},
	}
}

type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Format string `json:"format"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
}

func (c *OllamaClient) Analyze(ctx context.Context, sample model.TelemetrySample) (*Finding, error) {
	prompt := buildPrompt(sample)

	body, err := json.Marshal(ollamaGenerateRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
		Format: "json",
	})
	if err != nil {
		return nil, fmt.Errorf("ai: marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ai: build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai: ollama unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ai: ollama status %d", resp.StatusCode)
	}

	var gen ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&gen); err != nil {
		return nil, fmt.Errorf("ai: decode ollama envelope: %w", err)
	}

	raw := strings.TrimSpace(gen.Response)
	if raw == "" {
		return nil, nil
	}

	var f Finding
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		return nil, fmt.Errorf("ai: decode finding: %w", err)
	}
	if strings.TrimSpace(f.Severity) == "" {
		return nil, nil // healthy
	}
	return &f, nil
}

func buildPrompt(sample model.TelemetrySample) string {
	var b strings.Builder
	b.WriteString("You are a network health classifier. Reply ONLY with a JSON object ")
	b.WriteString(`{"rule":string,"severity":"info"|"warn"|"critical"|"","summary":string,"confidence":number}.`)
	b.WriteString(" Use severity \"\" when healthy.\n")
	fmt.Fprintf(&b, "subject_kind=%s subject_id=%s\n", sample.SubjectKind, sample.SubjectID)
	if sample.CPUPercent != nil {
		fmt.Fprintf(&b, "cpu_pct=%.1f\n", *sample.CPUPercent)
	}
	if sample.MemoryPercent != nil {
		fmt.Fprintf(&b, "memory_pct=%.1f\n", *sample.MemoryPercent)
	}
	if sample.LatencyMs != nil {
		fmt.Fprintf(&b, "latency_ms=%d\n", *sample.LatencyMs)
	}
	if sample.WANState != nil {
		fmt.Fprintf(&b, "wan_state=%s\n", *sample.WANState)
	}
	if sample.ConnectorState != nil {
		fmt.Fprintf(&b, "connector_state=%s\n", *sample.ConnectorState)
	}
	if sample.TunnelState != nil {
		fmt.Fprintf(&b, "tunnel_state=%s\n", *sample.TunnelState)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// ChainClient — try a primary client, fall back on error (mode "auto").
// ---------------------------------------------------------------------------

type ChainClient struct {
	primary  ModelClient
	fallback ModelClient
}

func NewChainClient(primary, fallback ModelClient) *ChainClient {
	return &ChainClient{primary: primary, fallback: fallback}
}

func (c *ChainClient) Analyze(ctx context.Context, sample model.TelemetrySample) (*Finding, error) {
	f, err := c.primary.Analyze(ctx, sample)
	if err != nil && c.fallback != nil {
		return c.fallback.Analyze(ctx, sample)
	}
	return f, err
}

// NewClient selects the ModelClient for a mode:
//
//	"fallback" -> deterministic rules only
//	"ollama"   -> Ollama only (errors surface, no spend)
//	"auto"/""  -> Ollama with deterministic fallback when unreachable
func NewClient(cfg Config) ModelClient {
	switch strings.ToLower(strings.TrimSpace(cfg.Mode)) {
	case "fallback":
		return NewRuleFallbackClient()
	case "ollama":
		return NewOllamaClient(cfg.OllamaURL)
	default:
		return NewChainClient(NewOllamaClient(cfg.OllamaURL), NewRuleFallbackClient())
	}
}
