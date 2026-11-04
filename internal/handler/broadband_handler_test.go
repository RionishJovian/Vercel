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
		gate, body string
		want       int
	}{{"", `{}`, 503}, {"true", `{"operation_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`, 400}} {
		t.Setenv("MM_NETWORK_CONTROLS_ENABLED", tc.gate)
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
	}
}
