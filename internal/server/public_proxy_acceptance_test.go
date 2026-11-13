package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
)

// Opt-in, disposable, loopback-only real-Caddy chain. No database, production
// service, RouterOS device or paid guest is contacted. Secure rate cookies are
// replayed by this HTTP fixture explicitly; this is not HTTPS browser acceptance.
func TestRealCaddySharedNATAcceptance(t *testing.T) {
	edgeBin, webBin := os.Getenv("MM_CADDY_EDGE_BIN"), os.Getenv("MM_CADDY_WEB_BIN")
	if edgeBin == "" || webBin == "" {
		t.Skip("set exact local Caddy binaries for disposable proxy-chain acceptance")
	}
	gin.SetMode(gin.TestMode)
	newEngine := func() *gin.Engine {
		r := gin.New()
		if err := configureProxyTrust(r, []string{"127.0.0.1"}); err != nil {
			t.Fatal(err)
		}
		r.Use(auth.PublicRequestLimits("disposable-proxy-fixture-signing-key-32-bytes"))
		r.GET("/api/v1/portal/:device/config", func(c *gin.Context) { c.JSON(200, gin.H{"source": c.ClientIP()}) })
		return r
	}
	api := httptest.NewServer(newEngine())
	defer api.Close()
	port := func() int {
		ln, e := net.Listen("tcp4", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		p := ln.Addr().(*net.TCPAddr).Port
		ln.Close()
		return p
	}
	webPort, edgePort := port(), port()
	work := t.TempDir()
	start := func(binary, name, config string) func() {
		path := filepath.Join(work, name+".Caddyfile")
		if e := os.WriteFile(path, []byte(config), 0600); e != nil {
			t.Fatal(e)
		}
		log, e := os.Create(filepath.Join(work, name+".log"))
		if e != nil {
			t.Fatal(e)
		}
		cmd := exec.Command(binary, "run", "--config", path, "--adapter", "caddyfile")
		cmd.Env = append(os.Environ(), "XDG_DATA_HOME="+work, "XDG_CONFIG_HOME="+work)
		cmd.Stdout = log
		cmd.Stderr = log
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		var once sync.Once
		stop := func() {
			once.Do(func() {
				_ = cmd.Process.Signal(os.Interrupt)
				done := make(chan struct{})
				go func() { _ = cmd.Wait(); close(done) }()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					_ = cmd.Process.Kill()
					<-done
				}
				log.Close()
			})
		}
		t.Cleanup(stop)
		return stop
	}
	webConfig := fmt.Sprintf("{\n admin off\n auto_https off\n servers {\n trusted_proxies static 127.0.0.1\n trusted_proxies_strict\n client_ip_headers X-Forwarded-For\n }\n}\nhttp://:%d {\n bind 127.0.0.1\n reverse_proxy %s\n}\n", webPort, api.URL)
	edgeConfig := fmt.Sprintf("{\n admin off\n auto_https off\n}\nhttp://:%d {\n bind 127.0.0.1 ::1\n reverse_proxy 127.0.0.1:%d\n}\n", edgePort, webPort)
	stopWeb := start(webBin, "web", webConfig)
	start(edgeBin, "edge", edgeConfig)
	tr := &http.Transport{Proxy: nil}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	base := fmt.Sprintf("http://127.0.0.1:%d/api/v1/portal/fixture/config", edgePort)
	request := func(url string, cookie *http.Cookie, spoof bool) (int, string, *http.Cookie, string, error) {
		q, e := http.NewRequest("GET", url, nil)
		if e != nil {
			return 0, "", nil, "", e
		}
		if cookie != nil {
			q.AddCookie(cookie)
		}
		if spoof {
			q.Header.Set("X-Forwarded-For", "203.0.113.66")
			q.Header.Set("X-Real-IP", "203.0.113.77")
		}
		resp, e := client.Do(q)
		if e != nil {
			return 0, "", nil, "", e
		}
		defer resp.Body.Close()
		var body struct {
			Source string `json:"source"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&body)
		if resp.StatusCode == 200 && decodeErr != nil {
			return resp.StatusCode, "", nil, "", decodeErr
		}
		var rate *http.Cookie
		for _, c := range resp.Cookies() {
			if c.Name == "__Secure-mm_rate" {
				rate = c
			}
		}
		return resp.StatusCode, body.Source, rate, resp.Header.Get("Retry-After"), nil
	}
	wait := func() {
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			code, _, _, _, e := request(base, nil, false)
			if e == nil && code == 200 {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("disposable Caddy chain did not become ready; inspect fixture logs")
	}
	wait()
	for _, tc := range []struct{ url, want string }{{base, "127.0.0.1"}, {fmt.Sprintf("http://[::1]:%d/api/v1/portal/fixture/config", edgePort), "::1"}} {
		code, source, _, _, e := request(tc.url, nil, true)
		if e != nil || code != 200 || source != tc.want {
			t.Fatalf("forwarding/spoof refusal: code=%d source=%q want=%q err=%v", code, source, tc.want, e)
		}
	}
	cookies := make([]*http.Cookie, 60)
	for i := range cookies {
		code, _, c, _, e := request(base, nil, false)
		if e != nil || code != 200 || c == nil {
			t.Fatalf("bootstrap %d: code=%d err=%v", i, code, e)
		}
		cookies[i] = c
	}
	var wg sync.WaitGroup
	for i, c := range cookies {
		wg.Add(1)
		go func(i int, c *http.Cookie) {
			defer wg.Done()
			for n := 0; n < 20; n++ {
				code, _, _, _, e := request(base, c, true)
				if e != nil || code != 200 {
					t.Errorf("NAT browser %d request %d: code=%d err=%v", i, n, code, e)
					return
				}
			}
		}(i, c)
	}
	wg.Wait()
	for n := 0; n < 9; n++ {
		code, _, _, _, e := request(base, cookies[0], false)
		if e != nil || code != 200 {
			t.Fatalf("budget used early: %d %v", code, e)
		}
	}
	code, _, _, retry, e := request(base, cookies[0], true)
	if e != nil || code != 429 || retry == "" {
		t.Fatalf("browser budget/spoof limit: %d retry=%q err=%v", code, retry, e)
	}
	code, _, _, _, e = request(base, cookies[1], false)
	if e != nil || code != 200 {
		t.Fatalf("other NAT customer blocked: %d %v", code, e)
	}
	stopWeb()
	tr.CloseIdleConnections()
	stopWeb = start(webBin, "web-restarted", webConfig)
	defer stopWeb()
	wait()
	code, source, _, _, e := request(base, cookies[1], true)
	if e != nil || code != 200 || source != "127.0.0.1" {
		t.Fatalf("web restart changed trust/session handling: %d %q %v", code, source, e)
	}
	apiAddr := api.Listener.Addr().String()
	api.Close()
	ln, err := net.Listen("tcp4", apiAddr)
	if err != nil {
		t.Fatal(err)
	}
	restartedAPI := httptest.NewUnstartedServer(newEngine())
	restartedAPI.Listener.Close()
	restartedAPI.Listener = ln
	restartedAPI.Start()
	defer restartedAPI.Close()
	tr.CloseIdleConnections()
	wait()
	code, source, reissued, _, err := request(base, cookies[1], true)
	if err != nil || code != 200 || source != "127.0.0.1" || reissued != nil {
		t.Fatalf("API restart did not preserve signed session recognition: code=%d source=%q reissued=%v err=%v", code, source, reissued != nil, err)
	}
	t.Log("PASS: actual Caddy edge/web chain; IPv4+IPv6 source distinction; spoof refusal; 60 shared-NAT rate sessions x20 requests; 429+Retry-After; unaffected second customer; web and API restart (counters reset, signed session retained). HTTP fixture only, no hardware/TLS-browser certification.")
}
