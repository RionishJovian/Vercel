// Package omada is a read-only, server-side Omada Controller Open API client.
// It deliberately does NOT implement apoptimization.RadioAdapter.Apply.
// AP channel changes require separately reviewed controller-version-specific
// adapters, durable owner approval and hardware verification.
package omada

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const maxResponse = 512 << 10

var controllerID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
var apMAC = regexp.MustCompile(`^([A-Fa-f0-9]{2}[:-]){5}[A-Fa-f0-9]{2}$`)

var (
	ErrUnconfigured = errors.New("omada controller configuration invalid")
	ErrUnauthorized = errors.New("omada controller Open API authentication failed")
	ErrUnavailable  = errors.New("omada controller radio status unavailable")
)

// Settings must be loaded from tenant-scoped server secrets, NEVER from
// query/body fields supplied to an endpoint. BaseURL must be HTTPS and point
// to an explicitly enrolled controller protected by a network egress policy.
type Settings struct {
	BaseURL      string
	OmadaID      string
	SiteID       string
	ClientID     string
	ClientSecret string
}

type Caller interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	root     *url.URL
	settings Settings
	http     Caller
}

// RadioRead contains the ORIGINAL controller payload and provenance. No
// inferred channel scoring, transmit-power limits or actual physical links.
// Consumers must not treat this as a safe input for Apply until a provider-
// version-specific schema and legal channel constraints are validated.
type RadioRead struct {
	Source       string          `json:"source"`
	ControllerID string          `json:"-"`
	SiteID       string          `json:"-"`
	APMAC        string          `json:"ap_mac"`
	ObservedAt   time.Time       `json:"observed_at"`
	Status       json.RawMessage `json:"status"`
	Config       json.RawMessage `json:"config"`
	Writable     bool            `json:"writable"`
}

func New(settings Settings, caller Caller) (*Client, error) {
	u, err := url.Parse(settings.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") {
		return nil, ErrUnconfigured
	}
	if !controllerID.MatchString(settings.OmadaID) ||
		!controllerID.MatchString(settings.SiteID) ||
		strings.TrimSpace(settings.ClientID) == "" ||
		strings.TrimSpace(settings.ClientSecret) == "" {
		return nil, ErrUnconfigured
	}
	if caller == nil {
		// The default client cannot reach arbitrary hosts. Opt-in production
		// callers must use RestrictedCaller with a server-side CIDR allowlist.
		return nil, ErrUnconfigured
	}
	if supplied, ok := caller.(*http.Client); ok {
		// Even an injected client must never forward an API token to a
		// controller-supplied redirect target. Preserve its transport/TLS
		// configuration while forcing no redirects.
		copy := *supplied
		copy.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		if copy.Timeout <= 0 || copy.Timeout > 10*time.Second {
			copy.Timeout = 10*time.Second
		}
		caller = &copy
	}
	return &Client{root: u, settings: settings, http: caller}, nil
}

func (c *Client) postToken(ctx context.Context) (string, error) {
	body, err := json.Marshal(map[string]string{
		"omadacId": c.settings.OmadaID,
		"client_id": c.settings.ClientID,
		"client_secret": c.settings.ClientSecret,
	})
	if err != nil { return "", ErrUnauthorized }
	endpoint := c.root.ResolveReference(&url.URL{Path: "/openapi/authorize/token", RawQuery: "grant_type=client_credentials"})
	resp, err := c.request(ctx, http.MethodPost, endpoint, body, "")
	if err != nil { return "", ErrUnauthorized }
	var result struct {
		AccessToken string `json:"accessToken"`
		Result struct{ AccessToken string `json:"accessToken"` } `json:"result"`
		ErrorCode *int `json:"errorCode"`
	}
	if err := json.Unmarshal(resp, &result); err != nil { return "", ErrUnauthorized }
	if result.ErrorCode != nil && *result.ErrorCode != 0 { return "", ErrUnauthorized }
	token := result.AccessToken
	if token == "" { token = result.Result.AccessToken }
	if len(token) < 8 || len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
		return "", ErrUnauthorized
	}
	return token, nil
}

func (c *Client) request(ctx context.Context, method string, endpoint *url.URL, body []byte, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewReader(body))
	if err != nil { return nil, ErrUnavailable }
	if len(body) > 0 { req.Header.Set("Content-Type", "application/json") }
	if token != "" { req.Header.Set("Authorization", "AccessToken="+token) }
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil { return nil, ErrUnavailable }
	defer resp.Body.Close()
	// Never follow a controller redirect: OpenAPI credentials must never be
	// forwarded to a different host, and an HTTP 302 is not an API response.
	if resp.StatusCode != http.StatusOK { return nil, ErrUnavailable }
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(payload) > maxResponse { return nil, ErrUnavailable }
	return payload, nil
}

func checkEnvelope(payload []byte) (json.RawMessage, error) {
	var envelope struct {
		ErrorCode *int `json:"errorCode"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil ||
		envelope.ErrorCode == nil || *envelope.ErrorCode != 0 ||
		len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return nil, ErrUnavailable
	}
	return envelope.Result, nil
}

// ReadAPRadios fails closed: this Open API route and payload have not been
// verified for a supported controller version. Never infer optimization
// readiness from a device list or guess unsupported radio endpoints.
func (c *Client) ReadAPRadios(_ context.Context, _ string) (RadioRead, error) {
	return RadioRead{}, ErrUnavailable
}
