package jellyfin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLocalBoundary(t *testing.T) {
	for _, endpoint := range []string{"https://example.com:8096", "http://localhost:8096", "http://10.0.0.1:8096", "http://169.254.169.254:80", "http://user:secret@127.0.0.1:8096", "http://127.0.0.1:8096/?api_key=secret", "http://127.0.0.1:8096/web", "http://127.0.0.1", "file:///tmp/server", "http://127.0.0.1:8096/#fragment"} {
		if _, err := NewLocal(endpoint, "key"); err == nil {
			t.Errorf("accepted unsafe endpoint %s", endpoint)
		}
	}
	for _, endpoint := range []string{"http://127.0.0.1:8096", "http://[::1]:8096"} {
		c, err := NewLocal(endpoint, "key")
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
	}
	if _, err := NewLocal("http://127.0.0.1:8096", "key\r\nheader"); err == nil {
		t.Fatal("accepted header injection")
	}
}

func TestSupportedAuthorizationEncodesCredential(t *testing.T) {
	credential := `secret", Client="injected`
	requests := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.RawQuery != "" || r.Header.Get("X-Emby-Token") != "" {
			t.Error("credential used a legacy or URL mechanism")
		}
		header := r.Header.Get("Authorization")
		if header != `MediaBrowser Token="`+url.PathEscape(credential)+`"` {
			t.Error("credential was not encoded into a single quoted parameter")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer s.Close()
	c, err := NewLocal(s.URL, credential)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Libraries(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatal("authentication was retried")
	}
}

func TestLibraryRequests(t *testing.T) {
	var calls int
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("api_key") != "" {
			t.Error("credential in URL")
		}
		if r.Header.Get("X-Emby-Token") != "" {
			t.Error("deprecated credential header sent")
		}
		if r.URL.Path != "/System/Info/Public" && r.Header.Get("Authorization") != `MediaBrowser Token="private-key"` {
			t.Error("missing credential header")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /System/Info/Public":
			if r.Header.Get("Authorization") != "" {
				t.Error("credential sent on public probe")
			}
			w.Write([]byte(`{"ServerName":"Test","Version":"12.1.0","StartupWizardCompleted":true}`))
		case "GET /Library/VirtualFolders":
			w.Write([]byte(`[{"Name":"Home videos","Locations":["/media/videos"],"ItemId":"abc","CollectionType":"homevideos"}]`))
		case "POST /Library/VirtualFolders":
			if r.URL.Query().Get("name") != "My videos" || r.URL.Query().Get("refreshLibrary") != "false" || r.URL.Query().Get("collectionType") != "homevideos" {
				t.Error("wrong query")
			}
			var body struct {
				LibraryOptions struct{ PathInfos []struct{ Path string } }
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if len(body.LibraryOptions.PathInfos) != 2 || body.LibraryOptions.PathInfos[0].Path != "/media/family, friends" || body.LibraryOptions.PathInfos[1].Path != `C:\Media\Videos` {
				t.Errorf("bad paths: %+v", body)
			}
			w.WriteHeader(204)
		case "POST /Library/Refresh":
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	c, err := NewLocal(s.URL, "private-key")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	info, err := c.Status(context.Background())
	if err != nil || !info.StartupWizardCompleted {
		t.Fatalf("status: %+v %v", info, err)
	}
	items, err := c.Libraries(context.Background())
	if err != nil || len(items) != 1 || items[0].Name != "Home videos" {
		t.Fatalf("libraries: %+v %v", items, err)
	}
	if err := c.AddLibrary(context.Background(), NewLibrary{Name: "My videos", Kind: "homevideos", Paths: []string{"/media/family, friends", `C:\Media\Videos`}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 4 {
		t.Fatalf("unexpected retries: %d", calls)
	}
}

func TestValidationMakesNoRequests(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected request") }))
	defer s.Close()
	c, _ := NewLocal(s.URL, "key")
	defer c.Close()
	for _, library := range []NewLibrary{
		{Name: "..", Kind: "movies", Paths: []string{"/media"}},
		{Name: "../test", Kind: "movies", Paths: []string{"/media"}},
		{Name: "Movies", Kind: "invalid", Paths: []string{"/media"}},
		{Name: "Movies", Kind: "movies"},
		{Name: "Movies", Kind: "movies", Paths: []string{"relative/path"}},
		{Name: "Movies", Kind: "movies", Paths: []string{"/media\x00"}},
	} {
		if err := c.AddLibrary(context.Background(), library); err == nil {
			t.Errorf("accepted %+v", library)
		}
	}
	noKey, _ := NewLocal(s.URL, "")
	defer noKey.Close()
	if _, err := noKey.Libraries(context.Background()); err == nil {
		t.Fatal("allowed unauthenticated libraries")
	}
	if err := noKey.Scan(context.Background()); err == nil {
		t.Fatal("allowed unauthenticated scan")
	}
}

func TestErrorsAndRedirectsNeverLeak(t *testing.T) {
	var targetCalls int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	for _, status := range []int{401, 403, 409, 500, 302} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", target.URL)
			w.WriteHeader(status)
			w.Write([]byte("secret-token /Users/private"))
		}))
		c, _ := NewLocal(s.URL, "secret-token")
		_, err := c.Libraries(context.Background())
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != status || strings.Contains(err.Error(), "secret-token") || strings.Contains(err.Error(), "/Users") {
			t.Fatalf("unsafe error: %v", err)
		}
		c.Close()
		s.Close()
	}
	if targetCalls != 0 {
		t.Fatal("followed credential-bearing redirect")
	}
}

func TestMalformedOversizedAndEmpty(t *testing.T) {
	for _, body := range []string{"[]", "null", "bad-json", strings.Repeat("x", maxResponse+1)} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		c, _ := NewLocal(s.URL, "key")
		items, err := c.Libraries(context.Background())
		if body == "[]" || body == "null" {
			if err != nil || items == nil || len(items) != 0 {
				t.Fatalf("empty library is not an error: %v", err)
			}
		} else if err == nil {
			t.Fatal("accepted malformed/oversized response")
		}
		c.Close()
		s.Close()
	}
}

func TestCancelledContext(t *testing.T) {
	c, _ := NewLocal("http://127.0.0.1:8096", "")
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Status(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}
