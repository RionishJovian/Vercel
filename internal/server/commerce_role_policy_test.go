package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/model"
)

// Inspect the actual production route declarations, then execute their named
// middleware. A standalone middleware test alone would not catch an omitted
// guard on the real route, which was the original defect.
func TestCommerceMutationRoutesHaveRoleGuards(t *testing.T) {
	want := map[string]model.UserRole{
		"CreatePolicy": model.RoleAdmin, "UpdatePolicy": model.RoleAdmin, "DeletePolicy": model.RoleAdmin,
		"CreateVoucher": model.RoleAdmin, "UpdateVoucherStatus": model.RoleAdmin,
		"DeleteVoucher": model.RoleAdmin, "StaffUpdateConfig": model.RoleAdmin,
		"PortalDeployment": model.RoleAdmin,
		"NetworkControl":   model.RoleAdmin,
		"ApproveGuest":     model.RoleAdmin, "RejectGuest": model.RoleAdmin,
		"Acknowledge": model.RoleAdmin, "Resolve": model.RoleAdmin,
		"Reboot":       model.RoleAdmin,
		"CreateBackup": model.RoleAdmin, "DeleteBackup": model.RoleAdmin, "Restore": model.RoleAdmin,
	}
	f, err := parser.ParseFile(token.NewFileSet(), "server.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		handler, ok := call.Args[len(call.Args)-1].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		min, ok := want[handler.Sel.Name]
		if !ok {
			return true
		}
		guard, ok := call.Args[len(call.Args)-2].(*ast.CallExpr)
		if !ok || len(guard.Args) != 1 {
			t.Errorf("%s missing explicit guard", handler.Sel.Name)
			return true
		}
		selector, ok := guard.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "RequireMinRole" {
			t.Errorf("wrong guard for %s", handler.Sel.Name)
			return true
		}
		role, ok := guard.Args[0].(*ast.SelectorExpr)
		if !ok {
			t.Fatal("invalid role")
		}
		expected := "RoleAdmin"
		if min == model.RoleAdmin {
			expected = "RoleAdmin"
		}
		if role.Sel.Name != expected {
			t.Errorf("%s requires %s, want %s", handler.Sel.Name, role.Sel.Name, expected)
		}
		found[handler.Sel.Name] = true
		return true
	})
	gin.SetMode(gin.TestMode)
	for name, min := range want {
		if !found[name] {
			t.Fatalf("production route not checked: %s", name)
		}
		for _, role := range []string{"VIEWER", "ADMIN", "OWNER", "DEVELOPER", "unknown"} {
			r := gin.New()
			called := false
			r.POST("/mutation", func(c *gin.Context) { c.Set(auth.ContextKeyUserRole, role) }, auth.RequireMinRole(min), func(c *gin.Context) { called = true; c.Status(http.StatusOK) })
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/mutation", nil))
			allowed := role == "ADMIN" || role == "OWNER" || role == "DEVELOPER"
			if called != allowed || (!allowed && w.Code != http.StatusForbidden) {
				t.Fatalf("%s role=%s code=%d handler=%v", name, role, w.Code, called)
			}
		}
	}
}
