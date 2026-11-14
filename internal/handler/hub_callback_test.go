package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
)

const (
	cbBiz    = "11111111-1111-4111-8111-111111111111"
	cbDevice = "22222222-2222-4222-8222-222222222222"
	cbUser   = "33333333-3333-4333-8333-333333333333"
)

// ---- fakes -----------------------------------------------------------------

// fakeHubService is a Hub whose allocator, outages and duplicate-address bugs the
// test controls. It allocates one /32 per router and never reuses one.
type fakeHubService struct {
	mu          sync.Mutex
	peers       map[string]hub.Peer
	next        int
	fixedAddr   string // when set, every allocation gets this /32 (a misbehaving Hub)
	enrolls     int
	removes     int
	inspectErr  error
	enrollErr   error
	inFlight    int
	maxInFlight int
	delay       time.Duration
}

func newFakeHub() *fakeHubService { return &fakeHubService{peers: map[string]hub.Peer{}, next: 2} }

func (f *fakeHubService) InspectRouter(_ context.Context, id hub.Identity) (hub.Peer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.inspectErr != nil {
		return hub.Peer{}, f.inspectErr
	}
	p, ok := f.peers[id.TenantID+"/"+id.RouterID]
	if !ok {
		return hub.Peer{}, hub.ErrPeerNotFound
	}
	return p, nil
}

func (f *fakeHubService) EnrollPeer(_ context.Context, id hub.Identity, key string, _ int) (hub.Peer, error) {
	f.mu.Lock()
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	delay := f.delay
	f.mu.Unlock()
	time.Sleep(delay)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inFlight--
	f.enrolls++
	if f.enrollErr != nil {
		return hub.Peer{}, f.enrollErr
	}
	addr := f.fixedAddr
	if addr == "" {
		addr = fmt.Sprintf("10.77.0.%d/32", f.next)
		f.next++
	}
	p := hub.Peer{PeerID: fmt.Sprintf("peer-%d", f.enrolls), TenantID: id.TenantID, RouterID: id.RouterID, RouterPublicKey: key,
		HubPublicKey: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("h", 32))), Endpoint: "hub.example.com:51820",
		AllowedIPs: []string{"10.77.0.1/32"}, TunnelAddress: addr, EnrollmentVersion: 1, State: "active"}
	f.peers[id.TenantID+"/"+id.RouterID] = p
	return p, nil
}

func (f *fakeHubService) RemovePeer(_ context.Context, id hub.Identity, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes++
	delete(f.peers, id.TenantID+"/"+id.RouterID)
	return nil
}

type fakeTokens struct {
	mu     sync.Mutex
	live   map[string][2]string // token -> biz, device
	issued int
}

func (f *fakeTokens) Issue(_ context.Context, biz, device, _ string, _ time.Duration) (string, time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.live == nil {
		f.live = map[string][2]string{}
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	for old, v := range f.live { // one live token per router
		if v == [2]string{biz, device} {
			delete(f.live, old)
		}
	}
	f.live[token] = [2]string{biz, device}
	f.issued++
	return token, time.Now().Add(15 * time.Minute), nil
}

func (f *fakeTokens) Consume(_ context.Context, token string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.live[token]
	if !ok {
		return "", "", database.ErrCallbackTokenInvalid
	}
	delete(f.live, token)
	return v[0], v[1], nil
}

type fakePeerState struct {
	mu         sync.Mutex
	byDevice   map[string]database.HubPeerState
	handshakes map[string]time.Time
}

func (f *fakePeerState) Record(_ context.Context, st database.HubPeerState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.byDevice == nil {
		f.byDevice = map[string]database.HubPeerState{}
	}
	for dev, other := range f.byDevice {
		if dev != st.DeviceID && other.Endpoint == st.Endpoint && (other.TunnelAddress == st.TunnelAddress || other.RouterPublicKey == st.RouterPublicKey) {
			return database.ErrTunnelAddressInUse
		}
	}
	f.byDevice[st.DeviceID] = st
	return nil
}

func (f *fakePeerState) TouchHandshake(_ context.Context, _, device string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.handshakes == nil {
		f.handshakes = map[string]time.Time{}
	}
	f.handshakes[device] = at
	return nil
}

type cbHarness struct {
	h        *VPSHandler
	hub      *fakeHubService
	tokens   *fakeTokens
	state    *fakePeerState
	prepared []string
}

func newCallbackHarness(t *testing.T) *cbHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c := &cbHarness{hub: newFakeHub(), tokens: &fakeTokens{}, state: &fakePeerState{}}
	c.h = &VPSHandler{
		liveHub: c.hub, enrollmentHub: c.hub, enrollmentRemover: c.hub, livePlan: func(string) bool { return true },
		liveDevice: func(_ *gin.Context, biz, id string) (*model.Device, error) {
			return &model.Device{DeviceID: id, BusinessID: biz, DeploymentLayer: "MAIN_GATEWAY"}, nil
		},
		automaticPrepare: func(_ context.Context, _, _, _, script string) (string, error) {
			c.prepared = append(c.prepared, script)
			return script, nil
		},
	}
	c.h.callbackDevice = func(_ context.Context, biz, id string) (*model.Device, error) {
		return &model.Device{DeviceID: id, BusinessID: biz, DeploymentLayer: "MAIN_GATEWAY"}, nil
	}
	if err := c.h.ConfigureCallbackEnrollment("https://megamagic.tech", c.tokens); err != nil {
		t.Fatal(err)
	}
	c.h.ConfigurePeerState(c.state)
	return c
}

