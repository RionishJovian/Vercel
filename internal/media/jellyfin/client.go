// Package jellyfin implements the local-server side of Magic Media.
// It is not a tenant-facing HTTP handler. A future paired connector must authorize
// the tenant and operation before calling this client.
package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const maxResponse = 2 << 20

type Client struct {
	base  url.URL
	token string
	http  *http.Client
}

// NewLocal only accepts literal loopback addresses. This prevents a future web
// form from turning the connector into an arbitrary-network request proxy.
// No DNS, environment proxy, URL credentials or credential-bearing redirects.
func NewLocal(endpoint, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u == nil {
		return nil, errors.New("invalid local Jellyfin address")
	}
	ip := net.ParseIP(u.Hostname())
	if (u.Scheme != "http" && u.Scheme != "https") || ip == nil || !ip.IsLoopback() || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("Jellyfin address must be a loopback IP with an explicit port and no credentials, path or query")
	}
	if strings.ContainsAny(token, "\r\n") || len(token) > 4096 {
		return nil, errors.New("invalid Jellyfin credential")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{base: *u, token: token, http: &http.Client{
		Transport: transport, Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

type ServerInfo struct {
	ServerName             string `json:"ServerName"`
	Version                string `json:"Version"`
	ID                     string `json:"Id"`
	StartupWizardCompleted bool   `json:"StartupWizardCompleted"`
}

type Library struct {
	Name            string   `json:"Name"`
	ItemID          string   `json:"ItemId"`
	CollectionType  string   `json:"CollectionType"`
	Locations       []string `json:"Locations"`
	RefreshProgress *float64 `json:"RefreshProgress"`
}

// APIError deliberately excludes upstream bodies, URLs and credentials.
type APIError struct{ Status int }

func (e *APIError) Error() string {
	switch e.Status {
	case 401, 403:
		return "Jellyfin denied access; check the server credential and its permissions"
	case 409:
		return "Jellyfin reported a conflict; reload the libraries before trying again"
	default:
		return fmt.Sprintf("Jellyfin request failed (HTTP %d)", e.Status)
	}
}

func (c *Client) request(ctx context.Context, method, path string, query url.Values, body any, auth bool, out any) error {
	if auth && c.token == "" {
		return errors.New("a local Jellyfin credential is required")
	}
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return errors.New("invalid Jellyfin request")
		}
		payload = bytes.NewReader(encoded)
	}
	u := c.base
	u.Path, u.RawQuery = path, query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), payload)
	if err != nil {
		return errors.New("invalid Jellyfin request")
	}
	if auth {
		// Jellyfin 12 disables legacy token headers. Use the supported scheme,
		// with an encoded quoted value so credentials cannot inject parameters.
		req.Header.Set("Authorization", `MediaBrowser Token="`+url.PathEscape(c.token)+`"`)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("cannot reach the local Jellyfin server")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode}
	}
	if out == nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return errors.New("Jellyfin response could not be read safely")
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New("Jellyfin returned an invalid response")
	}
	return nil
}

func (c *Client) Status(ctx context.Context) (ServerInfo, error) {
	var info ServerInfo
	err := c.request(ctx, http.MethodGet, "/System/Info/Public", nil, nil, false, &info)
	return info, err
}

func (c *Client) Libraries(ctx context.Context) ([]Library, error) {
	items := []Library{}
	err := c.request(ctx, http.MethodGet, "/Library/VirtualFolders", nil, nil, true, &items)
	if items == nil {
		items = []Library{}
	}
	return items, err
}

type NewLibrary struct {
	Name string
	// Jellyfin collection types, not arbitrary user-supplied API paths.
	Kind string
	// Paths refer to the SERVER's filesystem, never the web user's computer.
	Paths []string
}

var windowsAbsolute = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

func ValidateLibrary(library NewLibrary) error {
	name := strings.TrimSpace(library.Name)
	if name == "" || len(name) > 120 || strings.ContainsAny(name, "/\\\r\n\x00") || name == "." || name == ".." {
		return errors.New("enter a valid library name")
	}
	switch library.Kind {
	case "movies", "tvshows", "music", "homevideos", "photos", "books":
	default:
		return errors.New("choose a supported library type")
	}
	if len(library.Paths) == 0 || len(library.Paths) > 20 {
		return errors.New("choose between one and twenty server folders")
	}
	for _, path := range library.Paths {
		// Windows paths must also validate when the client is built on macOS.
		if path == "" || len(path) > 4096 || strings.ContainsAny(path, "\x00\r\n") || (!strings.HasPrefix(path, "/") && !windowsAbsolute.MatchString(path) && !strings.HasPrefix(path, `\\`)) {
			return errors.New("media folders must use absolute server paths")
		}
	}
	return nil
}

func (c *Client) AddLibrary(ctx context.Context, library NewLibrary) error {
	if err := ValidateLibrary(library); err != nil {
		return err
	}
	name := strings.TrimSpace(library.Name)
	pathInfos := make([]map[string]string, 0, len(library.Paths))
	for _, path := range library.Paths {
		pathInfos = append(pathInfos, map[string]string{"Path": path})
	}
	// Use JSON PathInfos instead of comma-delimited query paths: folder names can
	// contain commas. Never auto-retry writes after an ambiguous network failure.
	body := map[string]any{"LibraryOptions": map[string]any{"PathInfos": pathInfos}}
	query := url.Values{"name": {name}, "collectionType": {library.Kind}, "refreshLibrary": {"false"}}
	return c.request(ctx, http.MethodPost, "/Library/VirtualFolders", query, body, true, nil)
}

// Scan requests a scan; acceptance is not proof that the scan has completed.
func (c *Client) Scan(ctx context.Context) error {
	return c.request(ctx, http.MethodPost, "/Library/Refresh", nil, nil, true, nil)
}
