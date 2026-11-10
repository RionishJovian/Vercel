package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestReadinessRequiresEveryDependency(t *testing.T) {
	tests := []struct {
		name             string
		db, redis, nats  bool
		wantCode         int
		wantStatus       string
	}{
		{name: "all up", db: true, redis: true, nats: true, wantCode: http.StatusOK, wantStatus: "ready"},
		{name: "database down", db: false, redis: true, nats: true, wantCode: http.StatusServiceUnavailable, wantStatus: "not_ready"},
		{name: "redis down", db: true, redis: false, nats: true, wantCode: http.StatusServiceUnavailable, wantStatus: "not_ready"},
		{name: "nats down", db: true, redis: true, nats: false, wantCode: http.StatusServiceUnavailable, wantStatus: "not_ready"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := readinessResult(tt.db, tt.redis, tt.nats)
			if code != tt.wantCode || body["status"] != tt.wantStatus {
				t.Fatalf("readiness = (%d, %v), want (%d, %s)", code, body, tt.wantCode, tt.wantStatus)
			}
			checks, ok := body["checks"].(gin.H)
			if !ok {
				t.Fatal("readiness checks missing")
			}
			for dependency, up := range map[string]bool{"database": tt.db, "redis": tt.redis, "nats": tt.nats} {
				want := "unavailable"
				if up {
					want = "ok"
				}
				if checks[dependency] != want {
					t.Errorf("%s = %v, want %s", dependency, checks[dependency], want)
				}
			}
		})
	}
}

func TestReadinessFailsClosedWithoutDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ready", readinessCheck(&Server{}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "\"not_ready\"") {
		t.Fatalf("empty service reported ready: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "error:") {
		t.Fatalf("readiness leaked internal database error: %s", w.Body.String())
	}
}

func TestRequestLoggerNeverWritesVoucherCodeOrQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zap.InfoLevel)
	r := gin.New()
	r.Use(loggerMiddleware(zap.New(core).Sugar()))
	r.GET("/api/v1/vouchers/:code/redeem", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	secretCode := "PRIVATE-VOUCHER-CODE"
	secretQuery := "PRIVATE-QUERY-TOKEN"
	paths := []string{
		"/api/v1/vouchers/" + secretCode + "/redeem?token=" + secretQuery,
		"/not-a-route/" + secretCode + "?token=" + secretQuery,
	}
	for _, path := range paths {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	}
	events := logs.All()
	if len(events) != len(paths) {
		t.Fatalf("logged %d requests, want %d", len(events), len(paths))
	}
	for index, event := range events {
		fields := event.ContextMap()
		if _, exists := fields["query"]; exists {
			t.Fatal("raw query must not be logged")
		}
		logged := fmt.Sprint(fields)
		if strings.Contains(logged, secretCode) || strings.Contains(logged, secretQuery) {
			t.Fatalf("secrets leaked into HTTP log: %s", logged)
		}
		wantPath := "/api/v1/vouchers/:code/redeem"
		if index == 1 {
			wantPath = "[unmatched]"
		}
		if fields["path"] != wantPath {
			t.Errorf("logged path = %v, want %q", fields["path"], wantPath)
		}
	}
}
