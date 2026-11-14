package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"mega-magic/internal/auth"
	"mega-magic/internal/database"
	"mega-magic/internal/hub"
	"mega-magic/internal/model"
)

// enrollmentDeadline bounds one enrolment request end to end (Hub lookups, the
// create and the management preparation), so an unreachable Hub never holds a
// worker or the per-router lock for long.
const enrollmentDeadline = 60 * time.Second

// callbackManagementPort matches the port the console has always sent.
const callbackManagementPort = 443

type callbackTokenStore interface {
	Issue(ctx context.Context, biz, device, requestedBy string, ttl time.Duration) (string, time.Time, error)
	Consume(ctx context.Context, token string) (biz, device string, err error)
}

type peerStateStore interface {
	Record(ctx context.Context, st database.HubPeerState) error
	TouchHandshake(ctx context.Context, biz, device string, at time.Time) error
}

// ConfigureCallbackEnrollment turns on router-generated-key enrolment. It is off
// unless the server configuration enables it and supplies the public base URL.
func (h *VPSHandler) ConfigureCallbackEnrollment(baseURL string, tokens callbackTokenStore) error {
	if _, err := hub.CallbackURL(baseURL); err != nil || tokens == nil {
		return errors.New("router-generated key enrolment needs an https base URL and a token store")
	}
	h.callbackBaseURL, h.callbackTokens = baseURL, tokens
	if h.callbackDevice == nil {
		h.callbackDevice = func(ctx context.Context, biz, id string) (*model.Device, error) {
			var device *model.Device
			err := withDeviceTenant(h.db, biz, func(tx *sql.Tx) error {
				var e error
				device, e = h.deviceRepo.GetDeviceByIDTXN(tx, id)
				return e
			})
			return device, err
		}
	}
	return nil
}

// ConfigurePeerState records each Hub allocation and verified handshake durably.
func (h *VPSHandler) ConfigurePeerState(store peerStateStore) { h.peerState = store }

// ConfigureRouterKeyPurge removes any server-held router private key once the
// router has proven it holds it, by completing a handshake.
func (h *VPSHandler) ConfigureRouterKeyPurge(f func(ctx context.Context, biz, device string) error) {
	h.purgeRouterKey = f
}

func (h *VPSHandler) callbackReady() bool {
	return h.callbackTokens != nil && h.callbackBaseURL != "" && h.callbackDevice != nil &&
		h.enrollmentHub != nil && h.liveHub != nil && h.livePlan != nil && h.automaticPrepare != nil
}

// IssueCallbackScript returns the one block to paste in WinBox for router-generated
// key enrolment. The app holds no key: it issues a single-use token and nothing else.
func (h *VPSHandler) IssueCallbackScript(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	biz, user := auth.GetBusinessID(c), auth.GetUserID(c)
	if biz == "" || user == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	if !model.HasAtLeastRole(model.UserRole(auth.GetUserRole(c)), model.RoleAdmin) {
		c.JSON(403, gin.H{"error": "gateway management permission required"})
		return
	}
	if !h.callbackReady() || h.liveDevice == nil {
		c.JSON(503, gin.H{"error": "Router-generated key setup is not enabled on this server", "code": "hub_callback_disabled"})
		return
	}
	if !h.livePlan(biz) {
		h.planDenied(c)
		return
	}
	device, err := h.liveDevice(c, biz, c.Param("deviceId"))
	if err != nil {
		c.JSON(503, gin.H{"error": "gateway lookup unavailable"})
		return
	}
	if device == nil || device.BusinessID != biz || device.DeviceID != c.Param("deviceId") || device.DeploymentLayer != "MAIN_GATEWAY" {
		c.JSON(404, gin.H{"error": "gateway not found in your tenant"})
		return
	}
	if h.automaticPreflight != nil {
		if e := h.automaticPreflight(c.Request.Context(), biz, device.DeviceID); e != nil {
			if errors.Is(e, ErrHubManagementConfigured) {
				c.JSON(409, gin.H{"error": "This router already has a saved management connection. Use Finish connection to verify it; no setup credentials were replaced.", "code": "hub_management_already_configured"})
				return
			}
			c.JSON(503, gin.H{"error": "One-paste preparation is unavailable on this server. No new Hub allocation was requested.", "code": "hub_management_preflight_failed"})
			return
		}
	}
	token, expires, err := h.callbackTokens.Issue(c.Request.Context(), biz, device.DeviceID, user, hub.CallbackTokenTTL)
	if err != nil {
		c.JSON(503, gin.H{"error": "setup token temporarily unavailable"})
		return
	}
	script, err := hub.BuildCallbackBootstrapScript(device.DeviceID, h.callbackBaseURL, token)
	if err != nil {
		c.JSON(503, gin.H{"error": "setup script unavailable"})
		return
	}
	c.JSON(200, gin.H{"device_id": device.DeviceID, "router_script": script, "expires_at": expires,
		"state": "awaiting_router_callback", "one_paste": true, "key_custody": "router_only",
		"lan_reachability_verified": false, "router_configuration_required": true})
}

