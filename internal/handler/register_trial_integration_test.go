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

// Self registration creates one business, one owner, the authorized login and a
// seven-day trial that grants access, then stops granting it.
func TestRegisterStartsSevenDayTrial(t *testing.T) {
	db := harness_testdb(t)
	svc, err := auth.NewService("test-register-secret-at-least-32-characters", time.Hour, 24*time.Hour, 4)
	if err != nil {
		t.Fatal(err)
	}
	svc.UseSessions(db)
	h := NewAuthHandler(database.NewUserRepository(db), svc, db)
	h.IndividualPassesEnabled = true
	gin.SetMode(gin.TestMode)
	email := "trial-" + randHex(6) + "@example.test"
	body := `{"company_name":"Trial Cafe","full_name":"Trial Owner","email":"` + email + `","password":"correct horse battery"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		User struct {
			UserID string `json:"user_id"`
		} `json:"user"`
		Business struct {
			BusinessID string `json:"business_id"`
		} `json:"business"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	status := func(now time.Time) (state string, access bool) {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		err = database.WithTenant(tx, out.Business.BusinessID, func() error {
			s, e := database.AccountPassStatusTXN(tx, out.Business.BusinessID, out.User.UserID, true, false, now)
			state, access = s.State, s.Access
			return e
		})
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	if s, ok := status(time.Now().UTC()); s != "trial" || !ok {
		t.Fatalf("fresh registration: state=%s access=%v", s, ok)
	}
	if s, ok := status(time.Now().UTC().Add(6*24*time.Hour + 23*time.Hour)); s != "trial" || !ok {
		t.Fatalf("day seven: state=%s access=%v", s, ok)
	}
	if s, ok := status(time.Now().UTC().Add(7*24*time.Hour + time.Minute)); s != "pass_required" || ok {
		t.Fatalf("after the trial: state=%s access=%v", s, ok)
	}
	// The same email cannot register again, so a trial is granted once per login.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.Register(c2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("duplicate registration returned %d", w2.Code)
	}
}
