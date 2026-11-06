package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/discovery"
	"mega-magic/internal/discoverystore"
)

type fakeDiscovery struct {
	created   discoverystore.Created
	createErr error
	view      discoverystore.View
	getErr    error
	paired    discoverystore.Paired
	pairErr   error
	obsErr    error
	gotObs    []discoverystore.ObservationIn
	gotToken  string
	gotSite   string
	sel       discoverystore.Selection
	selErr    error
	linkErr   error
}

func (f *fakeDiscovery) Create(_ context.Context, _, _, site string) (discoverystore.Created, error) {
	f.gotSite = site
	return f.created, f.createErr
}
func (f *fakeDiscovery) Get(context.Context, string, string) (discoverystore.View, error) {
	return f.view, f.getErr
}
func (f *fakeDiscovery) Select(context.Context, string, string, string) (discoverystore.Selection, error) {
	return f.sel, f.selErr
}
func (f *fakeDiscovery) Link(context.Context, string, string, string, string) error { return f.linkErr }
func (f *fakeDiscovery) Cancel(context.Context, string, string) error               { return f.getErr }
func (f *fakeDiscovery) Pair(context.Context, string) (discoverystore.Paired, error) {
	return f.paired, f.pairErr
}
func (f *fakeDiscovery) AddObservations(_ context.Context, token, _ string, in []discoverystore.ObservationIn) (int, int, error) {
	f.gotToken, f.gotObs = token, in
	return len(in), 0, f.obsErr
}

const sid = "0a1b2c3d-0000-4000-8000-000000000001"

func router(f *fakeDiscovery) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewDiscoveryHandler(f)
	r := gin.New()
	// Only the owner routes get a tenant; the helper routes must work (and never look for one) without it.
	owner := func(c *gin.Context) {
		c.Set(auth.ContextKeyUserID, "user-1")
		c.Set(auth.ContextKeyBusinessID, "biz-1")
		c.Next()
	}
	r.POST("/s", owner, h.CreateSession)
	r.GET("/s/:id", owner, h.GetSession)
	r.DELETE("/s/:id", owner, h.CancelSession)
	r.POST("/s/:id/select", owner, h.SelectCandidate)
	r.POST("/s/:id/link", owner, h.LinkRouter)
	r.POST("/pair", h.Pair)
	r.POST("/s/:id/observations", h.Observations)
	return r
}

func do(r *gin.Engine, method, path, body string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const goodObs = `{"device_key":"sw:0123456789abcdef","macs":["48:A9:8A:00:00:06"],"identity":"hAP","board":"C52iG-5HaxD2HaxD","version":"7.25","interface":"bridge/wifi2","claimed_ip":"192.168.88.1","source_ip":"192.168.88.1","heard_count":3,"status":"nearby_unverified"}`

func upload(obs string) string { return `{"observations":[` + obs + `]}` }

func TestDiscoveryHandlerCannotReachARouter(t *testing.T) {
	// The store interface is the whole of the handler's reach. Nothing in it can read, log in to or configure a router.
	ty := reflect.TypeOf((*DiscoveryStore)(nil)).Elem()
	allowed := map[string]bool{"Create": true, "Get": true, "Cancel": true, "Pair": true, "AddObservations": true, "Select": true, "Link": true}
	for i := 0; i < ty.NumMethod(); i++ {
		if !allowed[ty.Method(i).Name] {
			t.Fatalf("unexpected capability on the discovery store: %s", ty.Method(i).Name)
		}
	}
	if ty.NumMethod() != len(allowed) {
		t.Fatalf("expected %d methods, got %d", len(allowed), ty.NumMethod())
	}
}

func TestCreateSession(t *testing.T) {
	f := &fakeDiscovery{created: discoverystore.Created{SessionID: sid, PairingCode: "ABCDEF-GHJKMNPQ"}}
	w := do(router(f), "POST", "/s", `{"site_id":"`+sid+`"}`)
	if w.Code != 201 || !strings.Contains(w.Body.String(), "ABCDEF-GHJKMNPQ") || w.Header().Get("Cache-Control") != "no-store" || f.gotSite != sid {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	if w := do(router(&fakeDiscovery{}), "POST", "/s", `{"site_id":"not-a-uuid"}`); w.Code != 400 {
		t.Fatalf("bad site: %d", w.Code)
	}
	if w := do(router(&fakeDiscovery{}), "POST", "/s", `{"site_id":"`+sid+`","business_id":"x"}`); w.Code != 400 {
		t.Fatalf("unknown fields must be refused: %d", w.Code)
	}
	if w := do(router(&fakeDiscovery{createErr: discoverystore.ErrLiveSession}), "POST", "/s", ``); w.Code != 409 {
		t.Fatalf("live session: %d", w.Code)
	}
	if w := do(router(&fakeDiscovery{createErr: discoverystore.ErrBadSite}), "POST", "/s", `{"site_id":"`+sid+`"}`); w.Code != 400 {
		t.Fatalf("missing site: %d", w.Code)
	}
}

func TestOwnerReadsAndCancelsHideForeignSessions(t *testing.T) {
	missing := &fakeDiscovery{getErr: discoverystore.ErrUnavailable}
	for _, m := range []string{"GET", "DELETE"} {
		if w := do(router(missing), m, "/s/"+sid, ""); w.Code != 404 {
			t.Fatalf("%s on a session that is not yours must look like it does not exist: %d", m, w.Code)
		}
	}
	if w := do(router(missing), "GET", "/s/not-a-uuid", ""); w.Code != 404 {
		t.Fatalf("malformed id: %d", w.Code)
	}
	if w := do(router(&fakeDiscovery{view: discoverystore.View{SessionID: sid, State: "open", Candidates: []discoverystore.Candidate{}}}), "GET", "/s/"+sid, ""); w.Code != 200 {
		t.Fatalf("get: %d", w.Code)
	}
}

func TestPair(t *testing.T) {
	ok := &fakeDiscovery{paired: discoverystore.Paired{SessionID: sid, Token: strings.Repeat("a", 64)}}
	if w := do(router(ok), "POST", "/pair", `{"pairing_code":"ABCDEF-GHJKMNPQ"}`); w.Code != 201 || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), sid) {
		t.Fatalf("pair: %d %s", w.Code, w.Body)
	}
	if w := do(router(&fakeDiscovery{pairErr: discoverystore.ErrUnavailable}), "POST", "/pair", `{"pairing_code":"x"}`); w.Code != 401 || strings.Contains(w.Body.String(), "expired") {
		t.Fatalf("every pairing failure must look the same: %d %s", w.Code, w.Body)
	}
	if w := do(router(ok), "POST", "/pair", `{"pairing_code":"A","business_id":"x"}`); w.Code != 400 {
		t.Fatalf("a supplied business id must be refused, not ignored: %d", w.Code)
	}
}

