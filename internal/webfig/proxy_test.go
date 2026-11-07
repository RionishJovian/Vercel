package webfig

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type routerRecord struct {
	hits    []string
	cookie  string
	headers http.Header
	block   chan struct{}
}

func fakeRouter(t *testing.T) (*routerRecord, Grant) {
	t.Helper()
	rec := &routerRecord{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.hits = append(rec.hits, r.Method+" "+r.URL.Path)
		rec.cookie, rec.headers = r.Header.Get("Cookie"), r.Header.Clone()
		if r.URL.Path == "/jsproxy" && rec.block != nil {
			<-rec.block
		}
		w.Header().Set("Set-Cookie", "username=admin; path=/")
		_, _ = w.Write([]byte("router:" + r.URL.Path))
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	host, port, _ := net.SplitHostPort(u.Host)
	n, _ := strconv.Atoi(port)
	return rec, Grant{UserID: "u1", BusinessID: "b1", DeviceID: "d1", Role: "OWNER", AuthSessionID: "s1", Scheme: "http", Host: host, Port: n}
}

// host is the subdomain of the session the test opened last.
var host = "webfig.test"

func issue(t *testing.T, p *Proxy, g Grant) string {
	t.Helper()
	ticket, label, err := p.store.Issue(g)
	if err != nil {
		t.Fatal(err)
	}
	host = label + ".webfig.test"
	return ticket
}

func request(p *Proxy, method, path, cookie, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://webfig.test/", nil)
	r.URL.Path, r.URL.RawQuery, _ = strings.Cut(path, "?")
	r.Host = host
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
		r.AddCookie(&http.Cookie{Name: "username", Value: "admin"})
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	r.Header.Set("Authorization", "Bearer app-token")
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	return w
}

func openSession(t *testing.T, p *Proxy, g Grant) string {
	t.Helper()
	ticket := issue(t, p, g)
	w := request(p, http.MethodGet, openPath+"?ticket="+url.QueryEscape(ticket), "", "")
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/webfig/" {
		t.Fatalf("open: %d %q", w.Code, w.Header().Get("Location"))
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieName {
			if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" {
				t.Fatalf("weak session cookie: %+v", c)
			}
			return c.Value
		}
	}
	t.Fatal("no session cookie")
	return ""
}

func allow(context.Context, Grant) error { return nil }

func TestTicketWorksOnceAndExpires(t *testing.T) {
	_, g := fakeRouter(t)
	p := NewProxy("webfig.test", NewStore(), allow)
	ticket := issue(t, p, g)
	if w := request(p, http.MethodGet, openPath+"?ticket="+url.QueryEscape(ticket), "", ""); w.Code != http.StatusFound {
		t.Fatalf("first use: %d", w.Code)
	}
	if w := request(p, http.MethodGet, openPath+"?ticket="+url.QueryEscape(ticket), "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("second use must fail: %d", w.Code)
	}
	now := time.Now()
	p.store.now = func() time.Time { return now }
	late := issue(t, p, g)
	p.store.now = func() time.Time { return now.Add(TicketLifetime) }
	if w := request(p, http.MethodGet, openPath+"?ticket="+url.QueryEscape(late), "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("expired ticket must fail: %d", w.Code)
	}
}

func TestNoSessionNeverReachesRouter(t *testing.T) {
	rec, _ := fakeRouter(t)
	p := NewProxy("webfig.test", NewStore(), allow)
	for _, cookie := range []string{"", "forged"} {
		if w := request(p, http.MethodGet, "/webfig/", cookie, ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("cookie %q: %d", cookie, w.Code)
		}
	}
	if len(rec.hits) != 0 {
		t.Fatalf("router contacted: %v", rec.hits)
	}
}

func TestProxyForwardsOnlyWhatTheRouterShouldSee(t *testing.T) {
	rec, g := fakeRouter(t)
	p := NewProxy("webfig.test", NewStore(), allow)
	value := openSession(t, p, g)
	w := request(p, http.MethodGet, "/webfig/", value, "")
	if w.Code != 200 || w.Body.String() != "router:/webfig/" {
		t.Fatalf("proxy: %d %s", w.Code, w.Body)
	}
	if strings.Contains(rec.cookie, value) || !strings.Contains(rec.cookie, "username=admin") {
		t.Fatalf("session cookie leaked or router cookie lost: %q", rec.cookie)
	}
	if rec.headers.Get("Authorization") != "" || rec.headers.Get("X-Forwarded-For") != "" {
		t.Fatalf("app credentials or client address forwarded: %v", rec.headers)
	}
	if w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("Set-Cookie") == "" {
		t.Fatalf("response headers: %v", w.Header())
	}
}