type callbackBody struct {
	Token     string `json:"token"`
	PublicKey string `json:"public_key"`
	Version   string `json:"routeros_version"`
}

func callbackText(c *gin.Context, status int, message string) {
	c.Header("Cache-Control", "no-store")
	c.Data(status, "text/plain; charset=utf-8", []byte(message))
}

// CallbackEnroll is called by the router itself. It is authenticated only by a
// single-use token, takes a PUBLIC key (never a private one), refuses RouterOS 6,
// and answers with the validated tunnel script the router then runs.
func (h *VPSHandler) CallbackEnroll(c *gin.Context) {
	if !h.callbackReady() {
		callbackText(c, http.StatusServiceUnavailable, "Router-generated key setup is not enabled")
		return
	}
	// Read the whole bounded body first: a decoder alone would stop at the end of
	// the first JSON value and never notice an oversized request.
	raw, readErr := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 2048))
	var body callbackBody
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if readErr != nil || c.ContentType() != "application/json" || decoder.Decode(&body) != nil || decoder.More() {
		callbackText(c, http.StatusBadRequest, "malformed request")
		return
	}
	key, err := base64.StdEncoding.DecodeString(body.PublicKey)
	zero := true
	for _, b := range key {
		if b != 0 {
			zero = false
		}
	}
	if !hub.ValidCallbackToken(body.Token) || err != nil || len(key) != 32 || zero || base64.StdEncoding.EncodeToString(key) != body.PublicKey {
		callbackText(c, http.StatusBadRequest, "valid token and WireGuard public key required; never send a private key")
		return
	}
	// Refuse before the token is spent: a router on RouterOS 6 can upgrade and retry.
	if hub.RequireWireGuard(body.Version) != nil {
		callbackText(c, http.StatusUnprocessableEntity, "Magic Hub needs RouterOS 7 or newer")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), enrollmentDeadline)
	defer cancel()
	biz, deviceID, err := h.callbackTokens.Consume(ctx, body.Token)
	if err != nil {
		code := http.StatusForbidden
		if !errors.Is(err, database.ErrCallbackTokenInvalid) {
			code = http.StatusServiceUnavailable
		}
		callbackText(c, code, "setup token is not valid; ask the app for a new script")
		return
	}
	device, err := h.callbackDevice(ctx, biz, deviceID)
	if err != nil {
		callbackText(c, http.StatusServiceUnavailable, "gateway lookup unavailable")
		return
	}
	if device == nil || device.BusinessID != biz || device.DeviceID != deviceID || device.DeploymentLayer != "MAIN_GATEWAY" {
		callbackText(c, http.StatusNotFound, "gateway not found")
		return
	}
	if !h.livePlan(biz) {
		callbackText(c, http.StatusForbidden, "this business plan does not include Magic Hub")
		return
	}
	unlock, err := h.enrollLocks.Lock(ctx, biz+"/"+deviceID)
	if err != nil {
		callbackText(c, http.StatusServiceUnavailable, "another setup for this router is still running; ask the app for a new script")
		return
	}
	defer unlock()
	if h.automaticPreflight != nil {
		if e := h.automaticPreflight(ctx, biz, deviceID); e != nil {
			if errors.Is(e, ErrHubManagementConfigured) {
				callbackText(c, http.StatusConflict, "this router already has a saved management connection")
				return
			}
			callbackText(c, http.StatusServiceUnavailable, "setup is unavailable on this server")
			return
		}
	}
	identity := hub.Identity{TenantID: biz, RouterID: deviceID, RequestedBy: "hub-callback"}
	peer, fail := h.reusableOrNewPeer(ctx, identity, body.PublicKey)
	if fail != nil {
		callbackText(c, fail.status, fail.message)
		return
	}
	scripts, buildErr := hub.BuildRouterScripts(deviceID, peer)
	if buildErr != nil {
		callbackText(c, http.StatusBadGateway, "Hub allocation lacks safe script configuration")
		return
	}
	script, prepErr := h.automaticPrepare(ctx, biz, deviceID, peer.RouterPublicKey, scripts.Setup)
	if prepErr != nil {
		if errors.Is(prepErr, ErrHubManagementConfigured) {
			callbackText(c, http.StatusConflict, "this router already has a saved management connection")
			return
		}
		callbackText(c, http.StatusServiceUnavailable, "management preparation failed")
		return
	}
	callbackText(c, http.StatusOK, script)
}

