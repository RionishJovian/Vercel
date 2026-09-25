package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestTenantConnMiddleware_noBusinessIDIsNoOp proves the middleware passes
// through (and never touches the pool) when there is no tenant in context — the
// flag-ON wiring must not affect non-tenant requests. A nil pool is safe here
// precisely because no business_id is present.
func TestTenantConnMiddleware_noBusinessIDIsNoOp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TenantConnMiddleware(nil))
	r.GET("/x", func(c *gin.Context) {
		if PinnedConn(c) != nil {
			t.Fatal("no pinned connection expected without a business id")
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (pass-through)", w.Code)
	}
}

// TestPinnedConn_nilWhenAbsent proves the accessor is nil on an unpinned request,
// which drives the handlers' fallback to the owner pool (flag OFF path).
func TestPinnedConn_nilWhenAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if PinnedConn(c) != nil {
		t.Fatal("expected nil pinned connection when none was set")
	}
}
