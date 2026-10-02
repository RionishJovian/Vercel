package handler

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegramBindingRequiresApprover(t *testing.T) {
	for _, body := range []string{
		`{"enabled":true,"target_chat_id":-100123,"approver_user_id":0}`,
		`{"enabled":true,"target_chat_id":0,"approver_user_id":123}`,
		`{"enabled":true,"target_chat_id":123,"approver_user_id":-1}`,
		`{"enabled":true,"target_chat_id":123,"approver_user_id":9007199254740992}`,
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("business_id", "business-test")
		c.Request = httptest.NewRequest("PUT", "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		(&GuestWifiHandler{}).PutBotBinding(c)
		if w.Code != 400 {
			t.Fatalf("invalid approver not rejected: %d %s", w.Code, w.Body.String())
		}
	}
}
