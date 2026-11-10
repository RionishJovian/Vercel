package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
)

// No database is wired: the generic Internet Access Voucher creation guard
// must reject the request before any transaction or RouterOS change.
func TestLegacyVoucherCreateCannotIssueUnprovisionedInternetAccessCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, planKey := range []string{"guest-internet", " GUEST-INTERNET ", "Guest-Internet"} {
		t.Run(planKey, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(auth.ContextKeyBusinessID, "owner-business")
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/vouchers", strings.NewReader(
				`{"plan_key":"` + planKey + `","plan_label":"Internet","duration_minutes":60,"count":1}`))
			c.Request.Header.Set("Content-Type", "application/json")

			(&VoucherHandler{}).CreateVoucher(c)
			if w.Code != http.StatusConflict {
				t.Fatalf("unverified HotSpot voucher creation accepted: %d %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "/api/v1/guest-vouchers") {
				t.Fatalf("response did not guide owner to the verified issue route: %s", w.Body.String())
			}
			if strings.Contains(w.Body.String(), "\"vouchers\"") {
				t.Fatalf("unverified voucher code returned: %s", w.Body.String())
			}
		})
	}
}
