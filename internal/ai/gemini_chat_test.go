package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGeminiChatMapsRolesAndUsageWithoutThoughtDisclosure(t *testing.T) {
	client := NewGeminiChatClient("test-private-key", "gemini-3.8-flash")
	client.http = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("x-goog-api-key") != "test-private-key" || strings.Contains(req.URL.String(), "test-private-key") {
			t.Fatal("key must appear only in the auth header")
		}
		if req.URL.Host != "generativelanguage.googleapis.com" || req.URL.RawQuery != "" {
			t.Fatal("unexpected provider URL")
		}
		var payload struct {
			SystemInstruction geminiContent   `json:"systemInstruction"`
			Contents          []geminiContent `json:"contents"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.SystemInstruction.Parts[0].Text != "tenant system" || len(payload.Contents) != 3 || payload.Contents[1].Role != "model" || payload.Contents[2].Parts[0].Text != "question" {
			t.Fatalf("bad provider role/context mapping: %+v", payload)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"private reasoning","thought":true},{"text":" Check the tunnel. "}]},"finishReason":"STOP"}],"modelVersion":"gemini-test-version","usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":5,"thoughtsTokenCount":3,"totalTokenCount":20}}`))}, nil
	})}
	reply, err := client.Chat(context.Background(), "tenant system", []ChatMessage{{Role: "user", Content: "earlier"}, {Role: "assistant", Content: "answer"}}, "question")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Answer != "Check the tunnel." || reply.CompletionTokens != 8 || reply.TotalTokens != 20 || reply.Model != "gemini-test-version" {
		t.Fatalf("bad answer/usage: %+v", reply)
	}
}

func TestGeminiChatErrorsDoNotExposeProviderBody(t *testing.T) {
	client := NewGeminiChatClient("secret", "gemini-3.8-flash")
	client.http = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"secret echoed"}}`))}, nil
	})}
	_, err := client.Chat(context.Background(), "tenant", nil, "question")
	var providerErr *GeminiError
	if !errors.As(err, &providerErr) || providerErr.Status != 429 || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestGeminiChatFailsClosedWithoutKeyOrWithUnsafeModel(t *testing.T) {
	for _, client := range []*GeminiChatClient{NewGeminiChatClient("", "gemini-3.8-flash"), NewGeminiChatClient("key", "gemini-x?key=bad")} {
		if _, err := client.Chat(context.Background(), "system", nil, "question"); err == nil {
			t.Fatal("invalid provider config accepted")
		}
	}
}
