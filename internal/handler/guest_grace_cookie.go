package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/model"
)

const guestGraceCookieName = "__Secure-mm_guest_grace"
const guestGraceCookieLifetime = 30 * 24 * time.Hour

type guestGraceCookie struct {
	Business string `json:"b"`
	Device   string `json:"d"`
	Request  string `json:"r"`
	Issued   int64  `json:"i"`
	Expires  int64  `json:"e"`
}

// Derive a separate-purpose signing key; cookies are not operator JWTs or
// verified guest identities. Configure only during server initialization.
func (h *GuestWifiHandler) EnableGuestCookies(secret string) error {
	if len(secret) < 32 {
		return errors.New("guest cookie signing key unavailable")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("mega-magic:guest-grace-cookie:v1"))
	h.graceCookieKey = mac.Sum(nil)
	return nil
}

func signGuestGraceCookie(key []byte, claim guestGraceCookie) string {
	raw, _ := json.Marshal(claim)
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func verifyGuestGraceCookie(key []byte, value, business, device string, now time.Time) (guestGraceCookie, error) {
	deny := errors.New("invalid guest recognition cookie")
	if len(key) != 32 || len(value) > 2048 {
		return guestGraceCookie{}, deny
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return guestGraceCookie{}, deny
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(parts[0]))
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return guestGraceCookie{}, deny
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	var claim guestGraceCookie
	if err != nil || json.Unmarshal(raw, &claim) != nil || claim.Business != business || claim.Device != device || claim.Request == "" || claim.Issued > now.Unix() || claim.Expires <= now.Unix() || claim.Expires <= claim.Issued || claim.Expires-claim.Issued > int64(guestGraceCookieLifetime/time.Second) {
		return guestGraceCookie{}, deny
	}
	return claim, nil
}

func (h *GuestWifiHandler) readGuestGraceCookie(c *gin.Context, business, device string, now time.Time) (*guestGraceCookie, error) {
	if len(h.graceCookieKey) != 32 {
		return nil, &httpError{status: http.StatusServiceUnavailable, msg: "guest recognition is unavailable"}
	}
	value, err := c.Cookie(guestGraceCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	claim, err := verifyGuestGraceCookie(h.graceCookieKey, value, business, device, now)
	if err != nil {
		return nil, &httpError{status: http.StatusForbidden, msg: "guest recognition could not be verified; contact the hotspot owner"}
	}
	return &claim, nil
}

func (h *GuestWifiHandler) writeGuestGraceCookie(c *gin.Context, business, device, request string, now time.Time) {
	claim := guestGraceCookie{Business: business, Device: device, Request: request, Issued: now.Unix(), Expires: now.Add(guestGraceCookieLifetime).Unix()}
	http.SetCookie(c.Writer, &http.Cookie{Name: guestGraceCookieName, Value: signGuestGraceCookie(h.graceCookieKey, claim), Path: "/api/v1/portal/" + device + "/", MaxAge: int(guestGraceCookieLifetime / time.Second), Expires: now.Add(guestGraceCookieLifetime), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

// A changed MAC must not mint new grace while the browser's original request is
// live or cooling down. Automatic transfer is intentionally not attempted here.
func checkGuestCookieRequest(previous *model.GuestPurchaseRequest, claim guestGraceCookie, clientMAC string, now time.Time) error {
	if previous == nil || previous.ID != claim.Request || previous.BusinessID != claim.Business || previous.DeviceID == nil || *previous.DeviceID != claim.Device {
		return &httpError{status: http.StatusForbidden, msg: "previous guest request could not be verified; contact the hotspot owner"}
	}
	resume, err := guestGraceRetry(previous, now)
	if err != nil {
		return err
	}
	if resume && previous.ClientMAC != clientMAC {
		return &httpError{status: http.StatusConflict, msg: "your existing payment window is still active on the previous connection; contact the hotspot owner to reconnect without resetting its allowance"}
	}
	return nil
}
