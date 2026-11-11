package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

const rateTestSecret = "disposable-test-key-at-least-32-bytes"

func rateEngine(secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(PublicRequestLimits(secret))
	for _, p := range []string{"/api/v1/portal/:device/config", "/api/v1/portal/:device/access", "/api/v1/media-guest/:id"} {
		r.GET(p, func(c *gin.Context) { c.Status(204) })
	}
	r.POST("/api/v1/auth/login", func(c *gin.Context) { c.Status(204) })
	return r
}
func rateRequest(r *gin.Engine, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	q := httptest.NewRequest("GET", path, nil)
	if path == "/api/v1/auth/login" {
		q.Method = "POST"
	}
	q.RemoteAddr = "192.0.2.10:1234"
	q.Header.Set("X-Forwarded-For", "203.0.113.99")
	if cookie != nil {
		q.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, q)
	return w
}
func rateCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == publicRateCookie {
			return c
		}
	}
	t.Fatal("missing signed rate cookie")
	return nil
}
func TestPublicRateLimitsIsolateSixtyBrowsersBehindOneNAT(t *testing.T) {
	for _, path := range []string{"/api/v1/portal/router/config", "/api/v1/auth/login"} {
		t.Run(path, func(t *testing.T) {
			r := rateEngine(rateTestSecret)
			cookies := make([]*http.Cookie, 60)
			for i := range cookies {
				w := rateRequest(r, path, nil)
				if w.Code != 204 {
					t.Fatalf("browser %d bootstrap: %d", i, w.Code)
				}
				cookies[i] = rateCookie(t, w)
			}
			for i, c := range cookies {
				for n := 0; n < 3; n++ {
					if w := rateRequest(r, path, c); w.Code != 204 {
						t.Fatalf("browser %d request %d: %d", i, n, w.Code)
					}
				}
			}
			for n := 0; n < 27; n++ {
				rateRequest(r, path, cookies[0])
			}
			w := rateRequest(r, path, cookies[0])
			if w.Code != 429 || w.Header().Get("Retry-After") == "" {
				t.Fatalf("abusive browser not limited: %d", w.Code)
			}
			if w := rateRequest(r, path, cookies[1]); w.Code != 204 {
				t.Fatalf("other NAT customer blocked: %d", w.Code)
			}
		})
	}
}
func TestPublicRateLimitsSharePortalAndMediaBudget(t *testing.T) {
	r := rateEngine(rateTestSecret)
	cookie := rateCookie(t, rateRequest(r, "/api/v1/portal/a/config", nil))
	for i := 0; i < 29; i++ {
		if w := rateRequest(r, fmt.Sprintf("/api/v1/portal/device-%d/access", i), cookie); w.Code != 204 {
			t.Fatal(w.Code)
		}
	}
	if w := rateRequest(r, "/api/v1/media-guest/x", cookie); w.Code != 429 {
		t.Fatalf("changing device or route evaded limit: %d", w.Code)
	}
}
func TestPublicRateCookieSurvivesRestartAndCannotBeForged(t *testing.T) {
	first := rateEngine(rateTestSecret)
	cookie := rateCookie(t, rateRequest(first, "/api/v1/portal/a/config", nil))
	if !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/api/v1/" || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unsafe cookie attributes: %+v", cookie)
	}
	second := rateEngine(rateTestSecret)
	w := rateRequest(second, "/api/v1/portal/a/config", cookie)
	if w.Code != 204 || len(w.Result().Cookies()) != 0 {
		t.Fatal("valid cookie was replaced after restart")
	}
	forged := *cookie
	forged.Value += "tampered"
	w = rateRequest(second, "/api/v1/portal/a/config", &forged)
	if got := rateCookie(t, w).Value; got == forged.Value {
		t.Fatal("forged cookie accepted")
	}
}
func TestPublicRateLimitsRetainAggregateCapWithoutCookies(t *testing.T) {
	r := rateEngine(rateTestSecret)
	for i := 0; i < 600; i++ {
		if w := rateRequest(r, "/api/v1/auth/login", nil); w.Code != 204 {
			t.Fatalf("request %d: %d", i, w.Code)
		}
	}
	if w := rateRequest(r, "/api/v1/auth/login", nil); w.Code != 429 {
		t.Fatalf("cookieless auth evaded aggregate cap: %d", w.Code)
	}
}
func TestPublicRateLimitsFailClosedWithoutSigningKey(t *testing.T) {
	r := rateEngine("")
	if w := rateRequest(r, "/api/v1/portal/a/config", nil); w.Code != 503 {
		t.Fatalf("missing key accepted: %d", w.Code)
	}
}

