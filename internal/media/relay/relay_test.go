package relay

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestRelayLeaseIsolationCancellationAndLimits(t *testing.T) {
	b := New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = b.Request(ctx, "one", Task{Expires: time.Now().Add(time.Minute)}, func(context.Context) error { return nil })
	}()
	var task Task
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if _, ok := b.Poll("two", "credential"); ok {
			t.Fatal("cross-tenant lease")
		}
		var ok bool
		task, ok = b.Poll("one", "credential")
		if ok {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if task.ID == "" {
		t.Fatal("missing lease")
	}
	if _, ok := b.Poll("one", "other"); ok {
		t.Fatal("task leased twice")
	}
	if b.Check(ctx, "one", "other", task.ID) == nil {
		t.Fatal("wrong connector accepted")
	}
	if b.Deliver(ctx, "two", "credential", task.ID, Response{Body: io.NopCloser(strings.NewReader("x"))}) == nil {
		t.Fatal("cross-tenant upload")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request did not cancel")
	}
	if b.Check(context.Background(), "one", "credential", task.ID) == nil {
		t.Fatal("cancelled lease accepted")
	}
}
