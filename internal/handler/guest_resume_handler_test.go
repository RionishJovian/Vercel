package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

type resumeHandlerStub struct {
	calls  int
	req    guestaccess.RecoveryRequest
	result guestaccess.ResumeResult
	err    error
}

func (s *resumeHandlerStub) Resume(_ context.Context, req guestaccess.RecoveryRequest) (guestaccess.ResumeResult, error) {
	s.calls++
	s.req = req
	return s.result, s.err
}

func TestGuestResumeHTTPBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const device = "11111111-1111-4111-8111-111111111111"
	const voucher = "22222222-2222-4222-8222-222222222222"
	secret, err := guestaccess.RecoverySecret()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                                                      string
		status, calls                                             int
		missing, duplicate, disabled, noConsent, foreign, unwired bool
		err                                                       error
		bytes                                                     int64
		expired                                                   bool
	}{
		{name: "verified", status: 200, calls: 1, bytes: 25000000},
		{name: "missing proof", missing: true, status: 403},
		{name: "duplicate proof", duplicate: true, status: 403},
		{name: "disabled portal", disabled: true, status: 403},
		{name: "terms", noConsent: true, status: 400},
		{name: "foreign gateway", foreign: true, status: 404},
		{name: "unwired", unwired: true, status: 503},
		{name: "denied", err: guestaccess.ErrRecoveryDenied, status: 403, calls: 1},
		{name: "internal error", err: errors.New("private credential"), status: 403, calls: 1},
		{name: "expired", expired: true, bytes: 1, status: 503, calls: 1},
		{name: "exhausted", status: 503, calls: 1},
		{name: "negative", bytes: -1, status: 503, calls: 1},
		{name: "oversized", bytes: 1048576000001, status: 503, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expires := time.Now().Add(time.Minute)
			if tc.expired {
				expires = time.Now().Add(-time.Second)
			}
			s := &resumeHandlerStub{err: tc.err, result: guestaccess.ResumeResult{ExpiresAt: expires, RemainingBytes: tc.bytes}}
			h := GuestVoucherHandler{Resumer: s, ResolveDevice: func(string) (*model.Device, error) {
				id := device
				if tc.foreign {
					id = voucher
				}
				return &model.Device{DeviceID: id, BusinessID: "server-tenant", DeploymentLayer: "MAIN_GATEWAY"}, nil
			}, PortalConfig: func(context.Context, string, string) (*model.PortalConfig, error) {
				return &model.PortalConfig{VoucherLoginEnabled: !tc.disabled, ConsentRequired: true}, nil
			}}
			if tc.unwired {
				h.Resumer = nil
			}
			r := gin.New()
			r.POST("/api/v1/portal/:deviceId/guest-resume", h.Resume)
			body := `{"client_mac":"02:00:00:00:00:02","consent":true,"business_id":"attacker","voucher_id":"attacker","secret":"attacker"}`
			if tc.noConsent {
				body = `{"client_mac":"02:00:00:00:00:02"}`
			}
			q := httptest.NewRequest("POST", "/api/v1/portal/"+device+"/guest-resume", strings.NewReader(body))
			q.Header.Set("Content-Type", "application/json")
			if !tc.missing {
				q.AddCookie(&http.Cookie{Name: guestRecoveryCookieName, Value: voucher + "." + secret})
			}
			if tc.duplicate {
				q.AddCookie(&http.Cookie{Name: guestRecoveryCookieName, Value: voucher + "." + secret})
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, q)
			if w.Code != tc.status || s.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, s.calls, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), secret) || strings.Contains(w.Body.String(), "private credential") {
				t.Fatal("leaked or renewed proof")
			}
			if s.calls > 0 && (s.req.BusinessID != "server-tenant" || s.req.DeviceID != device || s.req.VoucherID != voucher || s.req.Secret != secret || s.req.NewMAC != "02:00:00:00:00:02") {
				t.Fatal("body overrode authority")
			}
			if w.Code == 200 && (!strings.Contains(w.Body.String(), `"state":"binding_verified"`) || !strings.Contains(w.Body.String(), `"remaining_bytes":25000000`)) {
				t.Fatal("incorrect connection claim or allowance")
			}
		})
	}
}
