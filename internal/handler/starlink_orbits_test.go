package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type memoryOrbitStore struct {
	mu sync.Mutex
	kv map[string]string
}

func (m *memoryOrbitStore) Get(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.kv[key]; ok {
		return v, nil
	}
	return "", errors.New("missing")
}

func (m *memoryOrbitStore) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.kv == nil {
		m.kv = map[string]string{}
	}
	switch v := value.(type) {
	case []byte:
		m.kv[key] = string(v)
	case string:
		m.kv[key] = v
	default:
		return fmt.Errorf("unexpected %T", value)
	}
	return nil
}

func celestrakBody(objects int) []byte {
	rows := make([]map[string]any, objects)
	for i := range rows {
		rows[i] = map[string]any{"OBJECT_NAME": fmt.Sprintf("STARLINK-%d", i), "NORAD_CAT_ID": 44000 + i, "EPOCH": "2026-11-05T12:00:00.123456", "MEAN_MOTION": 15.06, "ECCENTRICITY": 0.0001, "INCLINATION": 53.0, "RA_OF_ASC_NODE": 10.0, "ARG_OF_PERICENTER": 90.0, "MEAN_ANOMALY": 20.0, "BSTAR": 0.0002, "MEAN_MOTION_DOT": 0.00001, "MEAN_MOTION_DDOT": 0}
	}
	body, _ := json.Marshal(rows)
	return body
}

// clock is a test time source that the refresh goroutine can read safely.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func orbitsFixture(t *testing.T, h http.HandlerFunc, store StarlinkOrbitStore) (*StarlinkOrbits, *clock) {
	t.Helper()
	upstream := httptest.NewServer(h)
	t.Cleanup(upstream.Close)
	c := &clock{t: time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)}
	s := NewStarlinkOrbits(store)
	s.url, s.now = upstream.URL, c.now
	return s, c
}

