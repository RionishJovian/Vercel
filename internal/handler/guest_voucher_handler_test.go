package handler

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/database"
	"mega-magic/internal/guestaccess"
	"mega-magic/internal/model"
)

type guestRedeemStub struct {
	calls int
	req   guestaccess.ProvisionRequest
	err   error
}

func (s *guestRedeemStub) Redeem(_ context.Context, r guestaccess.ProvisionRequest) (model.GuestVoucherGrant, error) {
	s.calls++
	s.req = r
	expiry := time.Now().Add(time.Minute)
	return model.GuestVoucherGrant{BusinessID: r.BusinessID, DeviceID: r.DeviceID, ClientMAC: r.ClientMAC, State: "active", DataMB: 10, ExpiresAt: &expiry}, s.err
}

func TestGuestVoucherRedeemBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const id = "11111111-1111-4111-8111-111111111111"
	for _, tc := range []struct {
		name, body       string
		enabled, consent bool
		err              error
		status, calls    int
	}{
		{"verified", `{"code":" code ","client_mac":"02:00:00:00:00:01","consent":true,"business_id":"attacker"}`, true, true, nil, 200, 1},
		{"terms required", `{"code":"CODE","client_mac":"02:00:00:00:00:01"}`, true, true, nil, 400, 0},
		{"disabled", `{"code":"CODE","client_mac":"02:00:00:00:00:01"}`, false, false, nil, 403, 0},
		{"bad MAC", `{"code":"CODE","client_mac":"00:00:00:00:00:00"}`, true, false, nil, 400, 0},
		{"busy", `{"code":"CODE","client_mac":"02:00:00:00:00:01"}`, true, false, database.ErrGuestVoucherBusy, 409, 1},
		{"uncertain activation", `{"code":"CODE","client_mac":"02:00:00:00:00:01"}`, true, false, guestaccess.ErrProvisionNeedsRepair, 503, 1},
		{"database error hidden", `{"code":"CODE","client_mac":"02:00:00:00:00:01"}`, true, false, errors.New("secret connection detail"), 403, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &guestRedeemStub{err: tc.err}
			h := GuestVoucherHandler{Redeemer: s, ResolveDevice: func(string) (*model.Device, error) {
				return &model.Device{DeviceID: id, BusinessID: "tenant-from-device", DeploymentLayer: "MAIN_GATEWAY"}, nil
			}, PortalConfig: func(context.Context, string, string) (*model.PortalConfig, error) {
				return &model.PortalConfig{VoucherLoginEnabled: tc.enabled, ConsentRequired: tc.consent}, nil
			}}
			r := gin.New()
			r.POST("/portal/:deviceId/guest-voucher", h.Redeem)
			w := httptest.NewRecorder()
			q := httptest.NewRequest("POST", "/portal/"+id+"/guest-voucher", strings.NewReader(tc.body))
			q.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, q)
			if w.Code != tc.status || s.calls != tc.calls {
				t.Fatalf("status=%d calls=%d %s", w.Code, s.calls, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "secret connection") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("unsafe response")
			}
			if s.calls > 0 && (s.req.BusinessID != "tenant-from-device" || s.req.DeviceID != id || s.req.Code != "CODE") {
				t.Fatalf("scope %+v", s.req)
			}
		})
	}
}

func TestGuestVoucherBatchBounds(t *testing.T) {
	b := guestVoucherBatch{DeviceID: "11111111-1111-4111-8111-111111111111", Label: "Daily", Minutes: 1, DataMB: 1, Count: 1}
	if !b.valid() {
		t.Fatal("valid batch rejected")
	}
	for _, mutate := range []func(*guestVoucherBatch){func(b *guestVoucherBatch) { b.Count = 201 }, func(b *guestVoucherBatch) { b.Minutes = 0 }, func(b *guestVoucherBatch) { b.DataMB = 0 }, func(b *guestVoucherBatch) { b.PriceMMK = -1 }, func(b *guestVoucherBatch) { b.DeviceID = "other" }} {
		bad := b
		mutate(&bad)
		if bad.valid() {
			t.Fatalf("invalid batch accepted %+v", bad)
		}
	}
}
