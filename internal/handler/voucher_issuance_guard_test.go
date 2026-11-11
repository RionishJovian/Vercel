package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
)

func TestGenericVoucherCreateRejectsRouterHotSpotCodesBeforeAnyWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, plan := range []string{"guest-internet", "GUEST-INTERNET", " guest-internet "} {
		t.Run(plan, func(t *testing.T) {
			r := gin.New()
			// Nil dependencies make any unexpected DB write panic: rejecting
			// this plan must happen before the generic voucher transaction.
			h := &VoucherHandler{}
			r.POST("/api/v1/vouchers", func(c *gin.Context) {
				c.Set(auth.ContextKeyBusinessID, "fixture-business")
			}, h.CreateVoucher)
			w := httptest.NewRecorder()
			body := "{\"plan_key\":\"" + plan + "\",\"plan_label\":\"Internet\",\"count\":1}"
			req := httptest.NewRequest(http.MethodPost, "/api/v1/vouchers", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("generic route issued an Internet Access Voucher: HTTP %d, body %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "/api/v1/guest-vouchers") {
				t.Fatalf("response must direct owner to router-verified issuer: %s", w.Body.String())
			}
		})
	}
}
