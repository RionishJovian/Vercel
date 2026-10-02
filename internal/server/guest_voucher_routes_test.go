package server

import (
	"context"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/config"
	"mega-magic/internal/guestaccess"
	"mega-magic/internal/handler"
	"net/http/httptest"
	"strings"
	"testing"
)

type guestRecoveryRouteStub struct{}

func (guestRecoveryRouteStub) Resume(context.Context, guestaccess.RecoveryRequest) (guestaccess.ResumeResult, error) {
	return guestaccess.ResumeResult{}, guestaccess.ErrRecoveryDenied
}

func (guestRecoveryRouteStub) Restore(context.Context, guestaccess.RecoveryRequest) (guestaccess.RecoveryClaim, error) {
	return guestaccess.RecoveryClaim{}, guestaccess.ErrRecoveryDenied
}

func TestGuestRecoveryRouteRequiresExplicitSameOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"guest-recovery", "guest-resume"} {
		for _, tc := range []struct {
			origin, csrf, authorization string
			status                      int
		}{
			{"https://app.test", "1", "", 400}, // reaches body validation
			{"", "1", "", 403},
			{"https://evil.test", "1", "", 403},
			{"https://app.test", "", "", 403},
			{"http://app.test", "1", "", 403},
			{"https://app.test/path", "1", "", 403},
			{"https://app.test", "", "Bearer not-a-cookie", 403},
			{"", "1", "Bearer not-a-cookie", 403},
		} {
			r := gin.New()
			registerGuestVoucherEndpoints(r, nil, nil, &handler.GuestVoucherHandler{Recoverer: guestRecoveryRouteStub{}, Resumer: guestRecoveryRouteStub{}})
			q := httptest.NewRequest("POST", "https://app.test/api/v1/portal/11111111-1111-4111-8111-111111111111/"+endpoint, strings.NewReader(`{`))
			q.Header.Set("Origin", tc.origin)
			q.Header.Set("X-MM-CSRF", tc.csrf)
			q.Header.Set("Authorization", tc.authorization)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, q)
			if w.Code != tc.status {
				t.Fatalf("origin=%q csrf=%q status=%d", tc.origin, tc.csrf, w.Code)
			}
		}
	}
	r := gin.New()
	registerGuestVoucherEndpoints(r, nil, nil, &handler.GuestVoucherHandler{})
	for _, route := range r.Routes() {
		if strings.HasSuffix(route.Path, "/guest-recovery") || strings.HasSuffix(route.Path, "/guest-resume") {
			t.Fatal("unwired recovery endpoint exposed")
		}
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
