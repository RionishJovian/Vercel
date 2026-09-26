package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRequireRole_customerOwnerBlocked proves the DEVELOPER-platform gate: a
// customer OWNER role is REJECTED (403), because the /api/v1/platform routes
// must never be reachable by a tenant account.
func TestRequireRole_customerOwnerBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextKeyUserRole, "OWNER")
		c.Next()
	})
	r.GET("/platform", RequireRole("DEVELOPER"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/platform", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("OWNER on DEVELOPER route = %d, want 403 (customer must be blocked)", w.Code)
	}
}

// TestRequireRole_viewerBlocked too — any customer role is 403.
func TestRequireRole_viewerBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextKeyUserRole, "VIEWER")
		c.Next()
	})
	r.GET("/platform", RequireRole("DEVELOPER"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/platform", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("VIEWER on DEVELOPER route = %d, want 403", w.Code)
	}
}

// TestRequireRole_developerAdmitted proves a DEVELOPER role passes the gate.
func TestRequireRole_developerAdmitted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(ContextKeyUserRole, "DEVELOPER")
		c.Next()
	})
	r.GET("/platform", RequireRole("DEVELOPER"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/platform", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("DEVELOPER on DEVELOPER route = %d, want 200", w.Code)
	}
}

// TestRequireRole_noRoleBlocked proves a request with no role context is 403
// (never defaults open).
func TestRequireRole_noRoleBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/platform", RequireRole("DEVELOPER"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/platform", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("no-role on DEVELOPER route = %d, want 403 (never defaults open)", w.Code)
	}
}
