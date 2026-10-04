package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"io"
	"mega-magic/internal/media/bridge"
	"mega-magic/internal/media/jellyfin"
	"mega-magic/internal/media/relay"
	"net/http"
	"time"
)

// RelayStep serves one guest request over an outbound HTTPS upload. Neither
// Jellyfin keys nor local URLs appear in the task or forwarded response.
func (r *Runner) RelayStep(ctx context.Context) error {
	var task relay.Task
	status, e := r.call(ctx, "/relay/poll", struct{}{}, &task)
	if e != nil || status == 204 {
		return e
	}
	if _, e = bridge.Hash(task.ID); e != nil || !time.Now().Before(task.Expires) || time.Until(task.Expires) > 30*24*time.Hour {
		return errors.New("invalid relay task")
	}
	ctx, cancel := context.WithDeadline(ctx, task.Expires)
	defer cancel()
	check := func(ctx context.Context) error {
		checkCtx, stop := context.WithTimeout(ctx, 3*time.Second)
		defer stop()
		_, e := r.call(checkCtx, "/relay/"+task.ID+"/check", struct{}{}, nil)
		return e
	}
	if e = check(ctx); e != nil {
		return e
	}
	if task.Operation == "catalog" {
		local, ok := r.local.(interface {
			Catalog(context.Context, string, int) ([]jellyfin.CatalogItem, error)
		})
		if !ok {
			return errors.New("catalog unavailable")
		}
		items, e := local.Catalog(ctx, task.Library, task.Start)
		if e != nil {
			return e
		}
		data, e := json.Marshal(items)
		if e != nil {
			return e
		}
		return r.upload(ctx, task.ID, bytes.NewReader(data), PlaybackMetadata{Status: 200, ContentType: "application/json", ContentLength: int64(len(data))})
	}
	// Cloud transport is metadata-only. Video stays on the owner's LAN even
	// when an older cloud release mistakenly requests a streaming task.
	return errors.New("playback requires the business local network")
}
func (r *Runner) upload(ctx context.Context, id string, body io.Reader, m PlaybackMetadata) error {
	if m.ContentType != "application/json" || m.Status != 200 || m.ContentLength < 0 || m.ContentLength > 1<<20 || m.ContentRange != "" {
		return errors.New("cloud transport accepts catalog metadata only")
	}
	u := r.cloud
	u.Path = "/api/v1/media-connector/relay/" + id + "/socket"
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	client := *r.http
	client.Timeout = 0
	handshake, stop := context.WithTimeout(ctx, 10*time.Second)
	conn, _, e := websocket.Dial(handshake, u.String(), &websocket.DialOptions{HTTPClient: &client, HTTPHeader: http.Header{"Authorization": {"Bearer " + r.token}}, CompressionMode: websocket.CompressionDisabled})
	stop()
	if e != nil {
		return errors.New("media relay unavailable")
	}
	defer conn.CloseNow()
	if e = wsjson.Write(ctx, conn, m); e != nil {
		return errors.New("media relay refused metadata")
	}
	stream := websocket.NetConn(ctx, conn, websocket.MessageBinary)
	defer stream.Close()
	// Hide Reader.WriteTo so bytes.Reader/catalog JSON cannot become one huge
	// WebSocket message. Copy's fixed-size buffer keeps frames below 64 KiB.
	if _, e = io.Copy(stream, struct{ io.Reader }{body}); e != nil {
		return errors.New("media relay disconnected")
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
	return nil
}
