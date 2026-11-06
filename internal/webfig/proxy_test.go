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

func request(p *Proxy, method, path, cookie, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://webfig.test"+path, nil)
	r.Host = "webfig.test"
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
	ticket, err := p.store.Issue(g)
	if err != nil {
		t.Fatal(err)
	}
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
	ticket, _ := p.store.Issue(g)
	if w := request(p, http.MethodGet, openPath+"?ticket="+url.QueryEscape(ticket), "", ""); w.Code != http.StatusFound {
		t.Fatalf("first use: %d", w.Code)
	}
	if w := request(p, http.MethodGet, openPath+"?ticket="+url.QueryEscape(ticket), "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("second use must fail: %d", w.Code)
	}
	now := time.Now()
	p.store.now = func() time.Time { return now }
	late, _ := p.store.Issue(g)
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
	for _, origin := range []string{"", "https://evil.test", "https://megamagic.tech"} {
		if w := request(p, http.MethodPost, "/jsproxy", value, origin); w.Code != http.StatusForbidden {
			t.Fatalf("origin %q: %d", origin, w.Code)
		}
	}
	if len(rec.hits) != 0 {
		t.Fatalf("router contacted: %v", rec.hits)
	}
	if w := request(p, http.MethodPost, "/jsproxy", value, "https://webfig.test"); w.Code != 200 {
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
	req.Host = "webfig.test"
	req.Header.Set("Origin", "https://webfig.test")
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
