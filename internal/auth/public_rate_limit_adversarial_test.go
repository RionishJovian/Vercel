package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Independent adversarial review of the shared-NAT limiter (issue 192).

func adversarialEngine(now *time.Time, secret string) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(publicRequestLimits(secret, func() time.Time { return *now }))
	r.GET("/api/v1/portal/a/config", func(c *gin.Context) { c.Status(204) })
	r.POST("/api/v1/auth/login", func(c *gin.Context) { c.Status(204) })
	return r
}

func advRequest(r *gin.Engine, remote, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	method := "GET"
	if strings.Contains(path, "/auth/") {
		method = "POST"
	}
	q := httptest.NewRequest(method, path, nil)
	q.RemoteAddr = remote
	for _, c := range cookies {
		q.AddCookie(c)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, q)
	return w
}

func TestTamperedExpiredOrForeignCookiesNeverSelectAnExistingBucket(t *testing.T) {
	now := time.Unix(1800000000, 0)
	r := adversarialEngine(&now, rateTestSecret)
	other := adversarialEngine(&now, "another-disposable-secret-of-32-bytes!!")
	good := rateCookie(t, advRequest(r, "192.0.2.10:1", "/api/v1/portal/a/config"))
	foreign := rateCookie(t, advRequest(other, "192.0.2.10:1", "/api/v1/portal/a/config"))
	parts := strings.Split(good.Value, ".")
	flip := func(s string) string {
		b := []byte(s)
		if b[0] == 'A' {
			b[0] = 'B'
		} else {
			b[0] = 'A'
		}
		return string(b)
	}
	variants := map[string]string{
		"signature bit flipped": parts[0] + "." + parts[1] + "." + flip(parts[2]),
		"nonce swapped":         flip(parts[0]) + "." + parts[1] + "." + parts[2],
		"timestamp rewritten":   parts[0] + ".1800000001." + parts[2],
		"other deployment key":  foreign.Value,
		"oversize":              good.Value + strings.Repeat("A", 200),
		"two segments":          parts[0] + "." + parts[1],
		"empty":                 "",
	}
	for name, v := range variants {
		t.Run(name, func(t *testing.T) {
			w := advRequest(r, "192.0.2.10:1", "/api/v1/portal/a/config", &http.Cookie{Name: publicRateCookie, Value: v})
			if w.Code != 204 {
				t.Fatalf("aggregate budget should still serve: %d", w.Code)
			}
			issued := rateCookie(t, w) // an invalid cookie must be replaced, never honoured
			if issued.Value == v {
				t.Fatal("tampered cookie was accepted as-is")
			}
		})
	}
	// Spent budget of the genuine cookie cannot be reset by presenting a tampered copy.
	for i := 0; i < 29; i++ {
		advRequest(r, "192.0.2.10:1", "/api/v1/portal/a/config", good)
	}
	if w := advRequest(r, "192.0.2.10:1", "/api/v1/portal/a/config", good); w.Code != 429 {
		t.Fatalf("genuine cookie not limited: %d", w.Code)
	}
	// Expiry and future-dating.
	now = now.Add(publicRateLifetime + time.Second)
	if w := advRequest(r, "192.0.2.10:1", "/api/v1/portal/a/config", good); rateCookie(t, w).Value == good.Value {
		t.Fatal("expired cookie accepted")
	}
	now = time.Unix(1800000000, 0).Add(-time.Hour)
	if w := advRequest(r, "192.0.2.10:1", "/api/v1/portal/a/config", good); rateCookie(t, w).Value == good.Value {
		t.Fatal("future-dated cookie accepted")
	}
}

func TestDuplicateRateCookiesAreTreatedAsInvalid(t *testing.T) {
	now := time.Unix(1800000000, 0)
	r := adversarialEngine(&now, rateTestSecret)
	a := rateCookie(t, advRequest(r, "192.0.2.10:1", "/api/v1/portal/a/config"))
	b := rateCookie(t, advRequest(r, "192.0.2.11:1", "/api/v1/portal/a/config"))
	w := advRequest(r, "192.0.2.10:1", "/api/v1/portal/a/config", a, b)
	if rateCookie(t, w).Value == a.Value || rateCookie(t, w).Value == b.Value {
		t.Fatal("duplicate cookies must not select either bucket")
	}
}

func TestIPv6AddressesInOneSlash64CannotMultiplyTheSourceBudget(t *testing.T) {
	now := time.Unix(1800000000, 0)
	r := adversarialEngine(&now, rateTestSecret)
	// 600 auth requests/minute per source. Rotating the low 64 bits must not help.
	for i := 0; i < 600; i++ {
		remote := fmt.Sprintf("[2001:db8:7:9::%x]:1", i+1)
		if w := advRequest(r, remote, "/api/v1/auth/login"); w.Code != 204 {
			t.Fatalf("request %d: %d", i, w.Code)
		}
	}
	if w := advRequest(r, "[2001:db8:7:9:ffff::1]:1", "/api/v1/auth/login"); w.Code != 429 {
		t.Fatalf("rotating addresses inside one /64 evaded the cap: %d", w.Code)
	}
	// A different customer prefix, and an IPv4 customer, are unaffected.
	if w := advRequest(r, "[2001:db8:7:a::1]:1", "/api/v1/auth/login"); w.Code != 204 {
		t.Fatalf("neighbouring /64 wrongly blocked: %d", w.Code)
	}
	if w := advRequest(r, "198.51.100.4:1", "/api/v1/auth/login"); w.Code != 204 {
		t.Fatalf("IPv4 client wrongly blocked: %d", w.Code)
	}
}

