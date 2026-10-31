package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAuthHandler(nil, nil, nil)
	h.RegistrationClosed = true
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{}`))
	h.Register(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("closed registration returned %d", w.Code)
	}
}
