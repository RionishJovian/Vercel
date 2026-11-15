package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// A router that lags or drops off the tunnel during a payment decision must give the owner a clear answer they
// can retry, not a raw 500. This runs without a database, unlike guest_decide_timeout_test.go.
func TestGuestRouterFailureIsAClearRetryableAnswer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	raw := fmt.Errorf("ssh dial 10.77.0.9:22: %w", context.DeadlineExceeded)
	err := guestRouterRetry(raw)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("the cause must stay in the chain for logs and callers")
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondErr(c, err)
	var body map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &body); e != nil {
		t.Fatal(e)
	}
	if rec.Code != http.StatusServiceUnavailable || body["retryable"] != true || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("want a retryable 503 with Retry-After, got %d %v", rec.Code, body)
	}
	msg, _ := body["error"].(string)
	if !strings.Contains(msg, "still waiting") || strings.Contains(msg, "10.77.0.9") || strings.Contains(msg, "deadline") {
		t.Fatalf("the owner must read plain words and no tunnel address: %q", msg)
	}
}

// Answers the handler already decided (already decided, not found, invalid allowance) keep their own status and
// are not presented as something to retry.
func TestGuestRouterRetryLeavesDecidedAnswersAlone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if guestRouterRetry(nil) != nil {
		t.Fatal("no error must stay no error")
	}
	conflict := &httpError{status: http.StatusConflict, msg: "request already decided"}
	if got := guestRouterRetry(conflict); got != error(conflict) {
		t.Fatalf("an existing answer must pass through unchanged, got %#v", got)
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	respondErr(c, conflict)
	if rec.Code != http.StatusConflict || strings.Contains(rec.Body.String(), "retryable") || rec.Header().Get("Retry-After") != "" {
		t.Fatalf("a final answer must not look retryable: %d %s", rec.Code, rec.Body.String())
	}
}
