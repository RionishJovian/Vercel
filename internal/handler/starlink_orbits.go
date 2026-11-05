package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	starlinkSourceURL = "https://celestrak.org/NORAD/elements/gp.php?GROUP=starlink&FORMAT=json"
	// CelesTrak allows one download per update (about every 2 hours), answers a
	// repeat with 403, firewalls an IP after 50 errors in 2 hours, and asks
	// automated clients to stop on any non-200. A failed attempt therefore waits
	// the same full interval as a successful one, and is never retried sooner.
	starlinkRefreshEvery = 6 * time.Hour
	// Past this age a request waits for the refresh instead of receiving old data first.
	starlinkWaitWhenOlder = 48 * time.Hour
	starlinkFetchTimeout  = 45 * time.Second
	starlinkMaxBody       = 32 << 20
	// A real catalog holds thousands of objects. Fewer means a partial or wrong response.
	starlinkMinObjects  = 1000
	starlinkStoreTTL    = 7 * 24 * time.Hour
	starlinkStoreKey    = "mm:starlink:orbits"
	starlinkNotBeforeK  = "mm:starlink:not_before"
	starlinkEpochLayout = "2006-01-02T15:04:05.999999"
)

// StarlinkOrbitStore keeps the download across restarts, so a redeploy does not
// repeat a download CelesTrak would refuse. The Redis cache client satisfies it.
type StarlinkOrbitStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
}

// StarlinkOrbits serves public orbit data for the map's calculated satellite
// positions. It holds no tenant data and says nothing about any site's internet.
type StarlinkOrbits struct {
	url    string
	client *http.Client
	store  StarlinkOrbitStore
	now    func() time.Time

	mu          sync.Mutex
	restored    bool
	current     *starlinkSnapshot
	nextAttempt time.Time
	inflight    chan struct{}
}

type starlinkSnapshot struct {
	Gzip      []byte    `json:"gzip"`
	FetchedAt time.Time `json:"fetched_at"`
	etag      string
}

func NewStarlinkOrbits(store StarlinkOrbitStore) *StarlinkOrbits {
	return &StarlinkOrbits{
		url:   starlinkSourceURL,
		store: store,
		now:   time.Now,
		client: &http.Client{
			Timeout: starlinkFetchTimeout,
			// A redirect is a non-200 to stop on, not something to follow.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Serve is the one route. The download only ever starts from a real request, so
// nothing is fetched unless someone has turned the layer on.
func (s *StarlinkOrbits) Serve(c *gin.Context) {
	snap := s.snapshot(c.Request.Context())
	if snap == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "starlink_orbits_unavailable"})
		return
	}
	c.Header("Cache-Control", "private, no-cache")
	c.Header("ETag", snap.etag)
	c.Header("Vary", "Accept-Encoding")
	if c.GetHeader("If-None-Match") == snap.etag {
		c.Status(http.StatusNotModified)
		return
	}
	if strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
		c.Header("Content-Encoding", "gzip")
		c.Data(http.StatusOK, "application/json", snap.Gzip)
		return
	}
	plain, err := gunzipStarlink(snap.Gzip)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "starlink_orbits_unavailable"})
		return
	}
	c.Data(http.StatusOK, "application/json", plain)
}

func (s *StarlinkOrbits) snapshot(ctx context.Context) *starlinkSnapshot {
	s.mu.Lock()
	if !s.restored {
		s.restored = true
		s.restore(ctx)
	}
	now := s.now()
	if s.inflight == nil && !now.Before(s.nextAttempt) {
		// Recorded before the request goes out, so a crash during it cannot cause a repeat.
		s.nextAttempt = now.Add(starlinkRefreshEvery)
		s.saveNotBefore(s.nextAttempt)
		done := make(chan struct{})
		s.inflight = done
		go s.refresh(done)
	}
	current, wait := s.current, s.inflight
	s.mu.Unlock()

	if wait != nil && (current == nil || now.Sub(current.FetchedAt) > starlinkWaitWhenOlder) {
		select {
		case <-wait:
		case <-ctx.Done():
		}
		s.mu.Lock()
		current = s.current
		s.mu.Unlock()
	}
	return current
}

func (s *StarlinkOrbits) refresh(done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), starlinkFetchTimeout)
	defer cancel()
	snap, err := s.download(ctx)
	if err != nil {
		log.Printf("starlink orbits: download failed, not retrying for %s: %v", starlinkRefreshEvery, err)
	} else {
		s.save(snap)
	}
	s.mu.Lock()
	if err == nil {
		s.current = snap
	}
	s.inflight = nil
	s.mu.Unlock()
	close(done)
}

