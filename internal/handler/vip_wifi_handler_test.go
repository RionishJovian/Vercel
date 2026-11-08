package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
)

func TestVIPStaffWiFiRequiresOwnerBeforeAnyRouterOrDBAccess(t *testing.T) {
	for _,tc:=range []struct{role string;want int}{
		{"VIEWER",403},
		{"ADMIN",403},
		{"",403},
		{"OWNER",503}, // accepted role, backend dependencies still fail closed
	}{
		t.Run(tc.role,func(t *testing.T){
			w:=httptest.NewRecorder()
			c,_:=gin.CreateTestContext(w)
			c.Request=httptest.NewRequest("POST","/vip-wifi",strings.NewReader(`{"request_id":"bad"}`))
			c.Request.Header.Set("Content-Type","application/json")
			c.Set(auth.ContextKeyBusinessID,"11111111-1111-4111-8111-111111111111")
			c.Set(auth.ContextKeyUserRole,tc.role)
			(&VIPWiFiHandler{}).Create(c)
			if w.Code!=tc.want{t.Fatalf("role=%q got=%d body=%s",tc.role,w.Code,w.Body.String())}
		})
	}
}
