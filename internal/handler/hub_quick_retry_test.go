package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/curve25519"
	"mega-magic/internal/auth"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
)

type retryEnrollmentHub struct {
	mu                    sync.Mutex
	peers                 map[string]hub.Peer
	keys                  []string
	failEnroll, committed bool
}

func (s *retryEnrollmentHub) InspectRouter(_ context.Context, id hub.Identity) (hub.Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	peer, ok := s.peers[id.TenantID+"/"+id.RouterID]
	if !ok {
		return hub.Peer{}, hub.ErrPeerNotFound
	}
	return peer, nil
}

func (s *retryEnrollmentHub) EnrollPeer(_ context.Context, id hub.Identity, key string, port int) (hub.Peer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = append(s.keys, key)
	peer := hub.Peer{PeerID: "peer", TenantID: id.TenantID, RouterID: id.RouterID,
		RouterPublicKey: key, HubPublicKey: base64.StdEncoding.EncodeToString([]byte(strings.Repeat("h", 32))),
		Endpoint: "hub.example.com:51820", AllowedIPs: []string{"10.77.0.9/32"},
		EnrollmentVersion: 1, State: "active", TunnelAddress: "10.77.0.2/32"}
	if existing, ok := s.peers[id.TenantID+"/"+id.RouterID]; ok && existing.RouterPublicKey != key {
		return hub.Peer{}, errors.New("existing key must not change")
	}
	if !s.failEnroll || s.committed {
		s.peers[id.TenantID+"/"+id.RouterID] = peer
	}
	if s.failEnroll {
		s.failEnroll = false
		return hub.Peer{}, errors.New("transient enrollment response failure")
	}
	return peer, nil
}

func quickRetryHandler(s *retryEnrollmentHub) *VPSHandler {
	return &VPSHandler{
		liveHub: s, enrollmentHub: s, livePlan: func(string) bool { return true },
		liveDevice: func(_ *gin.Context, biz, id string) (*model.Device, error) {
			return &model.Device{DeviceID: id, BusinessID: biz, DeploymentLayer: "MAIN_GATEWAY"}, nil
		},
		automaticPrepare: func(_ context.Context, _, _, _, script string) (string, error) { return script, nil },
	}
}

func requestQuickEnrollment(h *VPSHandler, biz string, resume ...bool) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"management_rest_port":443,"confirm_enrollment":true}`))
	if len(resume) > 0 && resume[0] {
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"management_rest_port":443,"confirm_enrollment":true,"resume_existing":true}`))
	}
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "deviceId", Value: "gateway"}}
	c.Set(auth.ContextKeyBusinessID, biz)
	c.Set(auth.ContextKeyUserID, "owner")
	c.Set(auth.ContextKeyUserRole, "OWNER")
	h.QuickEnrollGateway(c)
	return w
}