func TestPublicRateWindowAndCookieExpiry(t *testing.T) {
	now := time.Unix(1800000000, 0)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(publicRequestLimits(rateTestSecret, func() time.Time { return now }))
	r.GET("/api/v1/portal/a/config", func(c *gin.Context) { c.Status(204) })
	cookie := rateCookie(t, rateRequest(r, "/api/v1/portal/a/config", nil))
	for i := 0; i < 29; i++ {
		rateRequest(r, "/api/v1/portal/a/config", cookie)
	}
	now = now.Add(45 * time.Second)
	if w := rateRequest(r, "/api/v1/portal/a/config", cookie); w.Code != 429 || w.Header().Get("Retry-After") != "15" {
		t.Fatalf("retry window: %d %s", w.Code, w.Header().Get("Retry-After"))
	}
	now = now.Add(16 * time.Second)
	if w := rateRequest(r, "/api/v1/portal/a/config", cookie); w.Code != 204 {
		t.Fatal("window failed to recover")
	}
	now = now.Add(publicRateLifetime)
	if w := rateRequest(r, "/api/v1/portal/a/config", cookie); len(w.Result().Cookies()) != 1 {
		t.Fatal("expired cookie accepted")
	}
}
func TestPublicRateSixtyConcurrentBrowsers(t *testing.T) {
	r := rateEngine(rateTestSecret)
	var wg sync.WaitGroup
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := rateRequest(r, "/api/v1/portal/a/config", nil)
			if w.Code != 204 {
				t.Errorf("bootstrap: %d", w.Code)
				return
			}
			cookie := rateCookie(t, w)
			for j := 0; j < 20; j++ {
				if w := rateRequest(r, "/api/v1/portal/a/access", cookie); w.Code != 204 {
					t.Errorf("NAT guest blocked: %d", w.Code)
					return
				}
			}
		}()
	}
	wg.Wait()
}
func TestPublicRateDuplicateCookieAndHeaderSpoofingCannotEvadeCap(t *testing.T) {
	r := rateEngine(rateTestSecret)
	for i := 0; i < 600; i++ {
		q := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		q.RemoteAddr = "192.0.2.10:1000"
		q.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i%250+1))
		q.AddCookie(&http.Cookie{Name: publicRateCookie, Value: fmt.Sprint(i)})
		q.AddCookie(&http.Cookie{Name: publicRateCookie, Value: "shadow"})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		if w.Code != 204 {
			t.Fatalf("request %d: %d", i, w.Code)
		}
	}
	if w := rateRequest(r, "/api/v1/auth/login", nil); w.Code != 429 {
		t.Fatal("spoofing evaded aggregate")
	}
}

func TestPublicRateStateIsBoundedAndRecoversAfterWindow(t *testing.T) {
	now := time.Unix(1800000000, 0)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(publicRequestLimits(rateTestSecret, func() time.Time { return now }))
	r.GET("/api/v1/portal/a/config", func(c *gin.Context) { c.Status(204) })
	blocked := false
	for i := 0; i < publicRateCapacity; i++ {
		q := httptest.NewRequest("GET", "/api/v1/portal/a/config", nil)
		q.RemoteAddr = fmt.Sprintf("198.18.%d.%d:1234", i/256, i%256)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		if w.Code == 429 {
			blocked = true
			break
		}
		if w.Code != 204 {
			t.Fatal(w.Code)
		}
	}
	if !blocked {
		t.Fatal("unbounded state accepted")
	}
	now = now.Add(time.Minute)
	if w := rateRequest(r, "/api/v1/portal/a/config", nil); w.Code != 204 {
		t.Fatal("expired state was not reclaimed")
	}
}
