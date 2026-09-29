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

const (
	defaultOpenRouterURL   = "https://openrouter.ai/api/v1"
	defaultOpenRouterModel = "nvidia/nemotron-3-super-120b-a12b:free"
)

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
	if strings.TrimSpace(model) == "" {
		model = defaultOpenRouterModel
	}
	return &OpenRouterClient{
		url:    strings.TrimRight(url, "/"),
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 25 * time.Second},
	}
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
		// NOTE: never include the request/headers (and thus the key) in the error.
		return nil, fmt.Errorf("ai: openrouter status %d", resp.StatusCode)
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