func TestSaturatedTablesStillServeKnownSourcesAndRecover(t *testing.T) {
	now := time.Unix(1800000000, 0)
	r := adversarialEngine(&now, rateTestSecret)
	known := rateCookie(t, advRequest(r, "203.0.113.5:1", "/api/v1/portal/a/config"))
	// Fill the session table from three sources (each capped at 6000/min).
	for src := 1; src <= 3; src++ {
		for i := 0; i < 6000; i++ {
			advRequest(r, fmt.Sprintf("198.51.100.%d:1", src), "/api/v1/portal/a/config")
		}
	}
	// An existing browser with a valid cookie keeps working.
	if w := advRequest(r, "203.0.113.5:1", "/api/v1/portal/a/config", known); w.Code != 204 {
		t.Fatalf("known browser refused during saturation: %d", w.Code)
	}
	// New visitors are served without a cookie (aggregate budget only).
	w := advRequest(r, "203.0.113.77:1", "/api/v1/portal/a/config")
	if w.Code != 204 {
		t.Fatalf("new visitor refused during saturation: %d", w.Code)
	}
	// After the window everything is reclaimed and cookies are issued again.
	now = now.Add(2 * time.Minute)
	w = advRequest(r, "203.0.113.78:1", "/api/v1/portal/a/config")
	if w.Code != 204 || len(w.Result().Cookies()) == 0 {
		t.Fatalf("no recovery after window: %d cookies=%d", w.Code, len(w.Result().Cookies()))
	}
}

// Owner console routes share the public portal URL prefix but must not spend the
// anonymous guest budget, while keeping their own authentication.
func ownerScopeEngine(now *time.Time) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(publicRequestLimits(rateTestSecret, func() time.Time { return *now }))
	requireToken := func(c *gin.Context) { // stands in for auth.Middleware on the protected group
		if c.GetHeader("Authorization") != "Bearer owner" {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
	owner := r.Group("/api/v1/portal", requireToken)
	owner.GET("/config", func(c *gin.Context) { c.Status(204) })
	owner.PUT("/config", func(c *gin.Context) { c.Status(204) })
	owner.POST("/router-preview", func(c *gin.Context) { c.Status(204) })
	r.GET("/api/v1/portal/:deviceId/config", func(c *gin.Context) { c.Status(204) })
	return r
}

func ownerRequest(r *gin.Engine, method, path string, authed bool, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	q := httptest.NewRequest(method, path, nil)
	q.RemoteAddr = "192.0.2.10:1"
	if authed {
		q.Header.Set("Authorization", "Bearer owner")
	}
	for _, c := range cookies {
		q.AddCookie(c)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, q)
	return w
}

func TestOwnerPortalRoutesDoNotSpendTheGuestBudgetOrIssueCookies(t *testing.T) {
	now := time.Unix(1800000000, 0)
	r := ownerScopeEngine(&now)
	guest := rateCookie(t, ownerRequest(r, "GET", "/api/v1/portal/device-1/config", false))
	for i := 0; i < 100; i++ {
		for _, route := range [][2]string{{"GET", "/api/v1/portal/config"}, {"PUT", "/api/v1/portal/config"}, {"POST", "/api/v1/portal/router-preview"}} {
			w := ownerRequest(r, route[0], route[1], true, guest)
			if w.Code != 204 {
				t.Fatalf("owner %s %s request %d: %d", route[0], route[1], i, w.Code)
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatalf("owner route issued a guest rate cookie")
			}
		}
	}
	// The guest browser's own budget (29 more after the bootstrap request) is untouched.
	for i := 0; i < 29; i++ {
		if w := ownerRequest(r, "GET", "/api/v1/portal/device-1/config", false, guest); w.Code != 204 {
			t.Fatalf("guest request %d after owner activity: %d", i, w.Code)
		}
	}
	if w := ownerRequest(r, "GET", "/api/v1/portal/device-1/config", false, guest); w.Code != 429 {
		t.Fatalf("guest budget no longer enforced: %d", w.Code)
	}
}

func TestOwnerPortalRoutesKeepTheirAuthentication(t *testing.T) {
	now := time.Unix(1800000000, 0)
	r := ownerScopeEngine(&now)
	for _, route := range [][2]string{{"GET", "/api/v1/portal/config"}, {"PUT", "/api/v1/portal/config"}, {"POST", "/api/v1/portal/router-preview"}} {
		if w := ownerRequest(r, route[0], route[1], false); w.Code != 401 {
			t.Fatalf("unauthenticated %s %s reached the handler: %d", route[0], route[1], w.Code)
		}
	}
}

func TestUnanonymousRoutesAnd404sCannotDrainOrMintGuestState(t *testing.T) {
	now := time.Unix(1800000000, 0)
	r := ownerScopeEngine(&now)
	for i := 0; i < 7000; i++ { // well past the 6,000/min guest aggregate
		w := ownerRequest(r, "GET", fmt.Sprintf("/api/v1/portal/unknown/%d/nothing", i), false)
		if w.Code != 404 || len(w.Result().Cookies()) != 0 {
			t.Fatalf("404 request %d: %d cookies=%d", i, w.Code, len(w.Result().Cookies()))
		}
	}
	if w := ownerRequest(r, "GET", "/api/v1/portal/device-1/config", false); w.Code != 204 {
		t.Fatalf("a flood of 404s spent the guest aggregate budget: %d", w.Code)
	}
}
