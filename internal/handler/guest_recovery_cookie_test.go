package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

func TestGuestRecoveryCookieIsPrivateDeviceScopedAndExpiryBound(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(time.Minute)
	secret, err := guestaccess.RecoverySecret()
	if err != nil {
		t.Fatal(err)
	}
	g := model.GuestVoucherGrant{DeviceID: "11111111-1111-4111-8111-111111111111", VoucherID: "22222222-2222-4222-8222-222222222222", State: "active", ExpiresAt: &expires, RecoverySecret: secret}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	if !setGuestRecoveryCookie(c, g, now) {
		t.Fatal("valid credential rejected")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("missing cookie")
	}
	ck := cookies[0]
	if ck.Name != guestRecoveryCookieName || ck.Value != g.VoucherID+"."+secret || !ck.Secure || !ck.HttpOnly || ck.Domain != "" || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/api/v1/portal/"+g.DeviceID+"/" || !ck.Expires.Equal(expires) || ck.MaxAge != 0 {
		t.Fatal("unsafe recovery cookie")
	}
	b, err := json.Marshal(g)
	if err != nil || strings.Contains(string(b), secret) || strings.Contains(string(b), "RecoverySecret") {
		t.Fatal("credential escaped JSON boundary")
	}
	// Code retry must not overwrite a paid cookie or start a new lifetime.
	g.RecoverySecret = ""
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	if !setGuestRecoveryCookie(c, g, now) || len(w.Result().Cookies()) != 0 {
		t.Fatal("retry replaced cookie")
	}
}

func TestGuestRecoveryCookieRejectsInvalidAuthority(t *testing.T) {
	now := time.Now()
	expires := now.Add(time.Minute)
	secret, _ := guestaccess.RecoverySecret()
	base := model.GuestVoucherGrant{DeviceID: "11111111-1111-4111-8111-111111111111", VoucherID: "22222222-2222-4222-8222-222222222222", State: "active", ExpiresAt: &expires, RecoverySecret: secret}
	for _, mutate := range []func(*model.GuestVoucherGrant){
		func(g *model.GuestVoucherGrant) { g.DeviceID = "../other" },
		func(g *model.GuestVoucherGrant) { g.VoucherID = "bad" },
		func(g *model.GuestVoucherGrant) { g.RecoverySecret = "voucher-code" },
		func(g *model.GuestVoucherGrant) { g.State = "provisioning" },
		func(g *model.GuestVoucherGrant) { g.ExpiresAt = nil },
		func(g *model.GuestVoucherGrant) { g.ExpiresAt = &now },
	} {
		g := base
		mutate(&g)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		if setGuestRecoveryCookie(c, g, now) || len(w.Result().Cookies()) != 0 {
			t.Fatal("invalid credential set cookie")
		}
	}
}
