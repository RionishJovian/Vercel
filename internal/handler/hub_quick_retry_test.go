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

func requestQuickEnrollment(h *VPSHandler, biz string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"management_rest_port":443,"confirm_enrollment":true}`))
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

// removingHub is a Hub that can end an allocation.
type removingHub struct {
	*retryEnrollmentHub
	removed []string
}

func (s *removingHub) RemovePeer(_ context.Context, id hub.Identity, peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removed = append(s.removed, peerID)
	delete(s.peers, id.TenantID+"/"+id.RouterID)
	return nil
}

func expireQuickKey(h *VPSHandler) {
	h.quickKeysMu.Lock()
	defer h.quickKeysMu.Unlock()
	key := h.quickKeys["tenant/gateway"]
	key.expires = time.Now().Add(-time.Second)
	h.quickKeys["tenant/gateway"] = key
}

// A script not pasted before its setup key ended must not be a dead end: the router
// never connected, so it gets a fresh allocation and one script that works.
func TestQuickEnrollmentKeyExpiryStartsAgain(t *testing.T) {
	s := &removingHub{retryEnrollmentHub: &retryEnrollmentHub{peers: map[string]hub.Peer{}}}
	h := quickRetryHandler(s.retryEnrollmentHub)
	h.ConfigureHubEnrollment(s)
	h.automaticPreflight = func(context.Context, string, string) error { return nil }
	if w := requestQuickEnrollment(h, "tenant"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	first := s.peers["tenant/gateway"]
	expireQuickKey(h)
	w := requestQuickEnrollment(h, "tenant")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"one_paste":true`) {
		t.Fatalf("expired setup is a dead end: %d %s", w.Code, w.Body.String())
	}
	if len(s.removed) != 1 || s.removed[0] != first.PeerID {
		t.Fatalf("old allocation not replaced: %v", s.removed)
	}
	second := s.peers["tenant/gateway"]
	if second.RouterPublicKey == first.RouterPublicKey {
		t.Fatal("the new allocation reused the lost key")
	}
	assertReservedRouterKey(t, w, second)
}

// A saved connection is never replaced this way: the preflight refuses first.
func TestQuickEnrollmentKeyExpiryNeverTouchesASavedConnection(t *testing.T) {
	s := &removingHub{retryEnrollmentHub: &retryEnrollmentHub{peers: map[string]hub.Peer{}}}
	h := quickRetryHandler(s.retryEnrollmentHub)
	h.ConfigureHubEnrollment(s)
	if w := requestQuickEnrollment(h, "tenant"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	expireQuickKey(h)
	h.automaticPreflight = func(context.Context, string, string) error { return ErrHubManagementConfigured }
	w := requestQuickEnrollment(h, "tenant")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "hub_management_already_configured") || len(s.removed) != 0 {
		t.Fatalf("saved connection was touched: %d %v", w.Code, s.removed)
	}
}

// Without a way to remove the old allocation, nothing is changed.
func TestQuickEnrollmentKeyExpiryHoldsWithoutRemoval(t *testing.T) {
	s := &retryEnrollmentHub{peers: map[string]hub.Peer{}}
	h := quickRetryHandler(s)
	if w := requestQuickEnrollment(h, "tenant"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	reserved := s.peers["tenant/gateway"].RouterPublicKey
	expireQuickKey(h)
	w := requestQuickEnrollment(h, "tenant")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "hub_router_key_unavailable") {
		t.Fatalf("status = %d", w.Code)
	}
	if len(s.keys) != 1 || s.peers["tenant/gateway"].RouterPublicKey != reserved {
		t.Fatal("the existing allocation changed")
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

func TestQuickEnrollmentSavedManagementConflict(t *testing.T) {
	for _, role := range []string{"OWNER", "ADMIN", "DEVELOPER", "VIEWER"} {
		for _, foreign := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/foreign=%v", role, foreign), func(t *testing.T) {
				s := &retryEnrollmentHub{peers: map[string]hub.Peer{}}
				h := quickRetryHandler(s)
				preflights, preparations := 0, 0
				h.ConfigureAutomaticPreflight(func(_ context.Context, biz, id string) error {
					preflights++
					if biz != "tenant" || id != "gateway" {
						t.Fatal("preflight used an untrusted target")
					}
					return fmt.Errorf("private diagnostic: %w", ErrHubManagementConfigured)
				})
				h.automaticPrepare = func(context.Context, string, string, string, string) (string, error) {
					preparations++
					return "", nil
				}
				if foreign {
					h.liveDevice = func(*gin.Context, string, string) (*model.Device, error) {
						return &model.Device{BusinessID: "other", DeviceID: "gateway", DeploymentLayer: "MAIN_GATEWAY"}, nil
					}
				}
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest("POST", "/", strings.NewReader(`{"management_rest_port":443,"confirm_enrollment":true,"resume_existing":true}`))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Params = gin.Params{{Key: "deviceId", Value: "gateway"}}
				c.Set(auth.ContextKeyBusinessID, "tenant")
				c.Set(auth.ContextKeyUserID, "owner")
				c.Set(auth.ContextKeyUserRole, role)
				h.QuickEnrollGateway(c)
				want := 409
				if foreign {
					want = 404
				}
				if role == "VIEWER" {
					want = 403
				}
				if w.Code != want {
					t.Fatalf("status %d, want %d", w.Code, want)
				}
				conflict := strings.Contains(w.Body.String(), "hub_management_already_configured")
				if conflict != (want == 409) || preflights != map[bool]int{true: 1, false: 0}[want == 409] {
					t.Fatal("saved connection state escaped authorization or was misclassified")
				}
				if len(s.keys) != 0 || len(s.peers) != 0 || len(h.quickKeys) != 0 || preparations != 0 {
					t.Fatal("saved connection conflict allocated or prepared replacement credentials")
				}
				if strings.Contains(w.Body.String(), "private diagnostic") || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("diagnostic leaked or response cacheable")
				}
			})
		}
	}
}

func TestQuickEnrollmentManagementSavedDuringPreparation(t *testing.T) {
	s := &retryEnrollmentHub{peers: map[string]hub.Peer{}}
	h := quickRetryHandler(s)
	h.ConfigureAutomaticPreflight(func(context.Context, string, string) error { return nil })
	h.automaticPrepare = func(context.Context, string, string, string, string) (string, error) {
		return "", ErrHubManagementConfigured
	}
	w := requestQuickEnrollment(h, "tenant")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "hub_management_already_configured") || strings.Contains(w.Body.String(), "router_script") {
		t.Fatal("concurrent saved connection was not preserved and classified")
	}
}
