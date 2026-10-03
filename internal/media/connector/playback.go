package connector

import (
	"context"
	"io"
	"mega-magic/internal/media/access"
	"mega-magic/internal/media/jellyfin"
	"mega-magic/internal/media/relay"
	"time"
)

// PlaybackMetadata contains only the headers a guest relay may forward. It never
// exposes an upstream URL, Jellyfin key or arbitrary upstream response headers.
type PlaybackMetadata = relay.Metadata

// StreamMedia is the owner-PC playback boundary. The relay must supply a check
// bound to the authenticated business/pass/guest and owner-selected library;
// request values alone are not authorization. The cloud ledger supplies expires.
// ready runs after authorization and local range validation, before media bytes.
// This is not a public HTTP endpoint and does not accept browser URLs.
func (r *Runner) StreamMedia(ctx context.Context, dst io.Writer, library, item, byteRange string, expires time.Time, check access.Check, ready func(PlaybackMetadata) error) (int64, error) {
	local, ok := r.local.(interface {
		OpenMedia(context.Context, string, string, string) (jellyfin.MediaStream, error)
	})
	if !ok || r.token == "" || ready == nil {
		return 0, access.ErrDenied
	}
	return access.Copy(ctx, dst, expires, check, func(streamCtx context.Context) (io.ReadCloser, error) {
		stream, err := local.OpenMedia(streamCtx, library, item, byteRange)
		if err != nil {
			return nil, access.ErrDenied
		}
		if stream.Body == nil {
			return nil, access.ErrDenied
		}
		if err = ready(PlaybackMetadata{
			Status: stream.Status, ContentType: stream.ContentType,
			ContentLength: stream.ContentLength, ContentRange: stream.ContentRange,
		}); err != nil {
			stream.Body.Close()
			return nil, access.ErrDenied
		}
		return stream.Body, nil
	})
}