// ---- request helpers ---------------------------------------------------------

func issue(h *VPSHandler, role string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", nil)
	c.Params = gin.Params{{Key: "deviceId", Value: cbDevice}}
	c.Set(auth.ContextKeyBusinessID, cbBiz)
	c.Set(auth.ContextKeyUserID, cbUser)
	c.Set(auth.ContextKeyUserRole, role)
	h.IssueCallbackScript(c)
	return w
}

var tokenInScript = regexp.MustCompile(`\\"token\\":\\"([A-Za-z0-9_-]{43})\\"`)

func issueToken(t *testing.T, h *VPSHandler) string {
	t.Helper()
	w := issue(h, "OWNER")
	if w.Code != 200 {
		t.Fatalf("issue status = %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Script  string `json:"router_script"`
		Custody string `json:"key_custody"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Custody != "router_only" {
		t.Fatal("issue response is malformed or claims the wrong key custody")
	}
	m := tokenInScript.FindStringSubmatch(body.Script)
	if m == nil {
		t.Fatal("script carries no token")
	}
	return m[1]
}

func routerPublicKey(t *testing.T) string {
	t.Helper()
	// The router makes its own key. Only the public half ever leaves it, so this
	// test, like the real router, never hands the server the private half.
	_, public, err := hub.GenerateRouterKeypair()
	if err != nil {
		t.Fatal(err)
	}
	return public
}

func callback(h *VPSHandler, body, contentType string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", contentType)
	h.CallbackEnroll(c)
	return w
}

func callbackJSON(token, key, version string) string {
	b, _ := json.Marshal(map[string]string{"token": token, "public_key": key, "routeros_version": version})
	return string(b)
}

// ---- tests -------------------------------------------------------------------

func TestCallbackEnrollmentIsOffUntilConfigured(t *testing.T) {
	h := &VPSHandler{}
	if w := issue(h, "OWNER"); w.Code != 503 || !strings.Contains(w.Body.String(), "hub_callback_disabled") {
		t.Fatalf("issue must be unavailable by default: %d %s", w.Code, w.Body.String())
	}
	if w := callback(h, callbackJSON(strings.Repeat("A", 43), routerPublicKey(t), "7.16 (stable)"), "application/json"); w.Code != 503 {
		t.Fatalf("callback must be unavailable by default: %d", w.Code)
	}
	if err := h.ConfigureCallbackEnrollment("http://megamagic.tech", &fakeTokens{}); err == nil {
		t.Fatal("a plain-HTTP base URL must be refused")
	}
	if err := h.ConfigureCallbackEnrollment("https://megamagic.tech", nil); err == nil {
		t.Fatal("a missing token store must be refused")
	}
}

func TestCallbackIssueNeedsAdminAndSavesNothingSecret(t *testing.T) {
	c := newCallbackHarness(t)
	if w := issue(c.h, "VIEWER"); w.Code != 403 || c.tokens.issued != 0 {
		t.Fatalf("viewer must not issue a script: %d", w.Code)
	}
	w := issue(c.h, "ADMIN")
	if w.Code != 200 {
		t.Fatalf("admin issue = %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "private-key") {
		t.Fatal("the script must never contain a private key")
	}
	if len(c.h.quickKeys) != 0 || c.hub.enrolls != 0 {
		t.Fatal("issuing a script must not create a key or a Hub allocation")
	}
}

func TestCallbackEnrollsOnlyThePublicKey(t *testing.T) {
	c := newCallbackHarness(t)
	token, key := issueToken(t, c.h), routerPublicKey(t)

	w := callback(c.h, callbackJSON(token, key, "7.16.2 (stable)"), "application/json")
	if w.Code != 200 {
		t.Fatalf("callback = %d: %s", w.Code, w.Body.String())
	}
	if c.hub.enrolls != 1 || c.hub.peers[cbBiz+"/"+cbDevice].RouterPublicKey != key {
		t.Fatal("the Hub must enrol exactly the key the router sent")
	}
	if strings.Contains(w.Body.String(), "private-key") || len(c.prepared) != 1 || strings.Contains(c.prepared[0], "private-key") {
		t.Fatal("the answer to the router must not contain a private key")
	}
	if len(c.h.quickKeys) != 0 {
		t.Fatal("the app must hold no router key in this flow")
	}
	st := c.state.byDevice[cbDevice]
	if st.RouterPublicKey != key || st.TunnelAddress != "10.77.0.2/32" || st.Endpoint != "hub.example.com:51820" || st.PeerID == "" {
		t.Fatalf("allocation not recorded: %+v", st)
	}
	if w.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
		t.Fatal("the answer must be uncacheable plain text")
	}
}

func TestCallbackTokenCannotBeReplayed(t *testing.T) {
	c := newCallbackHarness(t)
	token, key := issueToken(t, c.h), routerPublicKey(t)
	if w := callback(c.h, callbackJSON(token, key, "7.16 (stable)"), "application/json"); w.Code != 200 {
		t.Fatal("first use must work")
	}
	if w := callback(c.h, callbackJSON(token, routerPublicKey(t), "7.16 (stable)"), "application/json"); w.Code != 403 {
		t.Fatalf("a spent token must be refused, got %d", w.Code)
	}
	if c.hub.enrolls != 1 {
		t.Fatal("a replay must not reach the Hub")
	}
	if w := callback(c.h, callbackJSON(strings.Repeat("A", 43), key, "7.16 (stable)"), "application/json"); w.Code != 403 {
		t.Fatalf("an unknown token must be refused, got %d", w.Code)
	}
}

func TestCallbackConcurrentReplayHasOneWinner(t *testing.T) {
	c := newCallbackHarness(t)
	c.hub.delay = 20 * time.Millisecond
	token, key := issueToken(t, c.h), routerPublicKey(t)
	codes := make(chan int, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- callback(c.h, callbackJSON(token, key, "7.16 (stable)"), "application/json").Code
		}()
	}
	wg.Wait()
	close(codes)
	ok := 0
	for code := range codes {
		if code == 200 {
			ok++
		}
	}
	if ok != 1 || c.hub.enrolls != 1 {
		t.Fatalf("exactly one request may enrol: wins=%d enrolls=%d", ok, c.hub.enrolls)
	}
}

func TestCallbackRefusesRouterOS6WithoutSpendingTheToken(t *testing.T) {
	c := newCallbackHarness(t)
	token, key := issueToken(t, c.h), routerPublicKey(t)
	for _, version := range []string{"6.49.13 (long-term)", "", "seven", `7.1"; /system reset`} {
		if w := callback(c.h, callbackJSON(token, key, version), "application/json"); w.Code != 422 {
			t.Fatalf("version %q must be refused, got %d", version, w.Code)
		}
	}
	if c.hub.enrolls != 0 || len(c.hub.peers) != 0 {
		t.Fatal("a RouterOS 6 router must never get a Hub allocation")
	}
	// After the router is upgraded, the same script still works.
	if w := callback(c.h, callbackJSON(token, key, "7.16 (stable)"), "application/json"); w.Code != 200 {
		t.Fatalf("token should survive a version refusal, got %d", w.Code)
	}
}

func TestCallbackRejectsMalformedRequestsBeforeSpendingTheToken(t *testing.T) {
	c := newCallbackHarness(t)
	token := issueToken(t, c.h)
	zero := base64.StdEncoding.EncodeToString(make([]byte, 32))
	_, private, _ := hub.GenerateRouterKeypair() // pretend an operator pasted the wrong half: still a valid key shape
	_ = private
	for name, tc := range map[string]struct{ body, contentType string }{
		"wrong content type": {callbackJSON(token, routerPublicKey(t), "7.16 (stable)"), "text/plain"},
		"extra field":        {`{"token":"` + token + `","public_key":"` + routerPublicKey(t) + `","routeros_version":"7.16","private_key":"x"}`, "application/json"},
		"trailing data":      {callbackJSON(token, routerPublicKey(t), "7.16") + `{}`, "application/json"},
		"zero key":           {callbackJSON(token, zero, "7.16 (stable)"), "application/json"},
		"short key":          {callbackJSON(token, "AAAA", "7.16 (stable)"), "application/json"},
		"non-canonical key":  {callbackJSON(token, strings.Repeat("A", 43)+"B", "7.16 (stable)"), "application/json"},
		"bad token shape":    {callbackJSON("short", routerPublicKey(t), "7.16 (stable)"), "application/json"},
		"oversize body":      {callbackJSON(token, routerPublicKey(t), "7.16 (stable)") + strings.Repeat(" ", 4096), "application/json"},
		"not json":           {"token=abc", "application/json"},
	} {
		if w := callback(c.h, tc.body, tc.contentType); w.Code != 400 {
			t.Fatalf("%s: status = %d, want 400", name, w.Code)
		}
	}
	if c.hub.enrolls != 0 {
		t.Fatal("malformed requests must never reach the Hub")
	}
	if w := callback(c.h, callbackJSON(token, routerPublicKey(t), "7.16 (stable)"), "application/json"); w.Code != 200 {
		t.Fatalf("the token must survive malformed attempts, got %d", w.Code)
	}
}

func TestCallbackSurvivesAnOfflineHub(t *testing.T) {
	c := newCallbackHarness(t)
	c.hub.inspectErr = errors.New("hub unreachable")
	token, key := issueToken(t, c.h), routerPublicKey(t)
	w := callback(c.h, callbackJSON(token, key, "7.16 (stable)"), "application/json")
	if w.Code != 503 || c.hub.enrolls != 0 || len(c.state.byDevice) != 0 {
		t.Fatalf("an unreachable Hub must fail closed without changes: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "hub unreachable") {
		t.Fatal("internal error text must not reach the router")
	}
	// Hub recovers: a new script works and allocates exactly once.
	c.hub.inspectErr = nil
	token = issueToken(t, c.h)
	if w := callback(c.h, callbackJSON(token, key, "7.16 (stable)"), "application/json"); w.Code != 200 || c.hub.enrolls != 1 {
		t.Fatalf("recovery failed: %d enrolls=%d", w.Code, c.hub.enrolls)
	}
}

func TestCallbackEnrolmentFailureLeavesNoState(t *testing.T) {
	c := newCallbackHarness(t)
	c.hub.enrollErr = errors.New("hub timed out")
	token, key := issueToken(t, c.h), routerPublicKey(t)
	if w := callback(c.h, callbackJSON(token, key, "7.16 (stable)"), "application/json"); w.Code != 502 {
		t.Fatalf("failed enrolment must be reported, got %d", w.Code)
	}
	if len(c.state.byDevice) != 0 || len(c.prepared) != 0 {
		t.Fatal("a failed enrolment must not record state or prepare management")
	}
}

func TestCallbackRetryReusesTheAllocation(t *testing.T) {
	c := newCallbackHarness(t)
	key := routerPublicKey(t)
	for i := 0; i < 3; i++ {
		if w := callback(c.h, callbackJSON(issueToken(t, c.h), key, "7.16 (stable)"), "application/json"); w.Code != 200 {
			t.Fatalf("attempt %d = %d", i, w.Code)
		}
	}
	if c.hub.enrolls != 1 || c.hub.removes != 0 {
		t.Fatalf("retries with the same key must reuse one /32: enrolls=%d removes=%d", c.hub.enrolls, c.hub.removes)
	}
}

func TestCallbackNeverTearsDownAConnectedRouter(t *testing.T) {
	c := newCallbackHarness(t)
	oldKey := routerPublicKey(t)
	callback(c.h, callbackJSON(issueToken(t, c.h), oldKey, "7.16 (stable)"), "application/json")
	now := time.Now()
	p := c.hub.peers[cbBiz+"/"+cbDevice]
	p.LastHandshakeAt = &now
	c.hub.peers[cbBiz+"/"+cbDevice] = p

	w := callback(c.h, callbackJSON(issueToken(t, c.h), routerPublicKey(t), "7.16 (stable)"), "application/json")
	if w.Code != 409 || c.hub.removes != 0 || c.hub.peers[cbBiz+"/"+cbDevice].RouterPublicKey != oldKey {
		t.Fatalf("a connected router must keep its tunnel: %d removes=%d", w.Code, c.hub.removes)
	}
}

func TestCallbackReplacesAnUnconnectedStaleAllocation(t *testing.T) {
	c := newCallbackHarness(t)
	callback(c.h, callbackJSON(issueToken(t, c.h), routerPublicKey(t), "7.16 (stable)"), "application/json")
	newKey := routerPublicKey(t)
	w := callback(c.h, callbackJSON(issueToken(t, c.h), newKey, "7.16 (stable)"), "application/json")
	if w.Code != 200 || c.hub.removes != 1 || c.hub.enrolls != 2 || c.hub.peers[cbBiz+"/"+cbDevice].RouterPublicKey != newKey {
		t.Fatalf("a never-connected stale peer is replaced once: %d removes=%d enrolls=%d", w.Code, c.hub.removes, c.hub.enrolls)
	}
}

func TestCallbackNeverSharesATunnelAddressBetweenRouters(t *testing.T) {
	c := newCallbackHarness(t)
	c.hub.fixedAddr = "10.77.0.2/32" // a Hub bug: it hands the same /32 to everyone
	other := "44444444-4444-4444-8444-444444444444"

	if w := callback(c.h, callbackJSON(issueToken(t, c.h), routerPublicKey(t), "7.16 (stable)"), "application/json"); w.Code != 200 {
		t.Fatalf("first router = %d", w.Code)
	}
	// Second router in the same business.
	token, _, _ := c.tokens.Issue(context.Background(), cbBiz, other, cbUser, time.Minute)
	w := callback(c.h, callbackJSON(token, routerPublicKey(t), "7.16 (stable)"), "application/json")
	if w.Code != 502 {
		t.Fatalf("a duplicate /32 must be refused, got %d", w.Code)
	}
	if _, orphan := c.hub.peers[cbBiz+"/"+other]; orphan || c.hub.removes != 1 {
		t.Fatal("the refused allocation must be removed from the Hub, not left as an orphan")
	}
	if c.state.byDevice[cbDevice].TunnelAddress != "10.77.0.2/32" || len(c.state.byDevice) != 1 {
		t.Fatal("the first router's allocation must be untouched")
	}
	if len(c.prepared) != 1 {
		t.Fatal("no setup may be prepared for the refused router")
	}
}

func TestEnrolmentsForOneRouterAreSerialised(t *testing.T) {
	s := &retryEnrollmentHub{peers: map[string]hub.Peer{}}
	slow := &slowHub{retryEnrollmentHub: s}
	h := quickRetryHandler(s)
	h.enrollmentHub = slow
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			requestQuickEnrollment(h, "tenant")
		}()
	}
	wg.Wait()
	if len(s.keys) != 1 {
		t.Fatalf("eight simultaneous pastes must make one allocation, made %d", len(s.keys))
	}
}