func TestObservationsAcceptsValidAndKeepsStatusUnverified(t *testing.T) {
	f := &fakeDiscovery{}
	w := do(router(f), "POST", "/s/"+sid+"/observations", upload(goodObs), "Authorization", "Bearer "+strings.Repeat("a", 64))
	if w.Code != 200 || len(f.gotObs) != 1 || f.gotToken != strings.Repeat("a", 64) || f.gotObs[0].HeardCount != 3 {
		t.Fatalf("upload: %d %s %+v", w.Code, w.Body, f.gotObs)
	}
}

func TestObservationsRejectsHostileInput(t *testing.T) {
	cases := map[string]string{
		"verified status":      strings.Replace(goodObs, "nearby_unverified", "connected", 1),
		"public claimed ip":    strings.Replace(goodObs, `"claimed_ip":"192.168.88.1"`, `"claimed_ip":"8.8.8.8"`, 1),
		"ipv6 source":          strings.Replace(goodObs, `"source_ip":"192.168.88.1"`, `"source_ip":"fe80::1"`, 1),
		"bad device key":       strings.Replace(goodObs, "sw:0123456789abcdef", "sw:xyz", 1),
		"bad mac":              strings.Replace(goodObs, "48:A9:8A:00:00:06", "48:a9:8a:00:00:06", 1),
		"no macs":              strings.Replace(goodObs, `"macs":["48:A9:8A:00:00:06"]`, `"macs":[]`, 1),
		"control char in name": strings.Replace(goodObs, `"hAP"`, `"h\u0000AP"`, 1),
		"overlong text":        strings.Replace(goodObs, `"hAP"`, `"`+strings.Repeat("a", 65)+`"`, 1),
		"zero count":           strings.Replace(goodObs, `"heard_count":3`, `"heard_count":0`, 1),
		"huge count":           strings.Replace(goodObs, `"heard_count":3`, `"heard_count":999999999`, 1),
		"unknown field":        strings.Replace(goodObs, `"identity"`, `"software_id":"X","identity"`, 1),
	}
	for name, obs := range cases {
		f := &fakeDiscovery{}
		if w := do(router(f), "POST", "/s/"+sid+"/observations", upload(obs), "Authorization", "Bearer "+strings.Repeat("a", 64)); w.Code != 400 || f.gotObs != nil {
			t.Errorf("%s: got %d, store called: %v", name, w.Code, f.gotObs != nil)
		}
	}
	tooMany := make([]string, 65)
	for i := range tooMany {
		tooMany[i] = goodObs
	}
	if w := do(router(&fakeDiscovery{}), "POST", "/s/"+sid+"/observations", `{"observations":[`+strings.Join(tooMany, ",")+`]}`, "Authorization", "Bearer "+strings.Repeat("a", 64)); w.Code != 400 {
		t.Errorf("more than the limit: %d", w.Code)
	}
	big := `{"observations":[],"pad":"` + strings.Repeat("a", maxDiscoveryBody) + `"}`
	if w := do(router(&fakeDiscovery{}), "POST", "/s/"+sid+"/observations", big, "Authorization", "Bearer x"); w.Code != 400 {
		t.Errorf("oversized body: %d", w.Code)
	}
}

