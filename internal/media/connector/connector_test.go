package connector

import (
	"context"
	"encoding/json"
	"errors"
	"mega-magic/internal/media/bridge"
	"mega-magic/internal/media/jellyfin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeLocal struct {
	writes int
	reads  int
	err    error
}

func (f *fakeLocal) Status(context.Context) (jellyfin.ServerInfo, error) {
	f.reads++
	return jellyfin.ServerInfo{ServerName: "PC", StartupWizardCompleted: true}, f.err
}
func (f *fakeLocal) Libraries(context.Context) ([]jellyfin.Library, error) {
	f.reads++
	return []jellyfin.Library{{Name: "Videos"}}, f.err
}
func (f *fakeLocal) AddLibrary(context.Context, jellyfin.NewLibrary) error { f.writes++; return f.err }
func (f *fakeLocal) Scan(context.Context) error                            { f.writes++; return f.err }
func (f *fakeLocal) Dashboard(context.Context) (jellyfin.Dashboard, error) {
	f.reads++
	return jellyfin.Dashboard{CollectedAt: time.Now(), Server: jellyfin.ServerInfo{ServerName: "PC"}}, f.err
}
func TestDashboardReadOnlyJob(t *testing.T) {
	f := &fakeLocal{}
	r, _ := New("https://mikromagic.com", f, false)
	defer r.Close()
	result := r.Execute(context.Background(), bridge.Job{Operation: "dashboard", Payload: json.RawMessage(`{}`), Expires: time.Now().Add(time.Minute)})
	if result.Dashboard == nil || result.ErrorCode != "" || f.writes != 0 || f.reads != 1 {
		t.Fatal("dashboard not read-only")
	}
}
func TestRoundTrip(t *testing.T) {
	token := strings.Repeat("a", 64)
	code := strings.Repeat("b", 64)
	id := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	polls, completions := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.RawQuery != "" {
			t.Error("secrets in URL")
		}
		switch req.URL.Path {
		case "/api/v1/media-connector/redeem":
			if req.Header.Get("Authorization") != "" {
				t.Error("unexpected credential")
			}
			var b map[string]string
			json.NewDecoder(req.Body).Decode(&b)
			if b["pairing_code"] != code {
				t.Error("wrong pairing code")
			}
			json.NewEncoder(w).Encode(map[string]string{"connector_token": token})
		case "/api/v1/media-connector/poll":
			polls++
			if req.Header.Get("Authorization") != "Bearer "+token {
				t.Error("missing token")
			}
			json.NewEncoder(w).Encode(bridge.Job{ID: id, Operation: "libraries", Payload: json.RawMessage(`{}`), Expires: time.Now().Add(time.Minute)})
		case "/api/v1/media-connector/jobs/" + id + "/result":
			completions++
			var result bridge.Completion
			json.NewDecoder(req.Body).Decode(&result)
			if len(result.Libraries) != 1 || result.Libraries[0].Name != "Videos" {
				t.Errorf("bad result %+v", result)
			}
			w.WriteHeader(204)
		default:
			t.Error("unexpected endpoint")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	local := &fakeLocal{}
	r, e := New(server.URL, local, false)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	if e = r.Pair(context.Background(), code); e != nil {
		t.Fatal(e)
	}
	if e = r.Step(context.Background()); e != nil {
		t.Fatal(e)
	}
	if polls != 1 || completions != 1 || local.writes != 0 {
		t.Fatal("unexpected retries or writes")
	}
}
func TestGuardedOperations(t *testing.T) {
	f := &fakeLocal{}
	r, _ := New("https://mikromagic.com", f, false)
	defer r.Close()
	for _, j := range []bridge.Job{
		{Operation: "shell", Payload: json.RawMessage(`{}`), Expires: time.Now().Add(time.Minute)},
		{Operation: "scan", Payload: json.RawMessage(`{"confirm":false}`), Expires: time.Now().Add(time.Minute)},
		{Operation: "scan", Payload: json.RawMessage(`{"confirm":true}`), Expires: time.Now().Add(time.Minute)},
		{Operation: "libraries", Payload: json.RawMessage(`{}`), Expires: time.Now().Add(-time.Minute)},
	} {
		if r.Execute(context.Background(), j).ErrorCode == "" {
			t.Fatal("unguarded operation")
		}
	}
	if f.writes != 0 || f.reads != 0 {
		t.Fatal("guard failure")
	}
	r.allowWrites = true
	if !r.Execute(context.Background(), bridge.Job{Operation: "scan", Payload: json.RawMessage(`{"confirm":true}`), Expires: time.Now().Add(time.Minute)}).Accepted || f.writes != 1 {
		t.Fatal("confirmed scan did not execute")
	}
	f.err = &jellyfin.APIError{Status: 403}
	if r.Execute(context.Background(), bridge.Job{Operation: "libraries", Expires: time.Now().Add(time.Minute)}).ErrorCode != "local_credential_denied" {
		t.Fatal("wrong safe error")
	}
}
func TestCloudSecurity(t *testing.T) {
	for _, u := range []string{"http://example.com", "https://user:key@example.com", "https://example.com/?key=secret", "https://example.com/path", "file:///tmp"} {
		if _, e := New(u, &fakeLocal{}, false); e == nil {
			t.Errorf("unsafe cloud %s", u)
		}
	}
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer server.Close()
	r, _ := New(server.URL, &fakeLocal{}, false)
	defer r.Close()
	r.token = strings.Repeat("a", 64)
	if e := r.Step(context.Background()); e == nil || strings.Contains(e.Error(), r.token) {
		t.Fatal("redirect or error unsafe")
	}
	if targetCalls != 0 {
		t.Fatal("followed redirect")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := r.Step(ctx); !errors.Is(e, context.Canceled) {
		t.Fatalf("lost cancel %v", e)
	}
}
