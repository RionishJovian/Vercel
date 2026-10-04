package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"mega-magic/internal/media/bridge"
	"mega-magic/internal/media/connector"
	"mega-magic/internal/media/jellyfin"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const relayLibrary = "11111111111111111111111111111111"
const relayItem = "22222222222222222222222222222222"

var relayPublication = strings.Repeat("a", 64)
var relayPass = strings.Repeat("b", 64)
var relayToken = strings.Repeat("c", 64)
var relayGuest = strings.Repeat("d", 64)

type playbackLedger struct {
	mu          sync.Mutex
	pass        bridge.MediaPass
	revoked     bool
	activations int
}

func (l *playbackLedger) ResolveConnector(_ context.Context, hash string) (string, error) {
	want, _ := bridge.Hash(relayToken)
	l.mu.Lock()
	defer l.mu.Unlock()
	if hash != want || l.revoked {
		return "", bridge.ErrUnavailable
	}
	return "business", nil
}
func (l *playbackLedger) ResolveGuestMediaPass(_ context.Context, pub, pass, hash string) (string, error) {
	want, _ := bridge.Hash(relayGuest)
	if pub != relayPublication || pass != relayPass || hash != want {
		return "", bridge.ErrPassDenied
	}
	return "business", nil
}
func (l *playbackLedger) PublicPublication(_ context.Context, id string) (bridge.Publication, error) {
	if id != relayPublication {
		return bridge.Publication{}, bridge.ErrPassDenied
	}
	return bridge.Publication{Design: bridge.MediaDesign{Libraries: []string{relayLibrary}}}, nil
}
func (l *playbackLedger) Connection(context.Context, string) (bridge.Connection, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	until := time.Now().Add(time.Hour)
	c := bridge.Connection{TokenExpires: &until}
	if l.revoked {
		now := time.Now()
		c.Revoked = &now
	}
	return c, nil
}
func (l *playbackLedger) MediaPass(context.Context, string, string, string) (bridge.MediaPass, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.pass, nil
}
func (l *playbackLedger) ActivateMediaPass(context.Context, string, string, string) (bridge.MediaPass, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pass.State != "paid" {
		return bridge.MediaPass{}, bridge.ErrPassDenied
	}
	if l.pass.ExpiresAt == nil {
		l.activations++
		now := time.Now()
		until := now.Add(time.Duration(l.pass.DurationSeconds) * time.Second)
		l.pass.ActivatedAt = &now
		l.pass.ExpiresAt = &until
	}
	return l.pass, nil
}
func (l *playbackLedger) AuthorizeMediaPass(ctx context.Context, bid, pass, hash string) (bridge.MediaPass, error) {
	p, _ := l.MediaPass(ctx, bid, pass, hash)
	if p.State != "paid" || p.ExpiresAt == nil || !p.ExpiresAt.After(time.Now()) {
		return p, bridge.ErrPassDenied
	}
	return p, nil
}

