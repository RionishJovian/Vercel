package handler

import (
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthenticationCookies(t *testing.T) {
	svc, _ := auth.NewService("test-cookie-secret-at-least-32-characters", time.Hour, 24*time.Hour, 4)
	h := NewAuthHandler(nil, svc, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.setSessionCookies(c, "access", "refresh")
	cookies := w.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("got %d cookies", len(cookies))
	}
	for _, cookie := range cookies {
		if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
			t.Fatal("unsafe session cookie flags")
		}
		want := "/api/v1"
		if cookie.Name == "mm_refresh" {
			want = "/api/v1/auth"
		}
		if cookie.Path != want || cookie.MaxAge <= 0 {
			t.Fatal("incorrect cookie scope or expiry")
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("session response cacheable")
	}
}
