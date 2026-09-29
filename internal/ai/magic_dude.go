package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ChatMessage is one validated conversational turn supplied to Magic Dude.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatReply carries the answer and the provider's metering figures.
type ChatReply struct {
	Answer           string `json:"answer"`
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}

// MagicDudeClient is the narrow contract used by the HTTP feature. Keeping it
// separate from telemetry classification makes the handler easy to test and
// prevents a chat request from changing the automated incident pipeline.
type MagicDudeClient interface {
	Chat(ctx context.Context, systemPrompt string, history []ChatMessage, question string) (*ChatReply, error)
}

// Chat asks OpenRouter for a plain-text operations answer. The caller supplies
// a redacted, tenant-scoped system prompt; this method never logs request data.
func (c *OpenRouterClient) Chat(ctx context.Context, systemPrompt string, history []ChatMessage, question string) (*ChatReply, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, fmt.Errorf("ai: openrouter is not configured")
	}

	messages := make([]openRouterMessage, 0, len(history)+2)
	messages = append(messages, openRouterMessage{Role: "system", Content: systemPrompt})
	for _, m := range history {
		messages = append(messages, openRouterMessage{Role: m.Role, Content: m.Content})
	}
	messages = append(messages, openRouterMessage{Role: "user", Content: question})

	body, err := json.Marshal(openRouterRequest{
		Model:     c.model,
		Messages:  messages,
		MaxTokens: 700,
	})
	if err != nil {
		return nil, fmt.Errorf("ai: marshal magic dude request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ai: build magic dude request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("HTTP-Referer", "https://mikromagic.com")
	req.Header.Set("X-Title", "Mega Magic — Magic Dude")

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
		return nil, fmt.Errorf("ai: decode magic dude response: %w", err)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("ai: openrouter returned no answer")
	}
	totalTokens := out.Usage.TotalTokens
	if totalTokens == 0 {
		totalTokens = out.Usage.PromptTokens + out.Usage.CompletionTokens
	}

	return &ChatReply{
		Answer:           strings.TrimSpace(out.Choices[0].Message.Content),
		Model:            c.model,
		PromptTokens:     out.Usage.PromptTokens,
		CompletionTokens: out.Usage.CompletionTokens,
		TotalTokens:      totalTokens,
	}, nil
}
