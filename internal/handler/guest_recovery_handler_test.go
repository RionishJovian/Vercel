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

type recoveryHandlerStub struct {
	calls  int
	req    guestaccess.RecoveryRequest
	err    error
	mutate func(*guestaccess.RecoveryClaim)
}

func (s *recoveryHandlerStub) Restore(_ context.Context, req guestaccess.RecoveryRequest) (guestaccess.RecoveryClaim, error) {
	s.calls++
	s.req = req
	g := guestaccess.RecoveryClaim{BusinessID: req.BusinessID, DeviceID: req.DeviceID, VoucherID: req.VoucherID,
		TransferID: "33333333-3333-4333-8333-333333333333", OldMAC: "02:00:00:00:00:01", NewMAC: req.NewMAC,
		TotalBytes: 75000000, UsedBytes: 40000000, ExpiresAt: time.Now().Add(time.Minute)}
	if s.mutate != nil {
		s.mutate(&g)
	}
	return g, s.err
}

func TestGuestRecoveryHTTPBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const deviceID = "11111111-1111-4111-8111-111111111111"
	const voucherID = "22222222-2222-4222-8222-222222222222"
	secret, err := guestaccess.RecoverySecret()
	if err != nil {
		t.Fatal(err)
	}
	validBody := `{"client_mac":"02:00:00:00:00:02","consent":true,"business_id":"attacker","voucher_id":"attacker","secret":"attacker"}`
	for _, tc := range []struct {
		name, body, cookie                                 string
		status, calls                                      int
		err                                                error
		mutate                                             func(*guestaccess.RecoveryClaim)
		disabled, missingConsent, foreignDevice, duplicate bool
	}{
		{name: "verified", status: 200, calls: 1},
		{name: "missing cookie", cookie: "missing", status: 403},
		{name: "bad proof", cookie: voucherID + ".CODE", status: 403},
		{name: "bad voucher", cookie: "bad." + secret, status: 403},
		{name: "duplicate cookie", duplicate: true, status: 403},
		{name: "bad MAC", body: `{"client_mac":"00:00:00:00:00:00"}`, status: 400},
		{name: "malformed body", body: `{`, status: 400},
		{name: "terms", missingConsent: true, status: 400},
		{name: "disabled", disabled: true, status: 403},
		{name: "foreign device result", foreignDevice: true, status: 404},
		{name: "denied", err: guestaccess.ErrRecoveryDenied, status: 403, calls: 1},
		{name: "uncertain", err: guestaccess.ErrRecoveryNeedsRepair, status: 503, calls: 1},
		{name: "internal error", err: errors.New("private server credential"), status: 403, calls: 1},
		{name: "foreign claim", mutate: func(g *guestaccess.RecoveryClaim) { g.BusinessID = "other" }, status: 503, calls: 1},
		{name: "wrong gateway", mutate: func(g *guestaccess.RecoveryClaim) { g.DeviceID = voucherID }, status: 503, calls: 1},
		{name: "wrong voucher", mutate: func(g *guestaccess.RecoveryClaim) { g.VoucherID = deviceID }, status: 503, calls: 1},
		{name: "wrong new MAC", mutate: func(g *guestaccess.RecoveryClaim) { g.NewMAC = g.OldMAC }, status: 503, calls: 1},
		{name: "missing transfer", mutate: func(g *guestaccess.RecoveryClaim) { g.TransferID = "" }, status: 503, calls: 1},
		{name: "expired", mutate: func(g *guestaccess.RecoveryClaim) { g.ExpiresAt = time.Now() }, status: 503, calls: 1},
		{name: "exhausted", mutate: func(g *guestaccess.RecoveryClaim) { g.UsedBytes = g.TotalBytes }, status: 503, calls: 1},
		{name: "negative usage", mutate: func(g *guestaccess.RecoveryClaim) { g.UsedBytes = -1 }, status: 503, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &recoveryHandlerStub{err: tc.err, mutate: tc.mutate}
			h := GuestVoucherHandler{Recoverer: s, ResolveDevice: func(string) (*model.Device, error) {
				id := deviceID
				if tc.foreignDevice {
					id = voucherID
				}
				return &model.Device{DeviceID: id, BusinessID: "server-tenant", DeploymentLayer: "MAIN_GATEWAY"}, nil
			}, PortalConfig: func(context.Context, string) (*model.PortalConfig, error) {
				return &model.PortalConfig{VoucherLoginEnabled: !tc.disabled, ConsentRequired: true}, nil
			}}
			r := gin.New()
			r.POST("/api/v1/portal/:deviceId/guest-recovery", h.Recover)
			body := validBody
			if tc.body != "" {
				body = tc.body
			}
			if tc.missingConsent {
				body = `{"client_mac":"02:00:00:00:00:02"}`
			}
			q := httptest.NewRequest("POST", "/api/v1/portal/"+deviceID+"/guest-recovery", strings.NewReader(body))
			q.Header.Set("Content-Type", "application/json")
			cookie := tc.cookie
			if cookie == "" {
				cookie = voucherID + "." + secret
			}
			if cookie != "missing" {
				q.AddCookie(&http.Cookie{Name: guestRecoveryCookieName, Value: cookie})
			}
			if tc.duplicate {
				q.AddCookie(&http.Cookie{Name: guestRecoveryCookieName, Value: voucherID + "." + secret})
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, q)
			if w.Code != tc.status || s.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, s.calls, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), secret) || strings.Contains(w.Body.String(), "private server") {
				t.Fatal("recovery leaked or renewed ownership proof")
			}
			if s.calls != 0 && (s.req.BusinessID != "server-tenant" || s.req.DeviceID != deviceID || s.req.VoucherID != voucherID || s.req.Secret != secret || s.req.NewMAC != "02:00:00:00:00:02") {
				t.Fatal("guest JSON overrode authoritative ownership")
			}
			if w.Code == 200 && !strings.Contains(w.Body.String(), `"allocated_bytes":35000000`) {
				t.Fatal("returned replenished allocation")
			}
		})
	}
}
