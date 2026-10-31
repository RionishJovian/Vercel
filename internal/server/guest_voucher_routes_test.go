package server

import (
	"context"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/config"
	"mega-magic/internal/guestaccess"
	"mega-magic/internal/handler"
)

type guestRecoveryRouteStub struct{}

func (guestRecoveryRouteStub) Resume(context.Context, guestaccess.RecoveryRequest) (guestaccess.ResumeResult, error) {
	return guestaccess.ResumeResult{}, guestaccess.ErrRecoveryDenied
}

func (guestRecoveryRouteStub) Restore(context.Context, guestaccess.RecoveryRequest) (guestaccess.RecoveryClaim, error) {
	return guestaccess.RecoveryClaim{}, guestaccess.ErrRecoveryDenied
}

// Guests redeem vouchers on the RouterBOARD login page. The cloud app must not
// expose any public guest endpoint, even when recovery handlers are wired.
func TestNoPublicGuestVoucherRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerGuestVoucherEndpoints(r, nil, nil, &handler.GuestVoucherHandler{Recoverer: guestRecoveryRouteStub{}, Resumer: guestRecoveryRouteStub{}})
	owner := 0
	for _, route := range r.Routes() {
		if strings.Contains(route.Path, "/portal/") {
			t.Fatalf("public guest route exposed: %s %s", route.Method, route.Path)
		}
		if strings.HasPrefix(route.Path, "/api/v1/guest-vouchers") {
			owner++
		}
	}
	if owner == 0 {
		t.Fatal("owner-side voucher routes must still be registered")
	}
}

func TestGuestVoucherRoutesFailClosed(t *testing.T) {
	r := gin.New()
	cfg := &config.Config{}
	if e := registerGuestVoucherRoutes(r, cfg, nil, nil, nil); e != nil || len(r.Routes()) != 0 {
		t.Fatal("disabled feature registered routes", e)
	}
	cfg.GuestWifi.VoucherAccessEnabled = true
	if e := registerGuestVoucherRoutes(r, cfg, nil, nil, nil); e == nil || len(r.Routes()) != 0 {
		t.Fatal("enabled without applier did not fail closed")
	}
}
