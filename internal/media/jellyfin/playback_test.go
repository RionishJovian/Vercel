package jellyfin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const fixtureLibrary = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const fixtureItem = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestLocalCatalogAndRestrictedStream(t *testing.T) {
	var forbidden atomic.Bool
	var streams atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), "local-secret") || strings.Contains(r.URL.String(), "local-secret") {
			t.Error("credential was missing or in URL")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/Items":
			if r.URL.Query().Get("ParentId") != fixtureLibrary || r.URL.Query().Get("Limit") != "100" || r.URL.Query().Get("EnableUserData") != "false" {
				t.Error("catalog was not bounded/scoped")
			}
			io.WriteString(w, `{"Items":[{"Id":"`+fixtureItem+`","Name":"Synthetic test","Type":"Video","RunTimeTicks":120000000,"Path":"/PRIVATE/SECRET","ApiKey":"LEAK"}]}`)
		case "/Items/" + fixtureItem + "/Ancestors":
			if forbidden.Load() {
				io.WriteString(w, `[{"Id":"cccccccccccccccccccccccccccccccc"}]`)
			} else {
				io.WriteString(w, `[{"Id":"`+fixtureLibrary+`"}]`)
			}
		case "/Items/" + fixtureItem:
			io.WriteString(w, `{"Id":"`+fixtureItem+`","Name":"Synthetic test","Type":"Video","RunTimeTicks":120000000}`)
		case "/Videos/" + fixtureItem + "/stream":
			streams.Add(1)
			if r.URL.Query().Get("Static") != "true" {
				t.Error("unexpected stream options")
			}
			w.Header().Set("Content-Type", "video/mp4")
			if r.Header.Get("Range") == "bytes=2-5" {
				w.Header().Set("Content-Range", "bytes 2-5/8")
				w.Header().Set("Content-Length", "4")
				w.WriteHeader(206)
				io.WriteString(w, "2345")
			} else {
				w.Header().Set("Content-Length", "8")
				io.WriteString(w, "01234567")
			}
		default:
			t.Error("unexpected API path", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, e := NewLocal(server.URL, "local-secret")
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	items, e := client.Catalog(context.Background(), fixtureLibrary, 0)
	if e != nil || len(items) != 1 {
		t.Fatal("catalog failed", e)
	}
	raw, _ := json.Marshal(items)
	if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "LEAK") {
		t.Fatal("catalog leaked fields")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stream, e := client.OpenMedia(ctx, fixtureLibrary, fixtureItem, "bytes=2-5")
	if e != nil {
		t.Fatal(e)
	}
	data, e := io.ReadAll(stream.Body)
	stream.Body.Close()
	if e != nil || string(data) != "2345" || stream.Status != 206 || stream.ContentLength != 4 {
		t.Fatal("seekable stream failed", e)
	}
	forbidden.Store(true)
	if _, e = client.OpenMedia(ctx, fixtureLibrary, fixtureItem, ""); e == nil {
		t.Fatal("cross-library media streamed")
	}
	if streams.Load() != 1 {
		t.Fatal("unapproved source was opened")
	}
}

func TestLocalStreamValidationAndRedirects(t *testing.T) {
	for _, tt := range []struct {
		requested, returned string
		match               bool
	}{
		{"bytes=2-5", "bytes 2-5/8", true},
		{"bytes=2-5", "bytes 0-3/8", false},
		{"bytes=2-", "bytes 2-7/8", true},
		{"bytes=2-100", "bytes 2-7/8", true},
		{"bytes=-3", "bytes 5-7/8", true},
		{"bytes=-100", "bytes 0-7/8", true},
		{"bytes=-3", "bytes 0-2/8", false},
		{"", "bytes 0-7/8", false},
		{"bytes=8-", "bytes 7-7/8", false},
	} {
		if matchesRequestedRange(tt.requested, tt.returned) != tt.match {
			t.Fatalf("unexpected seek validation: %q -> %q", tt.requested, tt.returned)
		}
	}
	for _, s := range []string{"bytes=", "bytes=-", "bytes=0-1,3-4", "bytes=4-1", "bytes=-0", "bytes=0-9223372036854775808", "bytes=0-1\r\nX: evil"} {
		if validRange(s) {
			t.Fatal("invalid range accepted", s)
		}
	}
	for _, s := range []string{"", "bytes=0-", "bytes=-100", "bytes=2-5"} {
		if !validRange(s) {
			t.Fatal("valid range refused", s)
		}
	}
	for _, s := range []string{"bytes 2-5/5", "bytes 2-4/8", "bytes 5-2/8", "bytes */8"} {
		if validContentRange(s, 4) {
			t.Fatal("invalid upstream range accepted")
		}
	}
	var leaked atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/Ancestors") {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `[{"Id":"`+fixtureLibrary+`"}]`)
			return
		}
		if r.URL.Path == "/Items/"+fixtureItem {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"Id":"`+fixtureItem+`","Name":"Test","Type":"Video"}`)
			return
		}
		http.Redirect(w, r, destination.URL, 302)
	}))
	defer source.Close()
	client, _ := NewLocal(source.URL, "local-secret")
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, e := client.OpenMedia(ctx, fixtureLibrary, fixtureItem, ""); e == nil {
		t.Fatal("stream redirect accepted")
	}
	if leaked.Load() {
		t.Fatal("credential-bearing redirect followed")
	}
	for _, bad := range []struct{ library, item, rangeHeader string }{{"http://evil.test", fixtureItem, ""}, {fixtureLibrary, "../secret", ""}, {fixtureLibrary, fixtureItem, "bytes=-0"}} {
		if _, e := client.OpenMedia(ctx, bad.library, bad.item, bad.rangeHeader); e == nil {
			t.Fatal("invalid stream request accepted")
		}
	}
	if _, e := client.OpenMedia(context.Background(), fixtureLibrary, fixtureItem, ""); e == nil {
		t.Fatal("unbounded stream deadline accepted")
	}
}