func TestOnlyWebFigPathsAreForwarded(t *testing.T) {
	rec, g := fakeRouter(t)
	p := NewProxy("webfig.test", NewStore(), allow)
	value := openSession(t, p, g)
	for _, path := range []string{"/rest/system/resource", "/rest", "/scep", "/.well-known/acme-challenge/x", "/api/v1/routers"} {
		if w := request(p, http.MethodGet, path, value, ""); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	if len(rec.hits) != 0 {
		t.Fatalf("router contacted: %v", rec.hits)
	}
}

func TestWritesFromOtherOriginsAreRefused(t *testing.T) {
	rec, g := fakeRouter(t)
	p := NewProxy("webfig.test", NewStore(), allow)
	value := openSession(t, p, g)
	for _, origin := range []string{"", "https://evil.test", "https://megamagic.tech", "https://webfig.test", "https://other.webfig.test"} {
		if w := request(p, http.MethodPost, "/jsproxy", value, origin); w.Code != http.StatusForbidden {
			t.Fatalf("origin %q: %d", origin, w.Code)
		}
	}
	if len(rec.hits) != 0 {
		t.Fatalf("router contacted: %v", rec.hits)
	}
	if w := request(p, http.MethodPost, "/jsproxy", value, "https://"+host); w.Code != 200 {
		t.Fatalf("own origin: %d", w.Code)
	}
}

func TestFailedRecheckEndsTheSession(t *testing.T) {
	rec, g := fakeRouter(t)
	var refuse bool
	p := NewProxy("webfig.test", NewStore(), func(context.Context, Grant) error {
		if refuse {
			return errors.New("locked")
		}
		return nil
	})
	value := openSession(t, p, g)
	now := time.Now()
	p.store.now = func() time.Time { return now.Add(recheckPeriod + time.Second) }
	refuse = true
	if w := request(p, http.MethodGet, "/webfig/", value, ""); w.Code != http.StatusForbidden {
		t.Fatalf("refused check: %d", w.Code)
	}
	refuse = false
	if w := request(p, http.MethodGet, "/webfig/", value, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("session must stay ended: %d", w.Code)
	}
	if len(rec.hits) != 0 {
		t.Fatalf("router contacted: %v", rec.hits)
	}
}

func TestSessionEndsAfterOneHour(t *testing.T) {
	_, g := fakeRouter(t)
	p := NewProxy("webfig.test", NewStore(), allow)
	value := openSession(t, p, g)
	now := time.Now()
	p.store.now = func() time.Time { return now.Add(SessionLifetime + time.Second) }
	if w := request(p, http.MethodGet, "/webfig/", value, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("expired: %d", w.Code)
	}
}

// A server with 30 second limits must not end WebFig's long-held request.
func TestHeldRequestOutlivesServerWriteTimeout(t *testing.T) {
	rec, g := fakeRouter(t)
	rec.block = make(chan struct{})
	p := NewProxy("webfig.test", NewStore(), allow)
	value := openSession(t, p, g)
	front := httptest.NewUnstartedServer(p)
	front.Config.WriteTimeout = 200 * time.Millisecond
	front.Config.ReadTimeout = 200 * time.Millisecond
	front.Start()
	defer front.Close()
	go func() { time.Sleep(700 * time.Millisecond); close(rec.block) }()
	req, _ := http.NewRequest(http.MethodPost, front.URL+"/jsproxy", strings.NewReader("x"))
	req.Host = host
	req.Header.Set("Origin", "https://"+host)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: value})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("held request was cut: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("held request: %d", res.StatusCode)
	}
}

func TestLockSwitch(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want Mode
	}{{"", ModeOff}, {"true", ModeOff}, {"OPEN", ModeOff}, {"open", ModeOpen}, {"locked", ModeLocked}} {
		t.Setenv("MM_WEBFIG_ACCESS", tc.env)
		if got := ModeFromEnv(); got != tc.want {
			t.Fatalf("%q: %s", tc.env, got)
		}
	}
	if Entitled(ModeOff, "DEVELOPER") || Entitled(ModeLocked, "OWNER") || !Entitled(ModeLocked, "DEVELOPER") || !Entitled(ModeOpen, "ADMIN") {
		t.Fatal("entitlement table is wrong")
	}
}

func TestProxyRejectsUntrustedRouterCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("untrusted TLS request reached the router")
	}))
	t.Cleanup(srv.Close)
	host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	p := NewProxy("webfig.test", NewStore(), allow)
	grant := Grant{UserID: "u1", BusinessID: "b1", DeviceID: "d1", Role: "OWNER", AuthSessionID: "s1", Scheme: "https", Host: host, Port: n}
	cookie := openSession(t, p, grant)
	w := request(p, http.MethodGet, "/webfig/", cookie, "")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "Router certificate could not be verified") {
		t.Fatalf("expected TLS verification refusal, got %d: %s", w.Code, w.Body.String())
	}
}

func TestEachSessionIsItsOwnOrigin(t *testing.T) {
	rec, g := fakeRouter(t)
	p := NewProxy("webfig.test", NewStore(), allow)
	first := openSession(t, p, g)
	firstHost := host
	second := openSession(t, p, g)
	if host == firstHost || !strings.HasSuffix(host, ".webfig.test") {
		t.Fatalf("sessions share an address: %q %q", firstHost, host)
	}
	if w := request(p, http.MethodGet, "/webfig/", first, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("a session's cookie must not work on another session's address: %d", w.Code)
	}
	host = firstHost
	if w := request(p, http.MethodGet, "/webfig/", second, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("second cookie on first address: %d", w.Code)
	}
	if w := request(p, http.MethodGet, "/webfig/", first, ""); w.Code != 200 {
		t.Fatalf("opening a second session must leave the first working: %d", w.Code)
	}
	host = "webfig.test"
	if w := request(p, http.MethodGet, "/webfig/", first, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("the bare domain serves no session: %d", w.Code)
	}
	if len(rec.hits) != 1 {
		t.Fatalf("router contacted: %v", rec.hits)
	}
}

func TestTicketOnlyWorksOnItsOwnAddress(t *testing.T) {
	_, g := fakeRouter(t)
	p := NewProxy("webfig.test", NewStore(), allow)
	ticket := issue(t, p, g)
	right := host
	host = "someone-else.webfig.test"
	if w := request(p, http.MethodGet, openPath+"?ticket="+url.QueryEscape(ticket), "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong address: %d", w.Code)
	}
	host = right
	if w := request(p, http.MethodGet, openPath+"?ticket="+url.QueryEscape(ticket), "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("a tried ticket stays spent: %d", w.Code)
	}
}

func TestRouterRepliesCannotReachOutsideTheSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, c := range []string{"__Host-mm_webfig=attacker; Path=/; Secure", "__Secure-x=attacker; Secure", "mm_access=attacker; Domain=megamagic.tech; Path=/", "username=admin; Path=/"} {
			w.Header().Add("Set-Cookie", c)
		}
		for _, name := range []string{"Service-Worker-Allowed", "Clear-Site-Data", "Alt-Svc", "Access-Control-Allow-Origin", "Link"} {
			w.Header().Set(name, "x")
		}
		w.Header().Set("Content-Security-Policy", "frame-ancestors *")
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	h, port, _ := net.SplitHostPort(u.Host)
	n, _ := strconv.Atoi(port)
	p := NewProxy("webfig.test", NewStore(), allow)
	value := openSession(t, p, Grant{Scheme: "http", Host: h, Port: n})
	w := request(p, http.MethodGet, "/webfig/", value, "")
	for _, c := range w.Result().Cookies() {
		if c.Domain != "" || !c.Secure || c.Name == cookieName || strings.HasPrefix(c.Name, "__") {
			t.Fatalf("unsafe cookie passed: %+v", c)
		}
	}
	names := []string{}
	for _, c := range w.Result().Cookies() {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "mm_access,username" {
		t.Fatalf("WebFig's own cookies must still pass, host-only: %v", names)
	}
	for _, name := range []string{"Service-Worker-Allowed", "Clear-Site-Data", "Alt-Svc", "Access-Control-Allow-Origin", "Link"} {
		if w.Header().Get(name) != "" {
			t.Fatalf("%s passed", name)
		}
	}
	if w.Header().Get("Content-Security-Policy") != "frame-ancestors 'none'" {
		t.Fatalf("router replaced the safety header: %q", w.Header().Get("Content-Security-Policy"))
	}
}

