package discovery

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Everything the helper sends is built here, so what leaves the machine can be shown exactly (--dry-run) and tested.
// It never sends the raw software id, uptime, IPv6 addresses or anything about other devices on the network.

const (
	maxUploadObservations = 64
	maxResponseBytes      = 16 << 10
)

var (
	// ErrSessionEnded means the server no longer accepts this helper: the session ended, expired or was cancelled.
	ErrSessionEnded = errors.New("the discovery session has ended")
	// ErrSlowDown means the server asked the helper to upload less often.
	ErrSlowDown = errors.New("the server asked for slower uploads")
)

// PairedSession is what the server returns once. The token is kept in memory only and never printed.
type PairedSession struct {
	SessionID string    `json:"session_id"`
	Token     string    `json:"helper_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type wireObservation struct {
	DeviceKey  string   `json:"device_key"`
	MACs       []string `json:"macs"`
	Identity   string   `json:"identity"`
	Board      string   `json:"board"`
	Platform   string   `json:"platform"`
	Version    string   `json:"version"`
	Interface  string   `json:"interface"`
	ClaimedIP  string   `json:"claimed_ip"`
	SourceIP   string   `json:"source_ip"`
	IPMismatch bool     `json:"ip_mismatch"`
	IPChanged  bool     `json:"ip_changed"`
	HeardCount int      `json:"heard_count"`
	Status     string   `json:"status"`
}

func privateV4(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is4() && a.IsPrivate()
}

// Payload builds the exact upload body. Observations that did not come from a private IPv4 address are left out:
// this helper is for routers on the local network, and the server would reject anything else.
func Payload(obs []Observation) ([]byte, error) {
	out := struct {
		Observations []wireObservation `json:"observations"`
	}{Observations: []wireObservation{}}
	for _, o := range obs {
		if !privateV4(o.SourceIP) {
			continue
		}
		key := o.DeviceKey
		if key == "" {
			key = "mac:" + o.MAC
		}
		macs := o.MACs
		if len(macs) == 0 {
			macs = []string{o.MAC}
		}
		claimed := o.ClaimedIP
		if !privateV4(claimed) {
			claimed = ""
		}
		out.Observations = append(out.Observations, wireObservation{
			DeviceKey: key, MACs: macs, Identity: o.Identity, Board: o.Board, Platform: o.Platform, Version: o.Version, Interface: o.Interface,
			ClaimedIP: claimed, SourceIP: o.SourceIP, IPMismatch: o.IPMismatch, IPChanged: o.IPChanged, HeardCount: max(o.Count, 1), Status: StatusNearbyUnverified,
		})
		if len(out.Observations) == maxUploadObservations {
			break
		}
	}
	return json.MarshalIndent(out, "", "  ")
}

// Uploader talks to the Mega Magic server over HTTPS only, with normal certificate verification.
type Uploader struct {
	base *url.URL
	hc   *http.Client
}

// NewUploader refuses anything but an https URL. hc may be nil; tests pass a client that trusts a local test certificate.
func NewUploader(server string, hc *http.Client) (*Uploader, error) {
	u, err := url.Parse(strings.TrimRight(server, "/"))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("the server address must be an https URL such as https://megamagic.tech")
	}
	if hc == nil {
		hc = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}
	}
	c := *hc
	c.Timeout = 15 * time.Second
	// A redirect could carry the token to another host or to plain http, so none is followed.
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Uploader{base: u, hc: &c}, nil
}

func (u *Uploader) do(ctx context.Context, path, token string, body []byte, v any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.base.String()+path, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := u.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode/100 == 2 && v != nil {
		if err := json.Unmarshal(data, v); err != nil {
			return resp.StatusCode, fmt.Errorf("unexpected response from the server")
		}
	}
	return resp.StatusCode, nil
}

// Pair trades the typed code for a short-lived token.
func (u *Uploader) Pair(ctx context.Context, code string) (PairedSession, error) {
	b, _ := json.Marshal(map[string]string{"pairing_code": strings.TrimSpace(code)})
	var out PairedSession
	status, err := u.do(ctx, "/api/v1/discovery/pair", "", b, &out)
	if err != nil {
		return out, err
	}
	if status != http.StatusCreated || out.SessionID == "" || out.Token == "" {
		return PairedSession{}, errors.New("the pairing code was not accepted; it may have expired or been used. Start a new session in Mega Magic")
	}
	return out, nil
}

// Upload sends one batch. It reports how many candidates the server kept.
func (u *Uploader) Upload(ctx context.Context, s PairedSession, obs []Observation) (accepted, dropped int, err error) {
	b, err := Payload(obs)
	if err != nil {
		return 0, 0, err
	}
	var out struct {
		Accepted int `json:"accepted"`
		Dropped  int `json:"dropped"`
	}
	status, err := u.do(ctx, "/api/v1/discovery/sessions/"+url.PathEscape(s.SessionID)+"/observations", s.Token, b, &out)
	switch {
	case err != nil:
		return 0, 0, err
	case status == http.StatusOK:
		return out.Accepted, out.Dropped, nil
	case status == http.StatusTooManyRequests:
		return 0, 0, ErrSlowDown
	case status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound:
		return 0, 0, ErrSessionEnded
	default:
		return 0, 0, fmt.Errorf("the server refused the upload (status %d)", status)
	}
}
