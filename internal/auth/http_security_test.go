package auth

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestCookieOriginBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, origin, header string
		want                 int
	}{
		{"same origin", "https://example.com", "1", 204},
		{"cross origin", "https://evil.example", "1", 403},
		{"missing origin", "", "1", 403},
		{"missing header", "https://example.com", "", 403},
		{"insecure origin", "http://example.com", "1", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.POST("/api/v1/auth/login", CookieOrigin(), func(c *gin.Context) { c.Status(204) })
			req := httptest.NewRequest("POST", "https://example.com/api/v1/auth/login", nil)
			req.Header.Set("Cookie", "mm_access=session")
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("X-MM-CSRF", tc.header)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("got %d want %d", w.Code, tc.want)
			}
		})
	}
}
func TestBoundedAuthenticationRate(t *testing.T) {
	r := gin.New()
	r.POST("/auth", BoundedAuthRateLimit(), func(c *gin.Context) { c.Status(204) })
	for i := 0; i < 31; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/auth", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		r.ServeHTTP(w, req)
		want := 204
		if i == 30 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("request %d: got %d", i, w.Code)
		}
	}
}

func TestNonBrowserCredentialRequest(t *testing.T) {
	r := gin.New()
	r.POST("/api/v1/auth/login", CookieOrigin(), func(c *gin.Context) { c.Status(204) })
	req := httptest.NewRequest("POST", "https://example.com/api/v1/auth/login", nil)
	req.Header.Set("X-MM-CSRF", "1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatalf("explicit API client rejected: %d", w.Code)
	}
}
