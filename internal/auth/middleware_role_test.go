package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/model"
)

func TestRequireMinRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		role string
		min  model.UserRole
		want int
	}{
		{"OWNER", model.RoleOwner, http.StatusOK},
		{"OWNER", model.RoleAdmin, http.StatusOK},
		{"ADMIN", model.RoleOwner, http.StatusForbidden},
		{"ADMIN", model.RoleAdmin, http.StatusOK},
		{"VIEWER", model.RoleViewer, http.StatusOK},
		{"DEVELOPER", model.RoleOwner, http.StatusOK},
	}
	for _, c := range cases {
		r := gin.New()
		r.GET("/x",
			func(ctx *gin.Context) { ctx.Set(ContextKeyUserRole, c.role) },
			RequireMinRole(c.min),
			func(ctx *gin.Context) { ctx.Status(http.StatusOK) })

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.ServeHTTP(w, req)
		if w.Code != c.want {
			t.Fatalf("role %s vs min %s = %d, want %d", c.role, c.min, w.Code, c.want)
		}
	}
}

func TestRequireMinRole_missingRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", RequireMinRole(model.RoleViewer), func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing role = %d, want 403", w.Code)
	}
}