type slowHub struct{ *retryEnrollmentHub }

func (s *slowHub) EnrollPeer(ctx context.Context, id hub.Identity, key string, port int) (hub.Peer, error) {
	time.Sleep(30 * time.Millisecond)
	return s.retryEnrollmentHub.EnrollPeer(ctx, id, key, port)
}

func TestConnectedRouterIsNotReallocatedAfterItsKeyWasPurged(t *testing.T) {
	s := &retryEnrollmentHub{peers: map[string]hub.Peer{}}
	h := quickRetryHandler(s)
	h.automaticPreflight = func(context.Context, string, string) error { return nil }
	now := time.Now()
	s.peers["tenant/gateway"] = hub.Peer{PeerID: "peer", TenantID: "tenant", RouterID: "gateway", RouterPublicKey: routerPublicKey(t), LastHandshakeAt: &now, State: "active", EnrollmentVersion: 1}
	w := requestQuickEnrollment(h, "tenant")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "hub_peer_already_connected") || len(s.keys) != 0 {
		t.Fatalf("a connected router must not be re-enrolled: %d %s", w.Code, w.Body.String())
	}
}

func TestVerifiedHandshakePurgesTheServerHeldKey(t *testing.T) {
	c := newCallbackHarness(t)
	var purged []string
	c.h.ConfigureRouterKeyPurge(func(_ context.Context, biz, device string) error {
		purged = append(purged, biz+"/"+device)
		return nil
	})
	fresh := time.Now().Add(-30 * time.Second)
	stale := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	peer := func(mut func(*hub.Peer)) hub.Peer {
		p := hub.Peer{PeerID: "p", TenantID: cbBiz, RouterID: cbDevice, State: "active", LastHandshakeAt: &fresh}
		mut(&p)
		return p
	}
	for name, p := range map[string]hub.Peer{
		"never handshaken": peer(func(p *hub.Peer) { p.LastHandshakeAt = nil }),
		"stale handshake":  peer(func(p *hub.Peer) { p.LastHandshakeAt = &stale }),
		"future handshake": peer(func(p *hub.Peer) { p.LastHandshakeAt = &future }),
		"inactive peer":    peer(func(p *hub.Peer) { p.State = "disabled" }),
		"foreign tenant":   peer(func(p *hub.Peer) { p.TenantID = "someone-else" }),
		"foreign router":   peer(func(p *hub.Peer) { p.RouterID = "another-router" }),
	} {
		c.h.observeVerifiedHandshake(context.Background(), cbBiz, cbDevice, p)
		if len(purged) != 0 || len(c.state.handshakes) != 0 {
			t.Fatalf("%s must not purge or record anything", name)
		}
	}
	c.h.observeVerifiedHandshake(context.Background(), cbBiz, cbDevice, peer(func(*hub.Peer) {}))
	if len(purged) != 1 || purged[0] != cbBiz+"/"+cbDevice || !c.state.handshakes[cbDevice].Equal(fresh) {
		t.Fatalf("a verified handshake must purge the key and record the time: %v", purged)
	}
	// Housekeeping must never break the request it rides on.
	c.h.ConfigureRouterKeyPurge(func(context.Context, string, string) error { return errors.New("db down") })
	c.h.observeVerifiedHandshake(context.Background(), cbBiz, cbDevice, peer(func(*hub.Peer) {}))
}

