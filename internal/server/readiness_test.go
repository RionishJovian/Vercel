package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestReadinessReport(t *testing.T) {
	cases := []struct {
		name              string
		db, redis, nats   bool
		wantCode          int
		wantStatus, wantR string
	}{
		{"healthy", true, true, true, http.StatusOK, "ready", "ok"},
		{"redis down", true, false, true, http.StatusOK, "degraded", "disconnected"},
		{"nats down", true, true, false, http.StatusOK, "degraded", "ok"},
		{"database down", false, true, true, http.StatusServiceUnavailable, "not_ready", "ok"},
	}
	for _, c := range cases {
		code, body := readinessReport(c.db, c.redis, c.nats)
		if code != c.wantCode || body["status"] != c.wantStatus {
			t.Fatalf("%s: got %d %v", c.name, code, body)
		}
		checks := body["checks"].(gin.H)
		if checks["redis"] != c.wantR {
			t.Fatalf("%s: redis check %v", c.name, checks["redis"])
		}
		if !c.db && checks["database"] != "error" {
			t.Fatalf("%s: database check %v", c.name, checks["database"])
		}
		for _, v := range checks {
			if strings.Contains(v.(string), ":") {
				t.Fatalf("%s: check text leaks detail: %v", c.name, v)
			}
		}
	}
}
