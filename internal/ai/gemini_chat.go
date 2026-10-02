package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const DefaultGeminiChatModel = "gemini-3.8-flash"

var geminiModelName = regexp.MustCompile(`^gemini-[a-zA-Z0-9.-]+$`)

// GeminiError deliberately omits the upstream response body: provider errors
// can echo input data. Only the HTTP status is needed by the public handler.
type GeminiError struct{ Status int }

func (e *GeminiError) Error() string { return fmt.Sprintf("ai: gemini status %d", e.Status) }

type GeminiChatClient struct {
	apiKey, model string
	http          *http.Client
}

func NewGeminiChatClient(apiKey, model string) *GeminiChatClient {
	if model == "" {
		model = DefaultGeminiChatModel
	}
	return &GeminiChatClient{apiKey: apiKey, model: model, http: &http.Client{
		Timeout:       25 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

type geminiPart struct {
	Text    string `json:"text,omitempty"`
	Thought bool   `json:"thought,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

func (c *GeminiChatClient) Chat(ctx context.Context, systemPrompt string, history []ChatMessage, question string) (*ChatReply, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, fmt.Errorf("ai: gemini is not configured")
	}
	if !geminiModelName.MatchString(c.model) {
		return nil, fmt.Errorf("ai: invalid Gemini model name")
	}
	contents := make([]geminiContent, 0, len(history)+1)
	for _, turn := range history {
		role := turn.Role
		if role == "assistant" {
			role = "model"
		}
		if role != "model" && role != "user" {
			return nil, fmt.Errorf("ai: invalid chat history role")
		}
		contents = append(contents, geminiContent{Role: role, Parts: []geminiPart{{Text: turn.Content}}})
	}
	contents = append(contents, geminiContent{Role: "user", Parts: []geminiPart{{Text: question}}})
	payload := struct {
		SystemInstruction geminiContent   `json:"systemInstruction"`
		Contents          []geminiContent `json:"contents"`
		GenerationConfig  struct {
			MaxOutputTokens int `json:"maxOutputTokens"`
		} `json:"generationConfig"`
	}{SystemInstruction: geminiContent{Parts: []geminiPart{{Text: systemPrompt}}}, Contents: contents}
	payload.GenerationConfig.MaxOutputTokens = 2048
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ai: could not encode Gemini request")
	}
	// The endpoint is fixed. API keys never enter URLs, logs or browser responses.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://generativelanguage.googleapis.com/v1beta/models/"+c.model+":generateContent", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ai: could not create Gemini request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai: Gemini request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &GeminiError{Status: resp.StatusCode}
	}
	var out struct {
		Candidates []struct {
			Content      geminiContent `json:"content"`
			FinishReason string        `json:"finishReason"`
		} `json:"candidates"`
		ModelVersion  string `json:"modelVersion"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			ThoughtsTokenCount   int `json:"thoughtsTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("ai: could not decode Gemini response")
	}
	if len(out.Candidates) == 0 {
		return nil, fmt.Errorf("ai: Gemini returned no answer")
	}
	if reason := out.Candidates[0].FinishReason; reason != "STOP" && reason != "MAX_TOKENS" {
		return nil, fmt.Errorf("ai: Gemini could not complete the answer")
	}
	var answer strings.Builder
	for _, part := range out.Candidates[0].Content.Parts {
		if !part.Thought {
			answer.WriteString(part.Text)
		}
	}
	if strings.TrimSpace(answer.String()) == "" {
		return nil, fmt.Errorf("ai: Gemini returned no answer")
	}
	completion := out.UsageMetadata.CandidatesTokenCount + out.UsageMetadata.ThoughtsTokenCount
	total := out.UsageMetadata.TotalTokenCount
	if total == 0 {
		total = out.UsageMetadata.PromptTokenCount + completion
	}
	model := out.ModelVersion
	if model == "" {
		model = c.model
	}
	return &ChatReply{Answer: strings.TrimSpace(answer.String()), Model: model,
		PromptTokens: out.UsageMetadata.PromptTokenCount, CompletionTokens: completion, TotalTokens: total}, nil
}