func TestObservationsAuthAndRateErrors(t *testing.T) {
	r := router(&fakeDiscovery{})
	if w := do(r, "POST", "/s/"+sid+"/observations", upload(goodObs)); w.Code != 401 {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := do(router(&fakeDiscovery{obsErr: discoverystore.ErrUnavailable}), "POST", "/s/"+sid+"/observations", upload(goodObs), "Authorization", "Bearer "+strings.Repeat("a", 64)); w.Code != 401 {
		t.Fatalf("bad token: %d", w.Code)
	}
	w := do(router(&fakeDiscovery{obsErr: discoverystore.ErrTooFast}), "POST", "/s/"+sid+"/observations", upload(goodObs), "Authorization", "Bearer "+strings.Repeat("a", 64))
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("too fast: %d", w.Code)
	}
}

func TestSelectAndLink(t *testing.T) {
	key := "sw:0123456789abcdef"
	f := &fakeDiscovery{sel: discoverystore.Selection{DeviceKey: key, SuggestedName: "hAP", LocalAddress: "192.168.88.1"}}
	if w := do(router(f), "POST", "/s/"+sid+"/select", `{"device_key":"`+key+`"}`); w.Code != 200 || !strings.Contains(w.Body.String(), "192.168.88.1") {
		t.Fatalf("select: %d %s", w.Code, w.Body)
	}
	for _, bad := range []string{`{"device_key":"nope"}`, `{}`, `{"device_key":"` + key + `","router_id":"x"}`} {
		if w := do(router(f), "POST", "/s/"+sid+"/select", bad); w.Code != 400 {
			t.Errorf("select %s: %d", bad, w.Code)
		}
	}
	if w := do(router(&fakeDiscovery{selErr: discoverystore.ErrUnavailable}), "POST", "/s/"+sid+"/select", `{"device_key":"`+key+`"}`); w.Code != 404 {
		t.Errorf("a candidate that is not in your session must look missing: %d", w.Code)
	}
	if w := do(router(f), "POST", "/s/"+sid+"/link", `{"device_key":"`+key+`","router_id":"`+sid+`"}`); w.Code != 204 {
		t.Errorf("link: %d", w.Code)
	}
	if w := do(router(f), "POST", "/s/"+sid+"/link", `{"device_key":"`+key+`","router_id":"nope"}`); w.Code != 400 {
		t.Errorf("link with a malformed router id: %d", w.Code)
	}
	if w := do(router(&fakeDiscovery{linkErr: discoverystore.ErrConflict}), "POST", "/s/"+sid+"/link", `{"device_key":"`+key+`","router_id":"`+sid+`"}`); w.Code != 409 {
		t.Errorf("conflict: %d", w.Code)
	}
	if w := do(router(&fakeDiscovery{linkErr: discoverystore.ErrUnavailable}), "POST", "/s/"+sid+"/link", `{"device_key":"`+key+`","router_id":"`+sid+`"}`); w.Code != 404 {
		t.Errorf("unselected or foreign: %d", w.Code)
	}
}

// The helper and the server are built from the same repository; this keeps their contract from drifting apart.
func TestServerAcceptsWhatTheHelperActuallySends(t *testing.T) {
	now := time.Now()
	helperObs := []discovery.Observation{
		{Brand: "MikroTik", Method: "MNDP", Status: discovery.StatusNearbyUnverified, MAC: "48:A9:8A:00:00:06", MACs: []string{"48:A9:8A:00:00:06"}, DeviceKey: "sw:0123456789abcdef",
			Identity: "hAP", Board: "C52iG-5HaxD2HaxD", Platform: "MikroTik", Version: "7.25 2026-10-22 14:12:00", Interface: "bridge/wifi2", ClaimedIP: "192.168.88.1", SourceIP: "192.168.88.1", Count: 3, FirstSeen: now, LastSeen: now},
		{Brand: "MikroTik", Method: "MNDP", Status: discovery.StatusNearbyUnverified, MAC: "48:A9:8A:00:00:07", ClaimedIP: "192.168.88.1", SourceIP: "192.168.88.1", Count: 1, FirstSeen: now, LastSeen: now},
	}
	payload, err := discovery.Payload(helperObs)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeDiscovery{}
	w := do(router(f), "POST", "/s/"+sid+"/observations", string(payload), "Authorization", "Bearer "+strings.Repeat("a", 64))
	if w.Code != 200 || len(f.gotObs) != 2 {
		t.Fatalf("the server must accept the helper's own payload: %d %s", w.Code, w.Body)
	}
	if f.gotObs[1].DeviceKey != "mac:48:A9:8A:00:00:07" {
		t.Fatalf("a device without a software id must be keyed by MAC: %+v", f.gotObs[1])
	}
}

func TestCapabilityReportsEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/c", NewDiscoveryHandler(&fakeDiscovery{}).Capability)
	if w := do(r, "GET", "/c", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":true`) {
		t.Fatalf("capability: %d %s", w.Code, w.Body)
	}
}