func getOrbits(s *StarlinkOrbits, headers map[string]string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/map/starlink", s.Serve)
	req := httptest.NewRequest(http.MethodGet, "/map/starlink", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// waitIdle lets a background refresh finish, so its result can be observed.
func waitIdle(s *StarlinkOrbits) {
	s.mu.Lock()
	wait := s.inflight
	s.mu.Unlock()
	if wait != nil {
		<-wait
	}
}

func TestStarlinkOrbitsServesCompactRowsAndCachesDownload(t *testing.T) {
	var calls atomic.Int32
	s, _ := orbitsFixture(t, func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.Write(celestrakBody(2000)) }, nil)

	w := getOrbits(s, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got struct {
		Source    string  `json:"source"`
		FetchedAt string  `json:"fetched_at"`
		Elements  [][]any `json:"elements"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Source != "CelesTrak" || got.FetchedAt != "2026-11-06T00:00:00Z" || len(got.Elements) != 2000 || len(got.Elements[0]) != 11 {
		t.Fatalf("unexpected payload: %s %s %d", got.Source, got.FetchedAt, len(got.Elements))
	}
	if strings.Contains(w.Body.String(), "OBJECT_NAME") {
		t.Fatal("source fields should not be passed through")
	}
	for range 3 {
		getOrbits(s, nil)
	}
	if calls.Load() != 1 {
		t.Fatalf("downloads = %d, want 1", calls.Load())
	}
}

func TestStarlinkOrbitsGzipAndETag(t *testing.T) {
	s, _ := orbitsFixture(t, func(w http.ResponseWriter, _ *http.Request) { w.Write(celestrakBody(2000)) }, nil)
	w := getOrbits(s, map[string]string{"Accept-Encoding": "gzip, br"})
	if w.Header().Get("Content-Encoding") != "gzip" || w.Header().Get("ETag") == "" {
		t.Fatalf("headers: %v", w.Header())
	}
	zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if plain, err := io.ReadAll(zr); err != nil || !json.Valid(plain) {
		t.Fatalf("not gzipped JSON: %v", err)
	}
	again := getOrbits(s, map[string]string{"If-None-Match": w.Header().Get("ETag")})
	if again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Fatalf("revalidation: %d with %d bytes", again.Code, again.Body.Len())
	}
}

func TestStarlinkOrbitsStopsAfterAnError(t *testing.T) {
	for _, status := range []int{http.StatusMovedPermanently, http.StatusForbidden, http.StatusNotFound, http.StatusBadGateway} {
		var calls atomic.Int32
		s, c := orbitsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.Header().Set("Location", "https://celestrak.org/elsewhere")
			w.WriteHeader(status)
		}, nil)
		for range 5 {
			if w := getOrbits(s, nil); w.Code != http.StatusServiceUnavailable {
				t.Fatalf("%d: status %d", status, w.Code)
			}
		}
		c.add(starlinkRefreshEvery - time.Minute)
		getOrbits(s, nil)
		if calls.Load() != 1 {
			t.Fatalf("%d: attempts = %d, want 1 until the interval passes", status, calls.Load())
		}
		c.add(2 * time.Minute)
		getOrbits(s, nil)
		if calls.Load() != 2 {
			t.Fatalf("%d: attempts = %d, want 2 after the interval", status, calls.Load())
		}
	}
}

func TestStarlinkOrbitsKeepsGoodDataWhenARefreshFails(t *testing.T) {
	var fail atomic.Bool
	s, c := orbitsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Write(celestrakBody(2000))
	}, nil)
	first := getOrbits(s, nil)
	fail.Store(true)
	c.add(starlinkRefreshEvery + time.Minute)
	second := getOrbits(s, nil)
	waitIdle(s)
	if second.Code != http.StatusOK || second.Header().Get("ETag") != first.Header().Get("ETag") {
		t.Fatalf("status %d, etag changed: %v", second.Code, second.Header().Get("ETag") != first.Header().Get("ETag"))
	}
}

func TestStarlinkOrbitsRejectsBadResponses(t *testing.T) {
	bodies := map[string][]byte{
		"text":      []byte("GP data has not updated since your last successful download"),
		"too few":   celestrakBody(starlinkMinObjects - 1),
		"truncated": celestrakBody(2000)[:5000],
	}
	for name, body := range bodies {
		s, _ := orbitsFixture(t, func(w http.ResponseWriter, _ *http.Request) { w.Write(body) }, nil)
		if w := getOrbits(s, nil); w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status %d", name, w.Code)
		}
	}
}

func TestStarlinkOrbitsDropsInvalidRows(t *testing.T) {
	good := celestrakGP{NoradID: 1, Epoch: "2026-11-05T12:00:00.123456", MeanMotion: 15, Ecc: 0.001}
	objects := make([]celestrakGP, 0, starlinkMinObjects+3)
	for range starlinkMinObjects {
		objects = append(objects, good)
	}
	objects = append(objects, celestrakGP{NoradID: 2, Epoch: "soon", MeanMotion: 15}, celestrakGP{NoradID: 3, Epoch: good.Epoch, MeanMotion: 0}, celestrakGP{NoradID: 4, Epoch: good.Epoch, MeanMotion: 15, Ecc: 1.2})
	snap, err := buildStarlinkSnapshot(objects, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := gunzipStarlink(snap.Gzip)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Elements [][]any `json:"elements"`
	}
	if err := json.Unmarshal(plain, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Elements) != starlinkMinObjects {
		t.Fatalf("kept %d rows", len(got.Elements))
	}
}

func TestStarlinkOrbitsSurvivesARestartWithoutDownloading(t *testing.T) {
	var calls atomic.Int32
	store := &memoryOrbitStore{}
	h := func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.Write(celestrakBody(2000)) }
	first, c := orbitsFixture(t, h, store)
	want := getOrbits(first, nil)

	// A new process, an hour later.
	c.add(time.Hour)
	second, _ := orbitsFixture(t, h, store)
	second.now = c.now
	got := getOrbits(second, nil)
	if got.Code != http.StatusOK || got.Header().Get("ETag") != want.Header().Get("ETag") {
		t.Fatalf("status %d, ETag %q want %q", got.Code, got.Header().Get("ETag"), want.Header().Get("ETag"))
	}
	if calls.Load() != 1 {
		t.Fatalf("downloads = %d, want 1", calls.Load())
	}
}

func TestStarlinkOrbitsRestartHonoursAFailedAttempt(t *testing.T) {
	var calls atomic.Int32
	store := &memoryOrbitStore{}
	h := func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(http.StatusForbidden) }
	first, c := orbitsFixture(t, h, store)
	getOrbits(first, nil)

	second, _ := orbitsFixture(t, h, store)
	second.now = c.now
	if w := getOrbits(second, nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", w.Code)
	}
	if calls.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", calls.Load())
	}
}

func TestStarlinkOrbitsWaitsForRefreshWhenDataIsVeryOld(t *testing.T) {
	var calls atomic.Int32
	s, c := orbitsFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Write(celestrakBody(2000 + int(calls.Load())))
	}, nil)
	first := getOrbits(s, nil)
	c.add(starlinkWaitWhenOlder + time.Hour)
	second := getOrbits(s, nil)
	if second.Code != http.StatusOK || second.Header().Get("ETag") == first.Header().Get("ETag") {
		t.Fatal("very old data should be replaced before answering")
	}
}
