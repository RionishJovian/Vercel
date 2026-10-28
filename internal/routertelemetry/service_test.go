package routertelemetry

import (
	"context"
	"errors"
	"mega-magic/internal/provisioner/routeros"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type readerFunc func(context.Context, string, string) (routeros.RouterTelemetry, error)

func (f readerFunc) ReadTelemetry(c context.Context, b, d string) (routeros.RouterTelemetry, error) {
	return f(c, b, d)
}
func ptr[T any](v T) *T { return &v }
func sample(at time.Time, device string, uptime, rx, tx uint64) routeros.RouterTelemetry {
	return routeros.RouterTelemetry{DeviceID: device, ReadAt: at, Observed: map[string]time.Time{"cpu": at, "memory": at, "interfaces": at}, Resources: routeros.RouterResources{Uptime: ptr(uptime), CPUPercent: ptr(0.0), MemoryTotal: ptr(uint64(1024)), MemoryUsed: ptr(uint64(512))}, Interfaces: []routeros.InterfaceTelemetry{{ID: "*1", Name: "ether1", Type: "ether", RXBytes: ptr(rx), TXBytes: ptr(tx)}}, Issues: []string{}}
}
func TestTelemetryRatesRetentionAndTenantIsolation(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	fail := false
	calls := 0
	service := New(readerFunc(func(_ context.Context, b, d string) (routeros.RouterTelemetry, error) {
		calls++
		if fail {
			return routeros.RouterTelemetry{}, errors.New("private key detail")
		}
		return sample(now, d, uint64(calls*100), uint64(calls*1000), uint64(calls*2000)), nil
	}))
	service.clock = func() time.Time { return now }
	first, _ := service.Read(context.Background(), "tenant-a", "router")
	if first.State != "live" || first.Snapshot.Interfaces[0].RXBitsPerSecond != nil {
		t.Fatal("first sample invented rate")
	}
	// Mutating a response cannot poison another caller's retained reading.
	*first.Snapshot.Resources.CPUPercent = 99
	*first.History[0].CPUPercent = 99
	same, _ := service.Read(context.Background(), "tenant-a", "router")
	if calls != 1 || *same.Snapshot.Resources.CPUPercent != 0 || *same.History[0].CPUPercent != 0 {
		t.Fatal("cache aliases caller data")
	}
	now = now.Add(15 * time.Second)
	second, _ := service.Read(context.Background(), "tenant-a", "router")
	if second.Snapshot.Interfaces[0].RXBitsPerSecond == nil || *second.Snapshot.Interfaces[0].RXBitsPerSecond != 8000.0/15 {
		t.Fatal("rate not derived from two readings")
	}
	fail = true
	now = now.Add(15 * time.Second)
	lost, _ := service.Read(context.Background(), "tenant-a", "router")
	if lost.State != "reconnecting" || lost.LastSuccessAt == nil || !lost.Snapshot.ReadAt.Equal(second.Snapshot.ReadAt) || len(lost.History) != 2 {
		t.Fatal("confirmed reading lost or refreshed on failure")
	}
	foreign, _ := service.Read(context.Background(), "tenant-b", "router")
	if foreign.Snapshot != nil || foreign.LastSuccessAt != nil {
		t.Fatal("cross-tenant cached data")
	}
}
func TestTelemetryResetAndSamplingGapsSuppressRates(t *testing.T) {
	now := time.Now().UTC()
	before := sample(now, "a", 100, 1000, 1000)
	for _, next := range []routeros.RouterTelemetry{sample(now.Add(15*time.Second), "a", 1, 2000, 2000), sample(now.Add(15*time.Second), "a", 115, 1, 1), sample(now.Add(3*time.Minute), "a", 280, 2000, 2000)} {
		applyRates(&next, &before)
		if next.Interfaces[0].RXBitsPerSecond != nil {
			t.Fatal("reboot/reset/gap produced a rate")
		}
	}
}
func TestTelemetryConcurrentReadersShareBoundedCollection(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{})
	now := time.Now().UTC()
	service := New(readerFunc(func(context.Context, string, string) (routeros.RouterTelemetry, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return sample(now, "router", 100, 0, 0), nil
	}))
	var group sync.WaitGroup
	group.Add(8)
	for range 8 {
		go func() {
			defer group.Done()
			if _, err := service.Read(context.Background(), "tenant", "router"); err != nil {
				t.Error(err)
			}
		}()
	}
	<-started
	close(release)
	group.Wait()
	if calls.Load() != 1 {
		t.Fatal("overlapping router polls", calls.Load())
	}
}
func TestTelemetryRejectsWrongIdentityAndBoundsCache(t *testing.T) {
	now := time.Now().UTC()
	service := New(readerFunc(func(_ context.Context, b, d string) (routeros.RouterTelemetry, error) {
		return sample(now, "wrong-router", 1, 0, 0), nil
	}))
	service.capacity = 2
	for _, device := range []string{"a", "b", "c"} {
		view, _ := service.Read(context.Background(), "tenant", device)
		if view.Snapshot != nil || view.State != "reconnecting" {
			t.Fatal("wrong router accepted")
		}
	}
	if len(service.entries) != 2 {
		t.Fatal("cache unbounded")
	}
}

func TestTelemetryPartialFailureKeepsPerMetricTimes(t *testing.T) {
	now := time.Now().UTC()
	calls := 0
	firstAt := now
	service := New(readerFunc(func(_ context.Context, b, d string) (routeros.RouterTelemetry, error) {
		calls++
		current := sample(now, d, uint64(calls*100), uint64(calls*1000), 1000)
		if calls > 1 {
			current.Interfaces = nil
			delete(current.Observed, "interfaces")
			current.Resources.CPUPercent = nil
			delete(current.Observed, "cpu")
			current.Issues = []string{"cpu_read_pending", "interface_read_pending"}
		}
		return current, nil
	}))
	service.clock = func() time.Time { return now }
	service.Read(context.Background(), "tenant", "router")
	now = now.Add(15 * time.Second)
	view, _ := service.Read(context.Background(), "tenant", "router")
	if view.State != "syncing" || view.Snapshot.Resources.CPUPercent == nil || !view.Snapshot.Observed["cpu"].Equal(firstAt) || !view.Snapshot.Observed["interfaces"].Equal(firstAt) {
		t.Fatal("lost the original metric times")
	}
	if view.History[1].CPUPercent != nil {
		t.Fatal("stale CPU became a new measured point")
	}
	if len(view.Snapshot.Interfaces) != 1 || view.Snapshot.Interfaces[0].RXBitsPerSecond != nil {
		t.Fatal("stale counter received a new rate")
	}
}
