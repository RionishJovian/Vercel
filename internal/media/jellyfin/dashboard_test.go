package jellyfin

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDashboardReadOnlyAndPrivacy(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || strings.Contains(r.URL.String(), "secret") {
			t.Error("unsafe dashboard request")
		}
		if r.URL.Path != "/System/Info/Public" && r.Header.Get("Authorization") != `MediaBrowser Token="secret"` {
			t.Error("missing credential")
		}
		switch r.URL.Path {
		case "/System/Info/Public":
			w.Write([]byte(`{"ServerName":"Test","Version":"12","StartupWizardCompleted":true}`))
		case "/System/Info":
			w.Write([]byte(`{"OperatingSystem":"Linux","ApiKey":"must-not-relay","ProgramDataPath":"/private"}`))
		case "/Items/Counts":
			w.Write([]byte(`{"MovieCount":0,"SeriesCount":5,"SongCount":12}`))
		case "/Sessions":
			if r.URL.Query().Get("activeWithinSeconds") != "120" {
				t.Error("unbounded session window")
			}
			w.Write([]byte(`[{"UserId":"user1","UserName":"Guest","DeviceName":"Phone","RemoteEndPoint":"secret-ip","AccessToken":"must-not-relay","NowPlayingItem":{"Name":"Movie","Path":"/private"},"PlayState":{"PlayMethod":"Transcode","IsPaused":true},"TranscodingInfo":{"CompletionPercentage":42}},{"UserId":"user1","DeviceName":"TV"}]`))
		case "/Library/VirtualFolders":
			w.Write([]byte(`[]`))
		default:
			t.Error("unexpected endpoint")
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	c, _ := NewLocal(s.URL, "secret")
	defer c.Close()
	d, e := c.Dashboard(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if d.Counts == nil || d.Counts.Movies == nil || *d.Counts.Movies != 0 || d.Counts.Albums != nil {
		t.Fatal("lost zero or invented absent count")
	}
	if d.ActiveSessions == nil || *d.ActiveSessions != 2 || d.ActiveUsers == nil || *d.ActiveUsers != 1 || len(d.Playback) != 1 || !d.Playback[0].Paused {
		t.Fatal("incorrect session aggregation")
	}
	b, _ := json.Marshal(d)
	for _, secret := range []string{"must-not-relay", "secret-ip", "/private", "user1"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("relayed private upstream fields")
		}
	}
	if e = ValidateDashboard(d); e != nil {
		t.Fatal(e)
	}
}
func TestDashboardUnavailableNotZero(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/System/Info/Public" {
			w.Write([]byte(`{"ServerName":"PC"}`))
		} else {
			w.WriteHeader(403)
		}
	}))
	defer s.Close()
	c, _ := NewLocal(s.URL, "key")
	defer c.Close()
	d, e := c.Dashboard(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if d.Counts != nil || d.ActiveUsers != nil || d.ActiveSessions != nil || len(d.Unavailable) != 4 {
		t.Fatal("failed reads became fake zero")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = c.Dashboard(ctx); e == nil {
		t.Fatal("ignored cancellation")
	}
}
func TestDashboardBounds(t *testing.T) {
	d := Dashboard{}
	if ValidateDashboard(d) == nil {
		t.Fatal("accepted undated snapshot")
	}
	for _, tc := range []struct {
		path, mount string
		win, want   bool
	}{
		{"/media/videos", "/media", false, true}, {"/mediabox", "/media", false, false}, {"relative", "/", false, false}, {"/Users/me/Movies", "/", false, true},
		{`C:\Media\Videos`, `c:\`, true, true}, {`\\server\videos`, `C:\`, true, false}, {"//server/videos", "/", false, false},
	} {
		if withinMount(tc.path, tc.mount, tc.win) != tc.want {
			t.Errorf("mount mismatch %+v", tc)
		}
	}
	if localFilesystem("nfs") || localFilesystem("smbfs") || !localFilesystem("apfs") {
		t.Fatal("filesystem allow-list incorrect")
	}
	value := math.NaN()
	if ValidateDashboard(Dashboard{CollectedAt: time.Now(), Host: &HostMetrics{CPU: &value}}) == nil {
		t.Fatal("accepted invalid percentage")
	}
}
