package handler

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/model"
)

func cookieFixture(t *testing.T) (*GuestWifiHandler, guestGraceCookie, time.Time) {
	t.Helper()
	h := &GuestWifiHandler{}
	if err := h.EnableGuestCookies(strings.Repeat("k", 32)); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	return h, guestGraceCookie{Business: "business", Device: "device", Request: "request", Issued: now.Unix(), Expires: now.Add(guestGraceCookieLifetime).Unix()}, now
}

func TestGuestCookieSigningScopeAndExpiry(t *testing.T) {
	h, claim, now := cookieFixture(t)
	value := signGuestGraceCookie(h.graceCookieKey, claim)
	got, err := verifyGuestGraceCookie(h.graceCookieKey, value, claim.Business, claim.Device, now)
	if err != nil || got != claim {
		t.Fatalf("valid cookie rejected: %v", err)
	}
	for _, tc := range []struct {
		name, value, business, device string
		now                           time.Time
	}{
		{"tampered", value + "x", claim.Business, claim.Device, now},
		{"unsigned", strings.Split(value, ".")[0], claim.Business, claim.Device, now},
		{"wrong tenant", value, "another-business", claim.Device, now},
		{"wrong gateway", value, claim.Business, "another-device", now},
		{"expired", value, claim.Business, claim.Device, time.Unix(claim.Expires, 0)},
		{"future issue", value, claim.Business, claim.Device, now.Add(-time.Second)},
		{"oversized", strings.Repeat("x", 2049), claim.Business, claim.Device, now},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := verifyGuestGraceCookie(h.graceCookieKey, tc.value, tc.business, tc.device, tc.now); err == nil {
				t.Fatal("invalid recognition accepted")
			}
		})
	}
	other := &GuestWifiHandler{}
	_ = other.EnableGuestCookies(strings.Repeat("z", 32))
	if _, err := verifyGuestGraceCookie(other.graceCookieKey, value, claim.Business, claim.Device, now); err == nil {
		t.Fatal("wrong signing key accepted")
	}
}

func TestGuestCookieHeadersAndMissingOrInvalidCookie(t *testing.T) {
	h, claim, now := cookieFixture(t)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "https://portal.example/api/v1/portal/device/guest-purchase", nil)
	if got, err := h.readGuestGraceCookie(c, claim.Business, claim.Device, now); got != nil || err != nil {
		t.Fatal("new browser denied")
	}
	h.writeGuestGraceCookie(c, claim.Business, claim.Device, claim.Request, now)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("cookie missing")
	}
	cookie := cookies[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Domain != "" || cookie.Path != "/api/v1/portal/device/" || cookie.MaxAge != int(guestGraceCookieLifetime/time.Second) {
		t.Fatal("unsafe cookie attributes")
	}
	c.Request.AddCookie(cookie)
	if got, err := h.readGuestGraceCookie(c, claim.Business, claim.Device, now); err != nil || got == nil || got.Request != claim.Request {
		t.Fatal("cookie cannot round trip")
	}
	c.Request.Header.Set("Cookie", guestGraceCookieName+"=forged")
	if _, err := h.readGuestGraceCookie(c, claim.Business, claim.Device, now); err == nil {
		t.Fatal("forged cookie fell back to new grace")
	}
}

func TestGuestCookiePreservesCooldownAcrossMACChange(t *testing.T) {
	_, claim, now := cookieFixture(t)
	end := now.Add(time.Minute)
	previous := &model.GuestPurchaseRequest{ID: claim.Request, BusinessID: claim.Business, DeviceID: &claim.Device, ClientMAC: "AA:BB:CC:DD:EE:FE", Status: "pending", GraceStartedAt: &now, GraceExpiresAt: &end, GraceDataMB: 50}
	original := *previous
	for _, tc := range []struct {
		name, mac string
		at        time.Time
		blocked   bool
	}{
		{"same MAC resumes", previous.ClientMAC, now, false},
		{"new MAC live window", "AA:BB:CC:DD:EE:FC", now, true},
		{"new MAC expired", "AA:BB:CC:DD:EE:FC", end, true},
		{"new MAC during six hours", "AA:BB:CC:DD:EE:FC", end.Add(6*time.Hour - time.Second), true},
		{"retry after six hours", "AA:BB:CC:DD:EE:FC", end.Add(6 * time.Hour), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkGuestCookieRequest(previous, claim, tc.mac, tc.at)
			if (err != nil) != tc.blocked {
				t.Fatalf("unexpected policy: %v", err)
			}
		})
	}
	if !reflect.DeepEqual(original, *previous) {
		t.Fatal("recognition modified grace quota or expiry")
	}
	previous.Status = "approved"
	if err := checkGuestCookieRequest(previous, claim, "AA:BB:CC:DD:EE:FC", end.Add(7*time.Hour)); err == nil {
		t.Fatal("paid request granted free grace")
	}
	if err := checkGuestCookieRequest(nil, claim, previous.ClientMAC, now); err == nil {
		t.Fatal("deleted request treated as new guest")
	}
}