type callbackFailure struct {
	status  int
	message string
}

// reusableOrNewPeer returns the router's Hub peer for exactly this public key. An
// existing peer with the same key is reused, so a retried paste never allocates a
// second /32. A peer with a different key is replaced only if it never completed a
// handshake; a connected router is never torn down by a setup request.
func (h *VPSHandler) reusableOrNewPeer(ctx context.Context, identity hub.Identity, publicKey string) (hub.Peer, *callbackFailure) {
	peer, err := h.liveHub.InspectRouter(ctx, identity)
	if err != nil && !errors.Is(err, hub.ErrPeerNotFound) {
		return hub.Peer{}, &callbackFailure{http.StatusServiceUnavailable, "Hub lookup unavailable; nothing changed"}
	}
	fresh := false
	if peer.PeerID != "" {
		if peer.TenantID != identity.TenantID || peer.RouterID != identity.RouterID {
			return hub.Peer{}, &callbackFailure{http.StatusBadGateway, "Magic Hub identity mismatch"}
		}
		if peer.RouterPublicKey != publicKey {
			if peer.LastHandshakeAt != nil {
				return hub.Peer{}, &callbackFailure{http.StatusConflict, "this router is already connected with a different key; remove its Hub connection first"}
			}
			if h.enrollmentRemover == nil {
				return hub.Peer{}, &callbackFailure{http.StatusConflict, "existing Hub allocation cannot be replaced; enrollment preserved"}
			}
			if h.enrollmentRemover.RemovePeer(ctx, identity, peer.PeerID) != nil {
				return hub.Peer{}, &callbackFailure{http.StatusServiceUnavailable, "Hub could not replace the old allocation; try again"}
			}
			peer = hub.Peer{}
		}
	}
	if peer.PeerID == "" {
		peer, err = h.enrollmentHub.EnrollPeer(ctx, identity, publicKey, callbackManagementPort)
		if err != nil || peer.TenantID != identity.TenantID || peer.RouterID != identity.RouterID || peer.RouterPublicKey != publicKey || peer.EnrollmentVersion != 1 || peer.State != "active" {
			return hub.Peer{}, &callbackFailure{http.StatusBadGateway, "enrollment not confirmed; ask the app for a new script"}
		}
		fresh = true
	}
	if e := h.recordAllocation(ctx, identity, peer, fresh); e != nil {
		return hub.Peer{}, &callbackFailure{http.StatusBadGateway, "Hub allocation conflicts with another router; no setup was issued"}
	}
	return peer, nil
}

// recordAllocation persists the allocation. A /32 or key that collides with another
// router is refused; if this call just created the Hub peer it is removed again, so
// a refusal never leaves an orphan peer behind.
func (h *VPSHandler) recordAllocation(ctx context.Context, identity hub.Identity, peer hub.Peer, fresh bool) error {
	if h.peerState == nil {
		return nil
	}
	err := h.peerState.Record(ctx, database.HubPeerState{BusinessID: identity.TenantID, DeviceID: identity.RouterID, PeerID: peer.PeerID,
		RouterPublicKey: peer.RouterPublicKey, TunnelAddress: peer.TunnelAddress, Endpoint: peer.Endpoint})
	if err == nil {
		return nil
	}
	if errors.Is(err, database.ErrTunnelAddressInUse) && fresh && h.enrollmentRemover != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		_ = h.enrollmentRemover.RemovePeer(cleanup, identity, peer.PeerID)
	}
	return err
}

// observeVerifiedHandshake runs whenever the app has seen a recent, tenant-matched
// handshake for a router. The router has then proved it holds its key, so any
// server-held copy (legacy flow) is deleted, and the handshake time is kept.
// Failures are deliberately silent to the caller: this is housekeeping.
func (h *VPSHandler) observeVerifiedHandshake(ctx context.Context, biz, device string, peer hub.Peer) {
	if peer.LastHandshakeAt == nil || peer.State != "active" || peer.TenantID != biz || peer.RouterID != device {
		return
	}
	age := time.Since(*peer.LastHandshakeAt)
	if age < 0 || age > 5*time.Minute {
		return
	}
	bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if h.peerState != nil {
		_ = h.peerState.TouchHandshake(bounded, biz, device, *peer.LastHandshakeAt)
	}
	if h.purgeRouterKey != nil {
		_ = h.purgeRouterKey(bounded, biz, device)
	}
}
