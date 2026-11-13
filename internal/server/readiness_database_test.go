package server

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestDatabaseReadinessBoundsUnresponsiveDriverAndLimitsConcurrency(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	probe := boundedDatabaseProbe(func(context.Context) error { calls.Add(1); <-release; return nil })
	result := make(chan error, 1)
	go func() { result <- probe() }()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("unresponsive database reported ready")
		}
	case <-time.After(3500 * time.Millisecond):
		t.Fatal("database probe did not return within its budget")
	}
	// An unresponsive driver may ignore cancellation. Retrying readiness must
	// report unavailable without starting one leaked database goroutine per poll.
	for i := 0; i < 10; i++ {
		if err := probe(); err == nil {
			t.Fatal("busy probe reported healthy")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("started %d stalled probes", calls.Load())
	}
}
func TestDatabaseReadinessHealthyAndFailure(t *testing.T) {
	if err := boundedDatabaseProbe(func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("missing DB deadline")
		}
		return nil
	})(); err != nil {
		t.Fatal(err)
	}
	if err := boundedDatabaseProbe(func(context.Context) error { return errors.New("private connection failure") })(); err == nil {
		t.Fatal("database failure was ignored")
	}
}
