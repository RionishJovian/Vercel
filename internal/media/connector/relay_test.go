package connector

import (
	"bytes"
	"context"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"io"
	"mega-magic/internal/media/relay"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRelayFramesBoundedForLargeCatalog(t *testing.T) {
	// bytes.Reader implements WriterTo. Without the reader-only wrapper it would
	// write this whole payload as one frame and violate the cloud's 64 KiB bound.
	data := bytes.Repeat([]byte("synthetic-catalog"), 20000)
	result := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, e := websocket.Accept(w, r, nil)
		if e != nil {
			result <- e
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var metadata relay.Metadata
		if e = wsjson.Read(ctx, conn, &metadata); e != nil {
			result <- e
			return
		}
		stream := websocket.NetConn(ctx, conn, websocket.MessageBinary)
		conn.SetReadLimit(64 << 10)
		actual, e := io.ReadAll(io.LimitReader(stream, metadata.ContentLength))
		if e == nil && !bytes.Equal(actual, data) {
			e = io.ErrUnexpectedEOF
		}
		result <- e
	}))
	defer server.Close()
	runner, e := New(server.URL, &fakeLocal{}, false)
	if e != nil {
		t.Fatal(e)
	}
	defer runner.Close()
	runner.token = strings.Repeat("a", 64)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = runner.upload(ctx, strings.Repeat("b", 64), bytes.NewReader(data), PlaybackMetadata{Status: 200, ContentType: "application/json", ContentLength: int64(len(data))}); e != nil {
		t.Fatal(e)
	}
	if e = <-result; e != nil {
		t.Fatal(e)
	}
}
