package handler

import (
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBroadbandWritesRequireGateAndExplicitConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		gate, broadband, body string
		want                  int
	}{
		{"", "", `{}`, 503},
		{"true", "", `{"confirm":true}`, 503}, // the general switch alone does not open paid PPPoE
		{"", "true", `{"confirm":true}`, 503}, // nor does this one alone
		{"true", "false", `{"confirm":true}`, 503},
		{"true", "true", `{"operation_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`, 400},
	} {
		t.Setenv("MM_NETWORK_CONTROLS_ENABLED", tc.gate)
		t.Setenv("MM_BROADBAND_WRITES_ENABLED", tc.broadband)
		r := gin.New()
		h := NewSecurityHandler(nil, nil)
		r.POST("/routers/:id/broadband/accounts", func(c *gin.Context) {
			c.Set(auth.ContextKeyBusinessID, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
			h.Broadband(c)
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/routers/cccccccc-cccc-4ccc-8ccc-cccccccccccc/broadband/accounts", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatal(w.Code, w.Body.String())
		}
		// A later check also answers 503 here, so the lock must be identified by its own code.
		if locked := strings.Contains(w.Body.String(), "broadband_writes_off"); locked != (tc.want == 503) {
			t.Fatalf("gate=%q broadband=%q: %s", tc.gate, tc.broadband, w.Body.String())
		}
	}
}
