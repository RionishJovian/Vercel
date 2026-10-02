// Package connector runs on the Jellyfin computer, never on the cloud API host.
package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mega-magic/internal/media/bridge"
	"mega-magic/internal/media/jellyfin"
	"net"
	"net/http"
	"net/url"
	"time"
)

type Local interface {
	Status(context.Context) (jellyfin.ServerInfo, error)
	Libraries(context.Context) ([]jellyfin.Library, error)
	AddLibrary(context.Context, jellyfin.NewLibrary) error
	Scan(context.Context) error
}
type Runner struct {
	cloud       url.URL
	http        *http.Client
	token       string
	local       Local
	allowWrites bool
}

func New(cloud string, local Local, allowWrites bool) (*Runner, error) {
	u, e := url.Parse(cloud)
	if e != nil || u == nil {
		return nil, errors.New("invalid cloud address")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := ip != nil && ip.IsLoopback()
	if (u.Scheme != "https" && !(u.Scheme == "http" && loopback)) || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || local == nil {
		return nil, errors.New("cloud connection requires HTTPS (HTTP allowed only on a literal loopback IP)")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Runner{cloud: *u, local: local, allowWrites: allowWrites, http: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (r *Runner) Close() { r.http.CloseIdleConnections() }
func (r *Runner) call(ctx context.Context, path string, body, out any) (int, error) {
	data, e := json.Marshal(body)
	if e != nil {
		return 0, errors.New("invalid connector request")
	}
	u := r.cloud
	u.Path = "/api/v1/media-connector" + path
	req, e := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(data))
	if e != nil {
		return 0, errors.New("invalid connector request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	resp, e := r.http.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		return 0, errors.New("cloud connection unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, errors.New("cloud refused connector request")
	}
	if out != nil && resp.StatusCode != 204 {
		data, e = io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
		if e != nil || len(data) > 1<<20 || json.Unmarshal(data, out) != nil {
			return resp.StatusCode, errors.New("invalid cloud response")
		}
	}
	return resp.StatusCode, nil
}
func (r *Runner) Pair(ctx context.Context, code string) error {
	if _, e := bridge.Hash(code); e != nil {
		return errors.New("invalid pairing code")
	}
	info, e := r.local.Status(ctx)
	if e != nil {
		return e
	}
	if !info.StartupWizardCompleted {
		return errors.New("finish Jellyfin setup before pairing")
	}
	var response struct {
		Token string `json:"connector_token"`
	}
	if _, e = r.call(ctx, "/redeem", map[string]string{"pairing_code": code, "server_name": info.ServerName}, &response); e != nil {
		return e
	}
	if _, e = bridge.Hash(response.Token); e != nil {
		return errors.New("invalid connector credential")
	}
	r.token = response.Token
	return nil
}
func (r *Runner) Execute(ctx context.Context, j bridge.Job) bridge.Completion {
	result := bridge.Completion{}
	if e := bridge.ValidateOperation(j.Operation, j.Payload); e != nil {
		return bridge.Completion{ErrorCode: "invalid_request"}
	}
	if !time.Now().Before(j.Expires) {
		return bridge.Completion{ErrorCode: "invalid_request"}
	}
	var e error
	switch j.Operation {
	case "dashboard":
		reader, ok := r.local.(interface {
			Dashboard(context.Context) (jellyfin.Dashboard, error)
		})
		if !ok {
			return bridge.Completion{ErrorCode: "operation_failed"}
		}
		var snapshot jellyfin.Dashboard
		snapshot, e = reader.Dashboard(ctx)
		if e == nil {
			e = jellyfin.ValidateDashboard(snapshot)
		}
		if e == nil {
			result.Dashboard = &snapshot
		}
	case "status":
		var info jellyfin.ServerInfo
		info, e = r.local.Status(ctx)
		if e == nil {
			result.Status = &info
		}
	case "libraries":
		result.Libraries, e = r.local.Libraries(ctx)
	case "add_library":
		if !r.allowWrites {
			return bridge.Completion{ErrorCode: "operation_failed"}
		}
		var body bridge.AddPayload
		json.Unmarshal(j.Payload, &body)
		e = r.local.AddLibrary(ctx, body.Library)
		result.Accepted = e == nil
	case "scan":
		if !r.allowWrites {
			return bridge.Completion{ErrorCode: "operation_failed"}
		}
		e = r.local.Scan(ctx)
		result.Accepted = e == nil
	}
	if e != nil {
		result = bridge.Completion{ErrorCode: "local_server_unavailable"}
		var api *jellyfin.APIError
		if errors.As(e, &api) {
			result.ErrorCode = "operation_failed"
			if api.Status == 401 || api.Status == 403 {
				result.ErrorCode = "local_credential_denied"
			}
		}
	}
	return result
}

// Step claims at most one job and never retries a write or ambiguous claim.
func (r *Runner) Step(ctx context.Context) error {
	if r.token == "" {
		return errors.New("connector is not paired")
	}
	var j bridge.Job
	status, e := r.call(ctx, "/poll", struct{}{}, &j)
	if e != nil {
		return e
	}
	if status == 204 {
		return nil
	}
	if !bridge.UUID.MatchString(j.ID) {
		return errors.New("invalid media job")
	}
	deadline, cancel := context.WithDeadline(ctx, j.Expires)
	defer cancel()
	result := r.Execute(deadline, j)
	_, e = r.call(ctx, "/jobs/"+j.ID+"/result", result, nil)
	return e
}
