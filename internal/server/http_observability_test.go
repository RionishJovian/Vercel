package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestReadinessFailsClosedForEachDependency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name           string
		database, redis, nats bool
		wantHTTP       int
		wantState      string
	}{
		{"healthy", true, true, true, http.StatusOK, "ready"},
		{"database down", false, true, true, http.StatusServiceUnavailable, "not_ready"},
		{"redis down", true, false, true, http.StatusServiceUnavailable, "not_ready"},
		{"nats down", true, true, false, http.StatusServiceUnavailable, "not_ready"},
		{"all down", false, false, false, http.StatusServiceUnavailable, "not_ready"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/ready", readinessCheckWithProbes(readinessProbes{
				database: func() error {
					if !tt.database {
						return errors.New("sensitive database connection details must not leak")
					}
					return nil
				},
				redis: func() bool { return tt.redis },
				nats:  func() bool { return tt.nats },
			}))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
			if w.Code != tt.wantHTTP {
				t.Fatalf("HTTP = %d, want %d: %s", w.Code, tt.wantHTTP, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("readiness must not be cached")
			}
			var response struct {
				Status string            `json:"status"`
				Checks map[string]string `json:"checks"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Status != tt.wantState {
				t.Fatalf("status = %q, want %q", response.Status, tt.wantState)
			}
			for dep, ok := range map[string]bool{"database": tt.database, "redis": tt.redis, "nats": tt.nats} {
				want := "disconnected"
				if ok {
					want = "ok"
				}
				if response.Checks[dep] != want {
					t.Fatalf("%s = %q, want %q", dep, response.Checks[dep], want)
				}
			}
			if strings.Contains(w.Body.String(), "sensitive") {
				t.Fatalf("raw database error leaked into readiness response: %s", w.Body.String())
			}
		})
	}
}

func TestReadinessMissingProbesIsUnavailable(t *testing.T) {
	r := gin.New()
	r.GET("/ready", readinessCheckWithProbes(readinessProbes{}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing dependencies passed readiness: HTTP %d", w.Code)
	}
}

func TestRequestLoggerRedactsVoucherCodesAndQueries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, recorded := observer.New(zap.InfoLevel)
	r := gin.New()
	r.Use(loggerMiddleware(zap.New(core).Sugar()))
	r.GET("/api/v1/vouchers/:code", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet,
		"/api/v1/vouchers/SECRET-VOUCHER-123?token=SECRET_QUERY_TOKEN", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet,
		"/not-a-real-path?password=SECRET_PASSWORD", nil))
	logs := recorded.All()
	if len(logs) != 2 {
		t.Fatalf("request log entries = %d, want 2", len(logs))
	}
	if got := logs[0].ContextMap()["path"]; got != "/api/v1/vouchers/:code" {
		t.Fatalf("unsafe or incorrect route template: %v", got)
	}
	if got := logs[1].ContextMap()["path"]; got != "unmatched" {
		t.Fatalf("unmatched path must be redacted, got %v", got)
	}
	for _, entry := range logs {
		fields, err := json.Marshal(entry.ContextMap())
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"SECRET-VOUCHER-123", "SECRET_QUERY_TOKEN", "SECRET_PASSWORD"} {
			if strings.Contains(string(fields), secret) {
				t.Fatalf("sensitive URL content was logged: %s", fields)
			}
		}
		if _, ok := entry.ContextMap()["query"]; ok {
			t.Fatalf("raw query field must not be logged")
		}
	}
}

func TestVersionIncludesCurrentBuildRevision(t *testing.T) {
	r := gin.New()
	r.GET("/api/v1/version", version)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/version", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("version HTTP status = %d", w.Code)
	}
	var response struct {
		Build  string `json:"build"`
		Commit string `json:"commit"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Build == "" || response.Build != response.Commit || response.Build != buildRevision {
		t.Fatalf("version endpoint lost build revision: %+v", response)
	}
}