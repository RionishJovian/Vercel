// Package hub implements the existing mikromagic-hub signed control-plane API.
// Callers must resolve tenant/router/user identity from authenticated server state.
// This client does not grant app permissions or configure a customer router.
package hub

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type Identity struct{ TenantID, RouterID, RequestedBy string }

type Options struct {
	URL        string
	KeyID      string
	SigningKey string
	// Transport is optional for isolated tests. Production defaults to verified TLS.
	Transport http.RoundTripper
}

type Client struct {
	base  string
	keyID string
	key   []byte
	http  *http.Client
	now   func() time.Time
}

type Peer struct {
	PeerID            string     `json:"peerId"`
	TenantID          string     `json:"tenantId"`
	RouterID          string     `json:"routerId"`
	RouterPublicKey   string     `json:"routerPublicKey"`
	TunnelAddress     string     `json:"tunnelAddress"`
	HubPublicKey      string     `json:"hubPublicKey"`
	Endpoint          string     `json:"endpoint"`
	AllowedIPs        []string   `json:"allowedIps"`
	State             string     `json:"state"`
	EnrollmentVersion int        `json:"enrollmentVersion"`
	LastHandshakeAt   *time.Time `json:"lastHandshakeAt"`
	RXBytes           *uint64    `json:"rxBytes"`
	TXBytes           *uint64    `json:"txBytes"`
}

func New(o Options) (*Client, error) {
	u, err := url.Parse(o.URL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" ||
		(u.Path != "" && u.Path != "/" && strings.TrimSuffix(u.Path, "/") != "/internal/provisioner") || !identifier.MatchString(o.KeyID) {
		return nil, errors.New("invalid Hub HTTPS URL or key identifier")
	}
	// Use the canonical 32-byte hex key contract. Do not silently fall back to
	// legacy passphrase signing or include key material in validation errors.
	key, err := hex.DecodeString(o.SigningKey)
	if err != nil || len(key) != 32 {
		return nil, errors.New("Hub signing key must be 32-byte hex")
	}
	transport := o.Transport
	if transport == nil {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	}
	return &Client{base: strings.TrimSuffix(o.URL, "/"), keyID: o.KeyID, key: key, now: time.Now,
		http: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// InspectPeer is read-only. Empty/stale handshake timestamps must not be rendered
// as a connected gateway merely because the peer record's state is active.
func (c *Client) InspectPeer(ctx context.Context, id Identity, peerID string) (Peer, error) {
	if !identifier.MatchString(peerID) {
		return Peer{}, errors.New("invalid peer identifier")
	}
	peer, err := c.requestPeer(ctx, id, http.MethodGet, "/v1/peers/"+peerID, "peers:inspect", nil)
	if err == nil && peer.PeerID != peerID {
		return Peer{}, errors.New("mismatched Hub peer identifier")
	}
	return peer, err
}

// EnrollPeer modifies the live Hub peer store when explicitly called. It is not
// wired to server routes by default; handlers must enforce role/tenant/approval.
func (c *Client) EnrollPeer(ctx context.Context, id Identity, publicKey string, restPort int) (Peer, error) {
	key, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(key) != 32 || base64.StdEncoding.EncodeToString(key) != publicKey || restPort < 1 || restPort > 65535 {
		return Peer{}, errors.New("invalid router public key or management port")
	}
	allZero := true
	for _, b := range key {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return Peer{}, errors.New("invalid router public key")
	}
	body := struct {
		TenantID           string `json:"tenantId"`
		RouterID           string `json:"routerId"`
		RouterPublicKey    string `json:"routerPublicKey"`
		ManagementRESTPort int    `json:"managementRestPort"`
	}{id.TenantID, id.RouterID, publicKey, restPort}
	// POST is the create-only extension. Older Hubs reject it; never fall back
	// to the legacy PUT route, which replaces existing router public keys.
	peer, err := c.requestPeer(ctx, id, http.MethodPost, "/v1/peers/by-router/"+id.RouterID, "peers:create", body)
	if err != nil {
		return Peer{}, err
	}
	address, addressErr := netip.ParsePrefix(peer.TunnelAddress)
	if peer.EnrollmentVersion != 1 || peer.RouterPublicKey != publicKey || peer.State != "active" ||
		addressErr != nil || !address.Addr().Is4() || !address.Addr().IsPrivate() || address.Bits() != 32 {
		return Peer{}, errors.New("Hub enrollment did not verify")
	}
	return peer, nil
}

// InspectRouter requires the read-only by-router extension on the Hub. Older
// deployments return an error; never fall back to PUT enrollment for lookup.
func (c *Client) InspectRouter(ctx context.Context, id Identity) (Peer, error) {
	if !identifier.MatchString(id.RouterID) {
		return Peer{}, errors.New("invalid router identifier")
	}
	return c.requestPeer(ctx, id, http.MethodGet, "/v1/peers/by-router/"+id.RouterID, "peers:inspect", nil)
}

func (c *Client) requestPeer(ctx context.Context, id Identity, method, path, scope string, body any) (Peer, error) {
	if !identifier.MatchString(id.TenantID) || !identifier.MatchString(id.RouterID) || !identifier.MatchString(id.RequestedBy) {
		return Peer{}, errors.New("invalid Hub request identity")
	}
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return Peer{}, errors.New("invalid Hub request body")
		}
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return Peer{}, errors.New("cannot generate Hub request identifier")
	}
	requestID := hex.EncodeToString(random)
	timestamp := c.now().UTC().Format(time.RFC3339Nano)
	canonical := strings.Join([]string{method, path, timestamp, requestID, scope, id.TenantID, id.RouterID, id.RequestedBy, string(payload)}, "\n")
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
	if err != nil {
		return Peer{}, errors.New("cannot construct Hub request")
	}
	for k, v := range map[string]string{"x-mm-key-id": c.keyID, "x-mm-timestamp": timestamp, "x-mm-request-id": requestID, "x-mm-scope": scope, "x-mm-tenant-id": id.TenantID, "x-mm-router-id": id.RouterID, "x-mm-requested-by": id.RequestedBy, "x-mm-signature": signature(c.key, canonical), "Content-Type": "application/json"} {
		req.Header.Set(k, v)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return Peer{}, errors.New("Hub request failed")
	}
	defer res.Body.Close()
	// Never echo Hub error bodies, URLs, signatures or credentials into app errors.
	if res.StatusCode != http.StatusOK {
		return Peer{}, fmt.Errorf("Hub returned HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64*1024+1))
	if err != nil || len(raw) > 64*1024 {
		return Peer{}, errors.New("invalid Hub response size")
	}
	var peer Peer
	if json.Unmarshal(raw, &peer) != nil || !identifier.MatchString(peer.PeerID) || peer.TenantID != id.TenantID || peer.RouterID != id.RouterID {
		return Peer{}, errors.New("invalid or mismatched Hub peer response")
	}
	return peer, nil
}

func signature(key []byte, canonical string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