func TestBackgroundScriptsAreRefused(t *testing.T) {
	rec, g := fakeRouter(t)
	p := NewProxy("webfig.test", NewStore(), allow)
	value := openSession(t, p, g)
	r := httptest.NewRequest(http.MethodGet, "https://webfig.test/webfig/x.js", nil)
	r.Host = host
	r.Header.Set("Service-Worker", "script")
	r.AddCookie(&http.Cookie{Name: cookieName, Value: value})
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || len(rec.hits) != 0 {
		t.Fatalf("%d %v", w.Code, rec.hits)
	}
}

func TestPathsCannotEscapeTheAllowList(t *testing.T) {
	rec, g := fakeRouter(t)
	p := NewProxy("webfig.test", NewStore(), allow)
	value := openSession(t, p, g)
	for _, raw := range []string{"/webfig/../rest/system/resource", "/webfig/./x", "/jsproxy/..", `/webfig/..\\rest`, "/webfig/%2e%2e/rest", "/webfig/%252e%252e/rest", "/webfig/..%2frest", "/webfig/%2E%2E%2Frest"} {
		parsed, err := url.ParseRequestURI(raw)
		if err != nil {
			parsed = &url.URL{Path: raw}
		}
		r := httptest.NewRequest(http.MethodGet, "https://webfig.test/", nil)
		r.URL = parsed
		r.Host = host
		r.AddCookie(&http.Cookie{Name: cookieName, Value: value})
		w := httptest.NewRecorder()
		p.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", raw, w.Code)
		}
	}
	if len(rec.hits) != 0 {
		t.Fatalf("router contacted: %v", rec.hits)
	}
}

func TestRevokingEndsHeldRequests(t *testing.T) {
	rec, g := fakeRouter(t)
	rec.block = make(chan struct{})
	defer close(rec.block)
	p := NewProxy("webfig.test", NewStore(), allow)
	value := openSession(t, p, g)
	done := make(chan int, 1)
	go func() { done <- request(p, http.MethodPost, "/jsproxy", value, "https://"+host).Code }()
	time.Sleep(150 * time.Millisecond)
	p.store.Revoke(value)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a held request kept running after the session was revoked")
	}
}

// No second browser request arrives to trigger the normal request-time check.
func TestAccessRevocationEndsHeldRequestWithoutNewTraffic(t *testing.T) {
	_, g := fakeRouter(t)
	var revoked atomic.Bool
	p := NewProxy("webfig.test", NewStore(), func(context.Context, Grant) error {
		if revoked.Load() {
			return errors.New("account access revoked")
		}
		return nil
	})
	started := make(chan struct{})
	p.transport = heldTransport{started: started}
	value := openSession(t, p, g)
	done := make(chan struct{})
	r := httptest.NewRequest(http.MethodPost, "https://"+host+"/jsproxy", nil)
	r.Header.Set("Origin", "https://"+host)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: value})
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		p.ServeHTTP(httptest.NewRecorder(), r.WithContext(ctx))
		close(done)
	}()
	<-started
	revoked.Store(true)
	select {
	case <-done:
	case <-time.After(recheckPeriod + 3*time.Second):
		cancel()
		<-done
		t.Fatal("held request survived account revocation without another browser request")
	}
	if _, ok := p.store.Lookup(value, strings.TrimSuffix(host, ".webfig.test"), recheckPeriod); ok {
		t.Fatal("revoked session is still usable")
	}
}

type heldTransport struct{ started chan struct{} }

func (h heldTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	close(h.started)
	<-r.Context().Done()
	return nil, r.Context().Err()
}

func TestWebFigMustLiveOnItsOwnDomain(t *testing.T) {
	for raw, want := range map[string]string{"": "", "megamagic.tech": "", "webfig.megamagic.tech": "", "WEBFIG.MEGAMAGIC.TECH": "", "localhost": "", "*.routers.example": "", "routers.example:443": "", " Routers.Example ": "routers.example"} {
		if got := SafeHost(raw); got != want {
			t.Fatalf("%q: %q", raw, got)
		}
	}
}