func assertReservedRouterKey(t *testing.T, w *httptest.ResponseRecorder, peer hub.Peer) {
	t.Helper()
	var body struct {
		Script   string `json:"router_script"`
		State    string `json:"state"`
		Verified bool   `json:"lan_reachability_verified"`
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.State != "hub_peer_enrolled" || body.Verified {
		t.Fatal("invalid or falsely verified enrollment result")
	}
	match := regexp.MustCompile(`private-key="([^"]+)"`).FindStringSubmatch(body.Script)
	if len(match) != 2 {
		t.Fatal("retry did not recover the one-paste installation artifact")
	}
	private, err := base64.StdEncoding.DecodeString(match[1])
	if err != nil {
		t.Fatal("invalid installation key")
	}
	public, err := curve25519.X25519(private, curve25519.Basepoint)
	if err != nil || base64.StdEncoding.EncodeToString(public) != peer.RouterPublicKey {
		t.Fatal("installation key does not match the already reserved tunnel")
	}
}

func TestQuickEnrollmentRetriesPreparationWithReservedKey(t *testing.T) {
	s := &retryEnrollmentHub{peers: map[string]hub.Peer{}}
	h := quickRetryHandler(s)
	prepare := 0
	h.automaticPrepare = func(_ context.Context, _, _, _, script string) (string, error) {
		prepare++
		if prepare == 1 {
			return "", errors.New("management preparation temporarily unavailable")
		}
		return script, nil
	}
	if w := requestQuickEnrollment(h, "tenant"); w.Code != 503 {
		t.Fatalf("first attempt status = %d, want 503", w.Code)
	}
	w := requestQuickEnrollment(h, "tenant")
	if w.Code != 200 {
		t.Fatalf("retry status = %d, want 200 without replacing the reserved key", w.Code)
	}
	if len(s.keys) != 1 {
		t.Fatal("retry tried to allocate another tunnel")
	}
	assertReservedRouterKey(t, w, s.peers["tenant/gateway"])
}

func TestQuickEnrollmentRetriesUncertainHubResponse(t *testing.T) {
	for _, committed := range []bool{false, true} {
		name := "before reservation"
		if committed {
			name = "after reservation"
		}
		t.Run(name, func(t *testing.T) {
			s := &retryEnrollmentHub{peers: map[string]hub.Peer{}, failEnroll: true, committed: committed}
			h := quickRetryHandler(s)
			if w := requestQuickEnrollment(h, "tenant"); w.Code != 502 {
				t.Fatalf("first attempt status = %d, want 502", w.Code)
			}
			w := requestQuickEnrollment(h, "tenant")
			if w.Code != 200 {
				t.Fatalf("retry status = %d, want 200", w.Code)
			}
			for _, key := range s.keys {
				if key != s.keys[0] {
					t.Fatal("retry generated a replacement tunnel key")
				}
			}
			assertReservedRouterKey(t, w, s.peers["tenant/gateway"])
		})
	}
}

func TestQuickEnrollmentConcurrentRetriesKeepOneKey(t *testing.T) {
	s := &retryEnrollmentHub{peers: map[string]hub.Peer{}}
	h := quickRetryHandler(s)
	responses := make([]*httptest.ResponseRecorder, 8)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range responses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			responses[i] = requestQuickEnrollment(h, "tenant")
		}(i)
	}
	close(start)
	wg.Wait()
	for _, w := range responses {
		if w.Code != 200 {
			t.Fatalf("concurrent retry status = %d, want 200", w.Code)
		}
		assertReservedRouterKey(t, w, s.peers["tenant/gateway"])
	}
	for _, key := range s.keys {
		if key != s.keys[0] {
			t.Fatal("concurrent requests proposed conflicting tunnel identities")
		}
	}
}

