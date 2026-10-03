package access

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPlaybackExpiryInterruptsBlockedStream(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	now := time.Now()
	var out bytes.Buffer
	_, e := copyChecked(context.Background(), &out, now.Add(100*time.Millisecond), 20*time.Millisecond, func(context.Context) error { return nil }, func(context.Context) (io.ReadCloser, error) { return reader, nil })
	if !errors.Is(e, ErrDenied) || time.Since(now) > time.Second {
		t.Fatal("blocked stream did not end at expiry", e)
	}
}

func TestPlaybackRefundAndCloudOutageCloseStream(t *testing.T) {
	for _, reason := range []string{"refunded", "cloud unavailable"} {
		t.Run(reason, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer writer.Close()
			var checks atomic.Int32
			var out bytes.Buffer
			_, e := copyChecked(context.Background(), &out, time.Now().Add(time.Hour), 20*time.Millisecond, func(context.Context) error {
				if checks.Add(1) > 1 {
					return errors.New(reason)
				}
				return nil
			}, func(context.Context) (io.ReadCloser, error) { return reader, nil })
			if !errors.Is(e, ErrDenied) {
				t.Fatal("revocation failed open", e)
			}
		})
	}
}

func TestPlaybackDenialBeforeSourceAndNormalTransfer(t *testing.T) {
	var opens int
	var out bytes.Buffer
	open := func(context.Context) (io.ReadCloser, error) {
		opens++
		return io.NopCloser(strings.NewReader("test media bytes")), nil
	}
	n, e := Copy(context.Background(), &out, time.Now().Add(time.Hour), func(context.Context) error { return errors.New("unpaid") }, open)
	if n != 0 || !errors.Is(e, ErrDenied) || opens != 0 {
		t.Fatal("unpaid media source opened")
	}
	n, e = Copy(context.Background(), &out, time.Now().Add(time.Hour), func(context.Context) error { return nil }, open)
	if e != nil || n != 16 || out.String() != "test media bytes" {
		t.Fatal("valid media transfer failed", e)
	}
	for _, expiry := range []time.Time{time.Now().Add(-time.Second), time.Time{}, time.Now().Add(31 * 24 * time.Hour)} {
		if _, e = Copy(context.Background(), &out, expiry, func(context.Context) error { return nil }, open); !errors.Is(e, ErrDenied) {
			t.Fatal("invalid deadline accepted")
		}
	}
	if opens != 1 {
		t.Fatal("invalid deadline opened local source")
	}
}