func TestCloudCatalogNeverStreamsMediaOrActivatesPass(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ledger := &playbackLedger{pass: bridge.MediaPass{ID: relayPass, State: "paid", DurationSeconds: 3600}}
	handler := NewMediaPlaybackHandler(ledger, true)
	app := gin.New()
	app.POST("/api/v1/media-connector/redeem", func(c *gin.Context) { c.JSON(200, gin.H{"connector_token": relayToken}) })
	app.POST("/api/v1/media-connector/relay/poll", handler.Poll)
	app.POST("/api/v1/media-connector/relay/:task/check", handler.Check)
	app.GET("/api/v1/media-connector/relay/:task/socket", handler.Socket)
	base := "/api/v1/media-playback/:id/:passId/"
	app.POST(base+"session", handler.Session)
	app.GET(base+"catalog/:library", handler.Catalog)
	app.GET(base+"stream/:library/:item", handler.Stream)
	cloud := httptest.NewServer(app)
	defer cloud.Close()
	const media = "0123456789abcdef"
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/System/Info/Public" && !strings.Contains(r.Header.Get("Authorization"), "local-only-fixture-key") {
			t.Error("local key missing")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/System/Info/Public":
			fmt.Fprint(w, `{"ServerName":"Fixture PC","StartupWizardCompleted":true}`)
		case "/Library/VirtualFolders":
			fmt.Fprintf(w, `[{"Name":"Fixture videos","ItemId":%q}]`, relayLibrary)
		case "/Items":
			fmt.Fprintf(w, `{"Items":[{"Id":%q,"Name":"Fixture title","Type":"Movie","RunTimeTicks":120000000}]}`, relayItem)
		case "/Items/" + relayItem + "/Ancestors":
			fmt.Fprintf(w, `[{"Id":%q}]`, relayLibrary)
		case "/Items/" + relayItem:
			fmt.Fprintf(w, `{"Id":%q,"Name":"Fixture title","Type":"Movie"}`, relayItem)
		case "/Videos/" + relayItem + "/stream":
			w.Header().Set("Content-Type", "video/mp4")
			if r.Header.Get("Range") == "bytes=0-" {
				w.Header().Set("Content-Length", "16")
				w.Header().Set("Content-Range", "bytes 0-15/16")
				w.WriteHeader(206)
				for i := range media {
					select {
					case <-r.Context().Done():
						return
					case <-time.After(100 * time.Millisecond):
					}
					fmt.Fprint(w, media[i:i+1])
					w.(http.Flusher).Flush()
				}
			} else if r.Header.Get("Range") == "bytes=4-7" {
				w.Header().Set("Content-Length", "4")
				w.Header().Set("Content-Range", "bytes 4-7/16")
				w.WriteHeader(206)
				fmt.Fprint(w, media[4:8])
			} else {
				w.Header().Set("Content-Length", "16")
				fmt.Fprint(w, media)
			}
		default:
			w.WriteHeader(404)
		}
	}))
	defer local.Close()
	jelly, e := jellyfin.NewLocal(local.URL, "local-only-fixture-key")
	if e != nil {
		t.Fatal(e)
	}
	defer jelly.Close()
	pc, e := connector.New(cloud.URL, jelly, false)
	if e != nil {
		t.Fatal(e)
	}
	defer pc.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if e = pc.Pair(ctx, strings.Repeat("e", 64)); e != nil {
		t.Fatal(e)
	}
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for ctx.Err() == nil {
			_ = pc.RelayStep(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()
	defer func() { cancel(); workers.Wait() }()
	jar, _ := cookiejar.New(nil)
	guest := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	path := cloud.URL + "/api/v1/media-playback/" + relayPublication + "/" + relayPass + "/"
	request := func(method, url, authorization, byteRange string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, url, nil)
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		if byteRange != "" {
			req.Header.Set("Range", byteRange)
		}
		response, e := guest.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		return response
	}
	// Owner/staff credentials cannot substitute for the guest's proof.
	response := request("POST", path+"session", "Bearer staff-session", "")
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("staff proof accepted")
	}
	response = request("POST", path+"session", "Bearer "+relayGuest, "")
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("session: %d", response.StatusCode)
	}
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || strings.Contains(cookies[0].Value, relayGuest) {
		t.Fatal("unsafe player session cookie")
	}
	if !handler.Available(ctx, "business", relayPublication) {
		t.Fatal("outbound PC catalog unavailable")
	}
	response = request("GET", path+"catalog/"+relayLibrary, "", "")
	var catalog struct{ Items []jellyfin.CatalogItem }
	json.NewDecoder(response.Body).Decode(&catalog)
	response.Body.Close()
	if response.StatusCode != 200 || len(catalog.Items) != 1 {
		t.Fatalf("catalog: %d %#v", response.StatusCode, catalog)
	}
	ledger.mu.Lock()
	activated := ledger.activations
	ledger.mu.Unlock()
	if activated != 0 {
		t.Fatal("browsing spent paid time")
	}
	response = request("GET", path+"stream/"+strings.Repeat("3", 32)+"/"+relayItem, "", "")
	response.Body.Close()
	if response.StatusCode != 410 {
		t.Fatal("unselected library accepted")
	}
	for _, seek := range []string{"", "bytes=4-7"} {
		response = request("GET", path+"stream/"+relayLibrary+"/"+relayItem, "", seek)
		data, e := io.ReadAll(response.Body)
		response.Body.Close()
		if e != nil {
			t.Fatal(e)
		}
		if response.StatusCode != 410 || !strings.Contains(string(data), "local_playback_required") || strings.HasPrefix(response.Header.Get("Content-Type"), "video/") {
			t.Fatalf("cloud must not stream: %d %q", response.StatusCode, data)
		}
		if strings.Contains(string(data), "local-only-fixture-key") {
			t.Fatal("local key leaked")
		}
	}
	ledger.mu.Lock()
	if ledger.activations != 0 {
		t.Error("cloud request consumed paid time")
	}
	ledger.pass.State = "refunded"
	ledger.mu.Unlock()
	response = request("GET", path+"stream/"+relayLibrary+"/"+relayItem, "", "")
	response.Body.Close()
	if response.StatusCode != 410 {
		t.Fatal("refunded pass streamed")
	}
	ledger.mu.Lock()
	ledger.pass.State = "paid"
	expires := time.Now().Add(500 * time.Millisecond)
	ledger.pass.ExpiresAt = &expires
	ledger.mu.Unlock()
	began := time.Now()
	response = request("GET", path+"stream/"+relayLibrary+"/"+relayItem, "", "bytes=0-")
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 410 || err != nil || time.Since(began) > 2*time.Second {
		t.Fatalf("cloud stream was not refused: %d %q %v", response.StatusCode, data, err)
	}
	response = request("GET", path+"stream/"+relayLibrary+"/"+relayItem, "", "")
	response.Body.Close()
	if response.StatusCode != 410 {
		t.Fatal("expired pass restarted")
	}
	// A guest cookie is publication/pass scoped, including API restarts.
	response = request("GET", cloud.URL+"/api/v1/media-playback/"+strings.Repeat("f", 64)+"/"+relayPass+"/catalog/"+relayLibrary, "", "")
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("cross-publication cookie accepted")
	}
}