func TestQuickEnrollmentKeyExpiryPreservesExistingTunnel(t *testing.T) {
	s := &retryEnrollmentHub{peers: map[string]hub.Peer{}}
	h := quickRetryHandler(s)
	if w := requestQuickEnrollment(h, "tenant"); w.Code != 200 {
		t.Fatalf("initial enrollment status = %d", w.Code)
	}
	reserved := s.peers["tenant/gateway"].RouterPublicKey
	h.quickKeysMu.Lock()
	key := h.quickKeys["tenant/gateway"]
	key.expires = time.Now().Add(-time.Second)
	h.quickKeys["tenant/gateway"] = key
	h.quickKeysMu.Unlock()
	w := requestQuickEnrollment(h, "tenant")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "hub_router_key_unavailable") {
		t.Fatalf("expired artifact status = %d, want explicit recovery hold", w.Code)
	}
	if len(s.keys) != 1 || s.peers["tenant/gateway"].RouterPublicKey != reserved {
		t.Fatal("expired setup rotated or re-enrolled the existing tunnel")
	}
	w = requestQuickEnrollment(h, "tenant", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"existing_tunnel_required":true`) || !strings.Contains(w.Body.String(), `"one_paste":false`) {
		t.Fatalf("explicit installed-tunnel repair unavailable: %d %s", w.Code, w.Body.String())
	}
	var result struct {
		Script string `json:"router_script"`
	}
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || strings.Contains(result.Script, "private-key=") || !strings.Contains(result.Script, "router key differs") || !strings.Contains(result.Script, "interface ownership mismatch") {
		t.Fatal("repair installs a replacement identity or lacks existing-interface guards")
	}
	if len(s.keys) != 1 || s.peers["tenant/gateway"].RouterPublicKey != reserved {
		t.Fatal("explicit repair changed the reserved tunnel")
	}
}

func TestQuickRecoveryCannotCreateNewEnrollment(t *testing.T) {
	s := &retryEnrollmentHub{peers: map[string]hub.Peer{}}
	w := requestQuickEnrollment(quickRetryHandler(s), "tenant", true)
	if w.Code != 409 || len(s.keys) != 0 || len(s.peers) != 0 {
		t.Fatal("repair created a new enrollment", w.Code)
	}
}

func TestQuickEnrollmentRejectsForeignInspectedPeer(t *testing.T) {
	for _, field := range []string{"tenant", "router"} {
		t.Run(field, func(t *testing.T) {
			s := &retryEnrollmentHub{peers: map[string]hub.Peer{}}
			h := quickRetryHandler(s)
			if w := requestQuickEnrollment(h, "tenant"); w.Code != 200 {
				t.Fatal("initial enrollment failed")
			}
			peer := s.peers["tenant/gateway"]
			if field == "tenant" {
				peer.TenantID = "foreign"
			} else {
				peer.RouterID = "foreign"
			}
			s.peers["tenant/gateway"] = peer
			h.automaticPrepare = func(context.Context, string, string, string, string) (string, error) {
				t.Fatal("foreign Hub identity reached management preparation")
				return "", nil
			}
			if w := requestQuickEnrollment(h, "tenant"); w.Code != 502 || len(s.keys) != 1 {
				t.Fatalf("foreign identity status = %d, want 502 without enrollment", w.Code)
			}
		})
	}
}

func TestQuickEnrollmentKeyCacheIsTenantBounded(t *testing.T) {
	h := &VPSHandler{}
	a, err := h.quickEnrollmentKeys("tenant-a", "gateway", true)
	if err != nil {
		t.Fatal("first key preparation failed")
	}
	aRetry, err := h.quickEnrollmentKeys("tenant-a", "gateway", true)
	if err != nil || a != aRetry {
		t.Fatal("retry did not retain its original key and expiry")
	}
	b, err := h.quickEnrollmentKeys("tenant-b", "gateway", true)
	if err != nil || a.public == b.public || a.private == b.private {
		t.Fatal("key crossed a business boundary")
	}
	missing, err := h.quickEnrollmentKeys("tenant-c", "gateway", false)
	if err != nil || missing.public != "" || missing.private != "" {
		t.Fatal("lookup generated or disclosed a key")
	}
	for len(h.quickKeys) < maxPendingQuickEnrollments {
		h.quickKeys[fmt.Sprintf("reserved-%d", len(h.quickKeys))] = quickEnrollmentKey{expires: time.Now().Add(time.Minute)}
	}
	if _, err := h.quickEnrollmentKeys("new", "gateway", true); err == nil {
		t.Fatal("pending key memory bound was not enforced")
	}
	for id, key := range h.quickKeys {
		key.expires = time.Now().Add(-time.Second)
		h.quickKeys[id] = key
	}
	if _, err := h.quickEnrollmentKeys("new", "gateway", true); err != nil || len(h.quickKeys) != 1 {
		t.Fatal("expired attempts did not release capacity")
	}
}

func TestQuickEnrollmentPreflightPreventsPeerReservation(t *testing.T) {
	s := &retryEnrollmentHub{peers: map[string]hub.Peer{}}
	h := quickRetryHandler(s)
	h.ConfigureAutomaticPreflight(func(context.Context, string, string) error { return errors.New("missing deployment source") })
	if w := requestQuickEnrollment(h, "tenant"); w.Code != 503 {
		t.Fatalf("status %d", w.Code)
	}
	if len(s.keys) != 0 || len(s.peers) != 0 || len(h.quickKeys) != 0 {
		t.Fatal("failed preflight allocated installation state")
	}
}
