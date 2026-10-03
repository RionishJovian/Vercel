package connector

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mega-magic/internal/media/access"
	"mega-magic/internal/media/jellyfin"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type playbackLocal struct {
	fakeLocal
	opens                    int
	body                     io.ReadCloser
	library, item, byteRange string
}

func (f *playbackLocal) OpenMedia(ctx context.Context, library, item, byteRange string) (jellyfin.MediaStream, error) {
	f.opens++
	f.library, f.item, f.byteRange = library, item, byteRange
	if _, ok := ctx.Deadline(); !ok {
		return jellyfin.MediaStream{}, errors.New("missing deadline")
	}
	return jellyfin.MediaStream{Body: f.body, Status: 206, ContentType: "video/mp4", ContentLength: 4, ContentRange: "bytes 0-3/10"}, nil
}

type trackedBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *trackedBody) Close() error { b.closed.Store(true); return nil }

func TestConnectorPlaybackAuthorizationAndMetadata(t *testing.T) {
	body := &trackedBody{Reader: strings.NewReader("clip")}
	local := &playbackLocal{body: body}
	r, err := New("https://mikromagic.com", local, false)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.token = strings.Repeat("a", 64)
	var dst bytes.Buffer
	readyCalls := 0
	ready := func(m PlaybackMetadata) error {
		readyCalls++
		if dst.Len() != 0 || m.Status != 206 || m.ContentRange != "bytes 0-3/10" || m.ContentType != "video/mp4" || m.ContentLength != 4 {
			t.Fatal("unsafe or late metadata")
		}
		return nil
	}
	deny := func(context.Context) error { return errors.New("unpaid") }
	if n, e := r.StreamMedia(context.Background(), &dst, "library", "item", "bytes=0-3", time.Now().Add(time.Minute), deny, ready); n != 0 || !errors.Is(e, access.ErrDenied) || local.opens != 0 || readyCalls != 0 {
		t.Fatal("denied stream opened")
	}
	allow := func(context.Context) error { return nil }
	n, e := r.StreamMedia(context.Background(), &dst, "library", "item", "bytes=0-3", time.Now().Add(time.Minute), allow, ready)
	if e != nil || n != 4 || dst.String() != "clip" || !body.closed.Load() || local.library != "library" || local.item != "item" || local.byteRange != "bytes=0-3" || readyCalls != 1 {
		t.Fatal("authorized range transfer failed", e)
	}
	if local.writes != 0 {
		t.Fatal("playback changed libraries")
	}
}

func TestConnectorPlaybackExpiryAndRelayFailure(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	local := &playbackLocal{body: reader}
	r, _ := New("https://mikromagic.com", local, false)
	defer r.Close()
	r.token = strings.Repeat("a", 64)
	allow := func(context.Context) error { return nil }
	ready := func(PlaybackMetadata) error { return nil }
	start := time.Now()
	_, e := r.StreamMedia(context.Background(), io.Discard, "library", "item", "", start.Add(100*time.Millisecond), allow, ready)
	if !errors.Is(e, access.ErrDenied) || time.Since(start) > time.Second {
		t.Fatal("blocked stream survived expiry", e)
	}
	body := &trackedBody{Reader: strings.NewReader("clip")}
	local.body = body
	_, e = r.StreamMedia(context.Background(), io.Discard, "library", "item", "", time.Now().Add(time.Minute), allow, func(PlaybackMetadata) error { return errors.New("relay unavailable") })
	if !errors.Is(e, access.ErrDenied) || !body.closed.Load() {
		t.Fatal("relay failure leaked source")
	}
	r.token = ""
	opens := local.opens
	_, e = r.StreamMedia(context.Background(), io.Discard, "library", "item", "", time.Now().Add(time.Minute), allow, ready)
	if !errors.Is(e, access.ErrDenied) || local.opens != opens {
		t.Fatal("unpaired connector opened stream")
	}
}