func TestKeyedLockSerialisesPerKeyAndHonoursContext(t *testing.T) {
	var l keyedLock
	var inside, worst int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := l.Lock(context.Background(), "router-a")
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			inside++
			if inside > worst {
				worst = inside
			}
			mu.Unlock()
			time.Sleep(2 * time.Millisecond)
			mu.Lock()
			inside--
			mu.Unlock()
			unlock()
		}()
	}
	wg.Wait()
	if worst != 1 {
		t.Fatalf("one router must never be worked on twice at once, saw %d", worst)
	}
	// Different routers do not block each other.
	unlockA, _ := l.Lock(context.Background(), "a")
	unlockB, err := l.Lock(context.Background(), "b")
	if err != nil {
		t.Fatal("independent routers must not wait for each other")
	}
	unlockB()
	// A waiter gives up when its context ends instead of hanging a worker.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := l.Lock(ctx, "a"); err == nil {
		t.Fatal("a blocked waiter must be released by its context")
	}
	unlockA()
	if len(l.held) != 0 {
		t.Fatalf("lock entries must not leak: %d", len(l.held))
	}
}

func TestCapabilityReportsRouterGeneratedKeysOnlyWhenEnabled(t *testing.T) {
	read := func(h *VPSHandler) map[string]any {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Set(auth.ContextKeyBusinessID, cbBiz)
		h.EnrollmentCapability(c)
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := read(&VPSHandler{})["callback_enrollment_enabled"]; got != false {
		t.Fatalf("must be false by default, got %v", got)
	}
	c := newCallbackHarness(t)
	if got := read(c.h)["callback_enrollment_enabled"]; got != true {
		t.Fatalf("must be true when configured, got %v", got)
	}
}
