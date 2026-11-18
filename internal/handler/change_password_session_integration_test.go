package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"mega-magic/internal/auth"
	"mega-magic/internal/database"
)

// Changing a password revokes every session in the database. The person who just changed it keeps a
// fresh session on this browser; the session they had before (any other device) stays signed out.
func TestChangePasswordKeepsThisBrowserSignedIn(t *testing.T) {
	db := harness_testdb(t)
	svc, err := auth.NewService("test-changepw-secret-at-least-32-characters", time.Hour, 24*time.Hour, 4)
	if err != nil {
		t.Fatal(err)
	}
	svc.UseSessions(db)
	h := NewAuthHandler(database.NewUserRepository(db), svc, db)
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	email := "chpw-" + randHex(6) + "@example.test"
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"company_name":"Pw Cafe","full_name":"Pw Owner","email":"`+email+`","password":"Correct-horse-battery-1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	var reg struct {
		Token string `json:"token"`
		User  struct {
			UserID string `json:"user_id"`
		} `json:"user"`
		Business struct {
			BusinessID string `json:"business_id"`
		} `json:"business"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reg); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateToken(reg.Token); err != nil {
		t.Fatalf("session before change should be valid: %v", err)
	}

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/me/password", strings.NewReader(`{"current_password":"Correct-horse-battery-1","new_password":"Another-long-passphrase-2"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(auth.ContextKeyUserID, reg.User.UserID)
	c.Set(auth.ContextKeyBusinessID, reg.Business.BusinessID)
	h.ChangePassword(c)
	if w.Code != http.StatusOK {
		t.Fatalf("change password: %d %s", w.Code, w.Body.String())
	}
	var fresh string
	for _, ck := range w.Result().Cookies() {
		if ck.Name == "mm_access" {
			fresh = ck.Value
		}
	}
	if fresh == "" {
		t.Fatal("no fresh access cookie after the password change")
	}
	if _, err := svc.ValidateToken(fresh); err != nil {
		t.Fatalf("fresh session should be valid: %v", err)
	}
	if _, err := svc.ValidateToken(reg.Token); err == nil {
		t.Fatal("the pre-change session must stay revoked")
	}
}
