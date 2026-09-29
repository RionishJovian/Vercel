package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mega-magic/internal/model"
)

const (
	defaultOpenRouterURL   = "https://openrouter.ai/api/v1"
	defaultOpenRouterModel = "openrouter/free"
)

// OpenRouterError preserves only the safe provider status/message needed for
// operator-facing recovery. It never includes request headers or API keys.
type OpenRouterError struct {
	Status  int
	Message string
}

func (e *OpenRouterError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("ai: openrouter status %d", e.Status)
	}
	return fmt.Sprintf("ai: openrouter status %d: %s", e.Status, e.Message)
}

func decodeOpenRouterError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	return &OpenRouterError{Status: resp.StatusCode, Message: strings.TrimSpace(envelope.Error.Message)}
}

// OpenRouterClient classifies a telemetry sample via an OpenRouter chat
// completion. response_format:json_object is MANDATORY — the probed model
// returns prose without it. Only used when ai.mode == "openrouter".
type OpenRouterClient struct {
	url    string
	apiKey string
	model  string
	http   *http.Client
}

// NewOpenRouterClient builds the client. url/model fall back to the probed
// defaults when empty. The api key is never logged or echoed.
func NewOpenRouterClient(url, apiKey, model string) *OpenRouterClient {
	if strings.TrimSpace(url) == "" {
		url = defaultOpenRouterURL
	}
	// Cost safety is fail-closed: only explicit OpenRouter free variants may be
	// used. A missing or paid model name falls back to the known free default.
	model = strings.TrimSpace(model)
	if !isFreeOpenRouterModel(model) {
		model = defaultOpenRouterModel
	}
	return &OpenRouterClient{
		url:    strings.TrimRight(url, "/"),
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 25 * time.Second},
	}
}

func isFreeOpenRouterModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == "openrouter/free" || strings.HasSuffix(model, ":free")
}

type openRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openRouterRequest struct {
	Model          string              `json:"model"`
	Messages       []openRouterMessage `json:"messages"`
	MaxTokens      int                 `json:"max_tokens"`
	ResponseFormat map[string]string   `json:"response_format,omitempty"`
}

type openRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *OpenRouterClient) Analyze(ctx context.Context, sample model.TelemetrySample) (*Finding, error) {
	body, err := json.Marshal(openRouterRequest{
		Model:          c.model,
		Messages:       []openRouterMessage{{Role: "user", Content: buildPrompt(sample)}},
		MaxTokens:      250,
		ResponseFormat: map[string]string{"type": "json_object"},
	})
	if err != nil {
		return nil, fmt.Errorf("ai: marshal openrouter request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ai: build openrouter request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai: openrouter unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeOpenRouterError(resp)
	}

	var out openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("ai: decode openrouter envelope: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("ai: openrouter returned no choices")
	}

	content := strings.TrimSpace(out.Choices[0].Message.Content)
	if content == "" {
		return nil, fmt.Errorf("ai: openrouter returned empty content")
	}

	var f Finding
	if err := json.Unmarshal([]byte(content), &f); err != nil {
		return nil, fmt.Errorf("ai: decode openrouter finding: %w", err)
	}
	if strings.TrimSpace(f.Severity) == "" {
		return nil, nil // healthy
	}
	return &f, nil
}
