package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

const guestRecoveryCookieName = "__Secure-mm_guest_recovery"

// Duplicate cookies are ambiguous (for example, a shadowing parent-path cookie).
// Never select a convenient one or fall back to guest-supplied JSON credentials.
func readGuestRecoveryCookie(r *http.Request) (voucherID, secret string, ok bool) {
	count := 0
	var value string
	for _, cookie := range r.Cookies() {
		if cookie.Name == guestRecoveryCookieName {
			count++
			value = cookie.Value
		}
	}
	if count != 1 {
		return "", "", false
	}
	id, proof, found := strings.Cut(value, ".")
	if !found || !guestVoucherUUID.MatchString(id) {
		return "", "", false
	}
	if _, err := guestaccess.HashSecret(proof); err != nil {
		return "", "", false
	}
	return id, proof, true
}

// An empty secret is an ordinary retry: preserve the existing cookie. The
// voucher ID is a lookup hint only; the random secret is the ownership proof.
// This is deliberately separate from grace recognition and grants no free time.
func setGuestRecoveryCookie(c *gin.Context, g model.GuestVoucherGrant, now time.Time) bool {
	if g.RecoverySecret == "" {
		return true
	}
	if !guestVoucherUUID.MatchString(g.DeviceID) || !guestVoucherUUID.MatchString(g.VoucherID) ||
		g.ExpiresAt == nil || !now.Before(*g.ExpiresAt) || g.State != "active" {
		return false
	}
	if _, err := guestaccess.HashSecret(g.RecoverySecret); err != nil {
		return false
	}
	// No Domain attribute: host-only, HTTPS-only and inaccessible to JavaScript.
	// The lifetime is bounded by the original paid expiry, not cookie issuance.
	http.SetCookie(c.Writer, &http.Cookie{
		Name: guestRecoveryCookieName, Value: g.VoucherID + "." + g.RecoverySecret,
		Path: "/api/v1/portal/" + g.DeviceID + "/", Secure: true, HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Expires: g.ExpiresAt.UTC(),
	})
	return true
}
