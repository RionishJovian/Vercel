package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/media/bridge"
	"mega-magic/internal/model"
)

type publicationFake struct {
	bid   string
	calls int
	err   error
}

func (f *publicationFake) Publication(_ context.Context, bid string) (bridge.Publication, error) {
	f.calls++
	f.bid = bid
	return bridge.Publication{}, sql.ErrNoRows
}
func (f *publicationFake) Publish(_ context.Context, bid string, d bridge.MediaDesign, rev int64) (bridge.Publication, error) {
	f.calls++
	f.bid = bid
	return bridge.Publication{Design: d, PublicID: strings.Repeat("a", 64), Revision: rev + 1}, f.err
}
func (f *publicationFake) Unpublish(_ context.Context, bid string, rev int64) error {
	f.calls++
	f.bid = bid
	return f.err
}
func (f *publicationFake) PublicPublication(_ context.Context, id string) (bridge.Publication, error) {
	f.calls++
	return bridge.Publication{}, f.err
}
func TestMediaPublicationBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	d := bridge.MediaDesign{Name: "Cinema", Theme: "charcoal", Accent: "#eeeeee", Price: 1500, Duration: 24, Layout: "shelves", PosterDensity: "compact"}
	raw, _ := json.Marshal(d)
	for _, tc := range []struct {
		name, role, revision, body string
		ready                      bool
		err                        error
		want, calls                int
	}{
		{"publish", "OWNER", "0", string(raw), true, nil, 200, 1},
		{"role", "VIEWER", "0", string(raw), true, nil, 403, 0},
		{"unmigrated", "OWNER", "0", string(raw), false, nil, 503, 0},
		{"missing revision", "OWNER", "", string(raw), true, nil, 400, 0},
		{"unknown credential", "OWNER", "0", strings.TrimSuffix(string(raw), "}") + `,"api_key":"secret"}`, true, nil, 400, 0},
		{"conflict", "OWNER", "1", string(raw), true, bridge.ErrPublicationConflict, 409, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &publicationFake{err: tc.err}
			h := &MediaPublicationHandler{Store: f, Ready: tc.ready}
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set(auth.ContextKeyBusinessID, "session-tenant")
				c.Set(auth.ContextKeyUserRole, tc.role)
			})
			r.PUT("/publication", auth.RequireMinRole(model.RoleAdmin), h.Publish)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("PUT", "/publication?revision="+tc.revision, strings.NewReader(tc.body))
			r.ServeHTTP(w, req)
			if w.Code != tc.want || f.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, f.calls, w.Body.String())
			}
			if f.calls > 0 && f.bid != "session-tenant" {
				t.Fatal("tenant scope lost")
			}
		})
	}
	f := &publicationFake{}
	h := &MediaPublicationHandler{Store: f, Ready: true}
	r := gin.New()
	r.GET("/guest/:id", h.Public)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/guest/"+strings.Repeat("a", 64), nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"playback_enabled":false`) || !strings.Contains(w.Body.String(), `"purchases_enabled":false`) {
		t.Fatal("public response implies playback readiness")
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/guest/not-an-id", nil))
	if w.Code != 404 || f.calls != 1 {
		t.Fatal("invalid sharing ID reached store")
	}
}
