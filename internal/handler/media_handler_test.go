package handler

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/media/bridge"
	"mega-magic/internal/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mediaStoreFake struct {
	bid, actor, pairHash, tokenHash string
	calls                           int
}

func (f *mediaStoreFake) Pair(_ context.Context, bid, hash string) error {
	f.bid = bid
	f.pairHash = hash
	f.calls++
	return nil
}
func (f *mediaStoreFake) Redeem(_ context.Context, pair, token, name string) error {
	f.pairHash = pair
	f.tokenHash = token
	f.calls++
	return nil
}
func (f *mediaStoreFake) Connection(_ context.Context, bid string) (bridge.Connection, error) {
	f.bid = bid
	f.calls++
	return bridge.Connection{}, nil
}
func (f *mediaStoreFake) Revoke(_ context.Context, bid string) error {
	f.bid = bid
	f.calls++
	return nil
}
func (f *mediaStoreFake) Enqueue(_ context.Context, bid, actor string, j bridge.Job) error {
	f.bid = bid
	f.actor = actor
	f.calls++
	return nil
}
func (f *mediaStoreFake) GetJob(_ context.Context, bid, id string) (bridge.Job, error) {
	f.bid = bid
	f.calls++
	return bridge.Job{ID: id}, nil
}
func (f *mediaStoreFake) Poll(_ context.Context, hash string) (*bridge.Job, error) {
	f.tokenHash = hash
	f.calls++
	return nil, nil
}
func (f *mediaStoreFake) Complete(_ context.Context, hash, id string, c bridge.Completion) error {
	f.tokenHash = hash
	f.calls++
	return nil
}
func TestMediaOwnerScopeAndRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []string{"OWNER", "ADMIN", "VIEWER"} {
		store := &mediaStoreFake{}
		h := NewMediaHandler(store)
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(auth.ContextKeyBusinessID, "tenant-from-session")
			c.Set(auth.ContextKeyUserID, "actor-from-session")
			c.Set(auth.ContextKeyUserRole, role)
		})
		r.POST("/jobs", auth.RequireMinRole(model.RoleAdmin), h.Enqueue)
		body := `{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","operation":"libraries","payload":{},"business_id":"victim","actor_id":"victim"}`
		req := httptest.NewRequest("POST", "/jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if role == "OWNER" || role == "ADMIN" {
			if w.Code != 202 || store.bid != "tenant-from-session" || store.actor != "actor-from-session" {
				t.Fatalf("lost tenant scope: %d %+v", w.Code, store)
			}
		} else if w.Code != 403 || store.calls != 0 {
			t.Fatal("insufficient role reached store")
		}
	}
}
func TestMediaConnectorCredentialBoundary(t *testing.T) {
	store := &mediaStoreFake{}
	h := NewMediaHandler(store)
	r := gin.New()
	r.POST("/poll", h.Poll)
	r.POST("/redeem", h.Redeem)
	req := httptest.NewRequest("POST", "/poll", strings.NewReader(`{}`))
	req.AddCookie(&http.Cookie{Name: "mm_access", Value: "browser-session"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 || store.calls != 0 {
		t.Fatal("browser cookie authenticated connector")
	}
	token := strings.Repeat("a", 64)
	req = httptest.NewRequest("POST", "/poll", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	hash, _ := bridge.Hash(token)
	if w.Code != 204 || store.tokenHash != hash || store.tokenHash == token {
		t.Fatal("token was not hashed")
	}
	req = httptest.NewRequest("POST", "/redeem", strings.NewReader(`{"pairing_code":"`+token+`","server_name":"Media PC","business_id":"victim"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var result map[string]string
	json.Unmarshal(w.Body.Bytes(), &result)
	returned := result["connector_token"]
	returnedHash, _ := bridge.Hash(returned)
	if w.Code != 201 || returnedHash != store.tokenHash || store.tokenHash == returned || store.pairHash != hash || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("unsafe redemption")
	}
}
func TestMediaMutationsRequireConfirmation(t *testing.T) {
	store := &mediaStoreFake{}
	h := NewMediaHandler(store)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(auth.ContextKeyBusinessID, "tenant")
		c.Set(auth.ContextKeyUserID, "actor")
	})
	r.POST("/jobs", h.Enqueue)
	for _, op := range []string{"scan", "add_library", "shell"} {
		req := httptest.NewRequest("POST", "/jobs", strings.NewReader(`{"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","operation":"`+op+`","payload":{}}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 400 || store.calls != 0 {
			t.Fatal("unguarded operation")
		}
	}
}
