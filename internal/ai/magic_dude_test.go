package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenRouterMagicDudeChat(t *testing.T) {
	client := NewOpenRouterClient("https://openrouter.test/v1", "secret-key", "test/model:free")
	client.http = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer secret-key" {
			t.Fatalf("authorization = %q", got)
		}
		body, _ := io.ReadAll(req.Body)
		payload := string(body)
		for _, want := range []string{`"role":"system"`, `"role":"assistant"`, `"role":"user"`, `"max_tokens":700`} {
			if !strings.Contains(payload, want) {
				t.Fatalf("payload missing %s: %s", want, payload)
			}
		}
		if strings.Contains(payload, "response_format") {
			t.Fatalf("chat must not force JSON: %s", payload)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":" Check the tunnel first. "}}],"usage":{"prompt_tokens":12,"completion_tokens":5,"total_tokens":17}}`)),
			Request:    req,
		}, nil
	})}

	reply, err := client.Chat(context.Background(), "system context", []ChatMessage{{Role: "assistant", Content: "Earlier answer"}}, "What should I check?")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Answer != "Check the tunnel first." || reply.TotalTokens != 17 || reply.Model != "test/model:free" {
		t.Fatalf("unexpected reply: %+v", reply)
	}
}

func TestOpenRouterMagicDudeRequiresKey(t *testing.T) {
	client := NewOpenRouterClient("https://openrouter.test/v1", "", "test/model:free")
	if _, err := client.Chat(context.Background(), "system", nil, "question"); err == nil {
		t.Fatal("expected missing-key error")
	}
}

func TestOpenRouterMagicDudeRateLimitIsTyped(t *testing.T) {
	client := NewOpenRouterClient("https://openrouter.test/v1", "key", "openrouter/free")
	client.http = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"Rate limit exceeded"}}`)),
			Request:    req,
		}, nil
	})}
	_, err := client.Chat(context.Background(), "system", nil, "question")
	var providerErr *OpenRouterError
	if !errors.As(err, &providerErr) || providerErr.Status != http.StatusTooManyRequests {
		t.Fatalf("expected typed 429, got %T: %v", err, err)
	}
}