// celestrakGP lists only the fields the browser's SGP4 needs.
type celestrakGP struct {
	NoradID     int     `json:"NORAD_CAT_ID"`
	Epoch       string  `json:"EPOCH"`
	MeanMotion  float64 `json:"MEAN_MOTION"`
	Ecc         float64 `json:"ECCENTRICITY"`
	Inclination float64 `json:"INCLINATION"`
	RAAN        float64 `json:"RA_OF_ASC_NODE"`
	ArgPerigee  float64 `json:"ARG_OF_PERICENTER"`
	MeanAnomaly float64 `json:"MEAN_ANOMALY"`
	BStar       float64 `json:"BSTAR"`
	MeanMotDot  float64 `json:"MEAN_MOTION_DOT"`
	MeanMotDDot float64 `json:"MEAN_MOTION_DDOT"`
}

func (s *StarlinkOrbits) download(ctx context.Context) (*starlinkSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MegaMagic-orbit-cache/1.0")
	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	var objects []celestrakGP
	if err := json.NewDecoder(io.LimitReader(res.Body, starlinkMaxBody)).Decode(&objects); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return buildStarlinkSnapshot(objects, s.now())
}

// buildStarlinkSnapshot keeps one compact row per valid object and only the
// gzipped result, which is about an eighth of the size of the source.
func buildStarlinkSnapshot(objects []celestrakGP, fetchedAt time.Time) (*starlinkSnapshot, error) {
	rows := make([][]any, 0, len(objects))
	for _, o := range objects {
		if _, err := time.Parse(starlinkEpochLayout, o.Epoch); err != nil || o.NoradID <= 0 || o.MeanMotion <= 0 || o.MeanMotion > 20 || o.Ecc < 0 || o.Ecc >= 1 {
			continue
		}
		rows = append(rows, []any{o.NoradID, o.Epoch, o.MeanMotion, o.Ecc, o.Inclination, o.RAAN, o.ArgPerigee, o.MeanAnomaly, o.BStar, o.MeanMotDot, o.MeanMotDDot})
	}
	if len(rows) < starlinkMinObjects {
		return nil, fmt.Errorf("only %d usable objects of %d", len(rows), len(objects))
	}
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if err := json.NewEncoder(zw).Encode(map[string]any{"source": "CelesTrak", "source_url": "https://celestrak.org/", "fetched_at": fetchedAt.UTC().Format(time.RFC3339), "elements": rows}); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	snap := &starlinkSnapshot{Gzip: compressed.Bytes(), FetchedAt: fetchedAt}
	snap.setETag()
	return snap, nil
}

func (s *starlinkSnapshot) setETag() {
	sum := sha256.Sum256(s.Gzip)
	s.etag = `"` + hex.EncodeToString(sum[:12]) + `"`
}

func gunzipStarlink(compressed []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

func storeContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 5*time.Second)
}

// restore loads what an earlier process downloaded and its waiting time. The map
// layer is optional, so a missing or failing store only costs an extra download.
func (s *StarlinkOrbits) restore(parent context.Context) {
	if s.store == nil {
		return
	}
	ctx, cancel := storeContext(context.WithoutCancel(parent))
	defer cancel()
	if raw, err := s.store.Get(ctx, starlinkStoreKey); err == nil {
		var snap starlinkSnapshot
		if json.Unmarshal([]byte(raw), &snap) == nil && len(snap.Gzip) > 0 && !snap.FetchedAt.IsZero() {
			snap.setETag()
			s.current = &snap
			s.nextAttempt = snap.FetchedAt.Add(starlinkRefreshEvery)
		}
	}
	if raw, err := s.store.Get(ctx, starlinkNotBeforeK); err == nil {
		if t, err := time.Parse(time.RFC3339, raw); err == nil && t.After(s.nextAttempt) {
			s.nextAttempt = t
		}
	}
}

func (s *StarlinkOrbits) save(snap *starlinkSnapshot) {
	if s.store == nil {
		return
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return
	}
	ctx, cancel := storeContext(context.Background())
	defer cancel()
	if err := s.store.Set(ctx, starlinkStoreKey, raw, starlinkStoreTTL); err != nil {
		log.Printf("starlink orbits: store failed: %v", err)
	}
}

func (s *StarlinkOrbits) saveNotBefore(t time.Time) {
	if s.store == nil {
		return
	}
	ctx, cancel := storeContext(context.Background())
	defer cancel()
	if err := s.store.Set(ctx, starlinkNotBeforeK, t.UTC().Format(time.RFC3339), starlinkRefreshEvery); err != nil {
		log.Printf("starlink orbits: store failed: %v", err)
	}
}
