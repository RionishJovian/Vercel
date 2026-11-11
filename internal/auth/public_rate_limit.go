package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const publicRateCookie = "__Secure-mm_rate"
const publicRateLifetime = time.Hour
const publicRateCapacity = 16384

type publicRateBucket struct {
	count int
	until time.Time
}

// PublicRequestLimits is installed once per API engine so changing a device ID
// or switching between public route groups cannot reset a browser's budget.
// The cookie proves only a server-issued random rate session, NEVER a guest,
// account, tenant, router attachment, payment or Internet entitlement.
// Cookie-less clients remain bounded by the aggregate source budget. Resetting
// cookies cannot evade that cap. Existing per-account failed-login locks remain.
// These process-local budgets match the current single API replica; scaling
// requires a shared limiter and new capacity evidence before rollout.
func PublicRequestLimits(secret string) gin.HandlerFunc {
	return publicRequestLimits(secret, time.Now)
}

func publicRequestLimits(secret string, now func() time.Time) gin.HandlerFunc {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("mega-magic:public-rate-session:v1"))
	key := mac.Sum(nil)
	var mu sync.Mutex
	// Source budgets and per-browser sessions live in separate bounded tables so
	// a flood of cookie-less requests (each mints a session) cannot evict or
	// refuse the aggregate source budget of legitimate visitors.
	ips := map[string]publicRateBucket{}
	sessions := map[string]publicRateBucket{}
	var nextPrune time.Time
	// take returns (allowed, retrySeconds, tableFull). tableFull means a NEW key
	// could not be tracked; callers decide whether that is fatal.
	take := func(table map[string]publicRateBucket, id string, limit int, t time.Time) (bool, int, bool) {
		b, known := table[id]
		if !t.Before(b.until) {
			b = publicRateBucket{until: t.Add(time.Minute)}
		}
		if !known && len(table) >= publicRateCapacity {
			return false, 1, true
		}
		if b.count >= limit {
			seconds := int((b.until.Sub(t) + time.Second - 1) / time.Second)
			if seconds > 60 {
				seconds = 60
			}
			if seconds < 1 {
				seconds = 1
			}
			return false, seconds, false
		}
		b.count++
		table[id] = b
		return true, 0, false
	}
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		scope := ""
		limit := 6000 // Aggregate requests/minute/source; allows 60 guests x 30 requests.
		switch {
		case strings.HasPrefix(path, "/api/v1/auth/"):
			if path == "/api/v1/auth/logout" {
				c.Next()
				return
			}
			scope = "auth"
			limit = 600
		case strings.HasPrefix(path, "/api/v1/portal/"), strings.HasPrefix(path, "/api/v1/media-guest/"), strings.HasPrefix(path, "/api/v1/media-playback/"):
			scope = "guest"
		default:
			c.Next()
			return
		}
		if len(secret) < 32 {
			c.AbortWithStatusJSON(503, gin.H{"error": "request protection unavailable"})
			return
		}
		t := now()
		token := ""
		cookies := 0
		for _, v := range c.Request.Cookies() {
			if v.Name == publicRateCookie {
				token = v.Value
				cookies++
			}
		}
		valid := cookies == 1 && validPublicRateToken(key, token, t)
		mu.Lock()
		if !t.Before(nextPrune) {
			for _, table := range []map[string]publicRateBucket{ips, sessions} {
				for id, b := range table {
					if !t.Before(b.until) {
						delete(table, id)
					}
				}
			}
			nextPrune = t.Add(time.Minute)
		}
		allowed, retry, _ := take(ips, "ip:"+scope+":"+sourceKey(c.ClientIP()), limit, t)
		// An absent/invalid cookie never selects an arbitrary client-supplied bucket.
		// Issuance and every rejected/accepted request still consume aggregate budget.
		if allowed && valid {
			var full bool
			allowed, retry, full = take(sessions, "session:"+scope+":"+token, 30, t)
			if full {
				// Session table saturated: degrade to the aggregate source budget
				// already charged above instead of refusing a legitimate browser.
				allowed = true
			}
		}
		mu.Unlock()
		c.Header("Cache-Control", "no-store")
		if !allowed {
			c.Header("Retry-After", strconv.Itoa(retry))
			c.AbortWithStatusJSON(429, gin.H{"error": "too many requests; try again shortly"})
			return
		}
		if !valid {
			nonce := make([]byte, 24)
			if _, err := rand.Read(nonce); err != nil {
				c.AbortWithStatusJSON(503, gin.H{"error": "request protection unavailable"})
				return
			}
			payload := base64.RawURLEncoding.EncodeToString(nonce) + "." + strconv.FormatInt(t.Unix(), 10)
			h := hmac.New(sha256.New, key)
			_, _ = h.Write([]byte(payload))
			token = payload + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil))
			// Count the bootstrap request in this browser's own allocation too.
			mu.Lock()
			allowed, retry, full := take(sessions, "session:"+scope+":"+token, 30, t)
			mu.Unlock()
			if !allowed && !full {
				c.Header("Retry-After", strconv.Itoa(retry))
				c.AbortWithStatusJSON(429, gin.H{"error": "too many requests; try again shortly"})
				return
			}
			if full {
				// Cannot track another browser right now: serve without issuing a
				// cookie. The aggregate source budget still applies.
				c.Next()
				return
			}
			http.SetCookie(c.Writer, &http.Cookie{Name: publicRateCookie, Value: token, Path: "/api/v1/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(publicRateLifetime / time.Second)})
		}
		c.Next()
	}
}

func validPublicRateToken(key []byte, token string, now time.Time) bool {
	if len(token) > 160 {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	nonce, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil || len(nonce) != 24 {
		return false
	}
	issued, e := strconv.ParseInt(parts[1], 10, 64)
	if e != nil || issued > now.Unix() || issued <= now.Add(-publicRateLifetime).Unix() {
		return false
	}
	sig, e := base64.RawURLEncoding.DecodeString(parts[2])
	if e != nil {
		return false
	}
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(parts[0] + "." + parts[1]))
	return hmac.Equal(sig, h.Sum(nil))
}

// sourceKey groups IPv6 clients by /64 (one customer LAN) so rotating addresses
// inside a prefix cannot mint unlimited independent source budgets.
func sourceKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	addr = addr.Unmap()
	if addr.Is6() {
		if p, err := addr.Prefix(64); err == nil {
			return p.String()
		}
	}
	return addr.String()
}
