// Package routertelemetry retains bounded, tenant/device-scoped router readings.
// No samples are accepted from the browser and no router writes are performed.
package routertelemetry

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"mega-magic/internal/provisioner/routeros"
)

type Reader interface {
	ReadTelemetry(context.Context, string, string) (routeros.RouterTelemetry, error)
}
type Point struct {
	ReadAt        time.Time `json:"read_at"`
	CPUPercent    *float64  `json:"cpu_percent"`
	MemoryPercent *float64  `json:"memory_percent"`
}
type View struct {
	DeviceID          string                    `json:"device_id"`
	Source            string                    `json:"source"`
	State             string                    `json:"state"`
	CheckedAt         time.Time                 `json:"checked_at"`
	LastSuccessAt     *time.Time                `json:"last_success_at"`
	RetryAfterSeconds int                       `json:"retry_after_seconds"`
	Reason            string                    `json:"reason,omitempty"`
	Snapshot          *routeros.RouterTelemetry `json:"snapshot"`
	History           []Point                   `json:"history"`
}
type key struct{ business, device string }
type entry struct {
	view       View
	next, last time.Time
	flight     chan struct{}
}
type Service struct {
	mu       sync.Mutex
	reader   Reader
	clock    func() time.Time
	entries  map[key]*entry
	interval time.Duration
	capacity int
}

func New(reader Reader) *Service {
	return &Service{reader: reader, clock: time.Now, entries: map[key]*entry{}, interval: 15 * time.Second, capacity: 64}
}

func (s *Service) Read(ctx context.Context, business, device string) (View, error) {
	if business == "" || device == "" || s.reader == nil {
		return View{}, errors.New("telemetry requires tenant and device")
	}
	k := key{business, device}
	for {
		s.mu.Lock()
		now := s.clock().UTC()
		e := s.entries[k]
		if e != nil && e.flight != nil {
			wait := e.flight
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return View{}, ctx.Err()
			case <-wait:
				continue
			}
		}
		if e != nil && now.Before(e.next) {
			e.last = now
			view := clone(e.view)
			s.mu.Unlock()
			return view, nil
		}
		if e == nil {
			if len(s.entries) >= s.capacity {
				var oldest key
				var at time.Time
				found := false
				for candidate, item := range s.entries {
					if item.flight == nil && (!found || item.last.Before(at)) {
						oldest = candidate
						at = item.last
						found = true
					}
				}
				if !found {
					s.mu.Unlock()
					return View{}, errors.New("telemetry busy")
				}
				delete(s.entries, oldest)
			}
			e = &entry{view: View{DeviceID: device, Source: "routeros-ssh", State: "connecting", History: []Point{}}}
			s.entries[k] = e
		}
		e.flight = make(chan struct{})
		e.last = now
		previous := e.view.Snapshot
		s.mu.Unlock()
		deadline, cancel := context.WithTimeout(ctx, 12*time.Second)
		sample, err := s.reader.ReadTelemetry(deadline, business, device)
		cancel()
		now = s.clock().UTC()
		if err == nil && (sample.DeviceID != device || sample.ReadAt.IsZero() || sample.ReadAt.After(now.Add(5*time.Second)) || now.Sub(sample.ReadAt) > 30*time.Second) {
			err = errors.New("telemetry identity or time mismatch")
		}
		s.mu.Lock()
		e.view.CheckedAt = now
		e.view.RetryAfterSeconds = int(s.interval.Seconds())
		e.view.Reason = ""
		if err != nil {
			e.view.State = "reconnecting"
			e.view.Reason = "router_read_retrying"
		} else {
			mergeSources(&sample, previous)
			applyRates(&sample, previous)
			e.view.State = "live"
			if len(sample.Issues) > 0 {
				e.view.State = "syncing"
			}
			e.view.Snapshot = &sample
			at := sample.ReadAt
			e.view.LastSuccessAt = &at
			point := Point{ReadAt: sample.ReadAt}
			if sample.Observed["cpu"].Equal(sample.ReadAt) {
				point.CPUPercent = sample.Resources.CPUPercent
			}
			if total, used := sample.Resources.MemoryTotal, sample.Resources.MemoryUsed; total != nil && used != nil && *total > 0 && sample.Observed["memory"].Equal(sample.ReadAt) {
				n := float64(*used) * 100 / float64(*total)
				point.MemoryPercent = &n
			}
			e.view.History = append(e.view.History, point)
			if len(e.view.History) > 60 {
				e.view.History = append([]Point(nil), e.view.History[len(e.view.History)-60:]...)
			}
		}
		e.next = now.Add(s.interval)
		close(e.flight)
		e.flight = nil
		view := clone(e.view)
		s.mu.Unlock()
		return view, nil
	}
}
func mergeSources(current, previous *routeros.RouterTelemetry) {
	if previous == nil {
		return
	}
	if current.Observed == nil {
		current.Observed = map[string]time.Time{}
	}
	for _, source := range []string{"cpu", "memory", "interfaces", "hotspot", "dhcp"} {
		if !current.Observed[source].IsZero() || previous.Observed[source].IsZero() {
			continue
		}
		current.Observed[source] = previous.Observed[source]
		switch source {
		case "cpu":
			current.Resources.CPUPercent = previous.Resources.CPUPercent
		case "memory":
			current.Resources.MemoryTotal = previous.Resources.MemoryTotal
			current.Resources.MemoryUsed = previous.Resources.MemoryUsed
		case "interfaces":
			current.Interfaces = append([]routeros.InterfaceTelemetry(nil), previous.Interfaces...)
		case "hotspot":
			current.HotspotSessions = previous.HotspotSessions
		case "dhcp":
			current.DHCPLeases = previous.DHCPLeases
		}
	}
}
func applyRates(current, previous *routeros.RouterTelemetry) {
	if previous == nil || current.Resources.Uptime == nil || previous.Resources.Uptime == nil || *current.Resources.Uptime < *previous.Resources.Uptime {
		return
	}
	if current.Observed["interfaces"].IsZero() || previous.Observed["interfaces"].IsZero() {
		return
	}
	elapsed := current.Observed["interfaces"].Sub(previous.Observed["interfaces"]).Seconds()
	if elapsed < 1 || elapsed > 120 {
		return
	}
	old := map[string]routeros.InterfaceTelemetry{}
	for _, item := range previous.Interfaces {
		old[item.ID] = item
	}
	for i := range current.Interfaces {
		item := &current.Interfaces[i]
		before, ok := old[item.ID]
		if !ok || item.Name != before.Name || item.Type != before.Type {
			continue
		}
		rate := func(after, before *uint64) *float64 {
			if after == nil || before == nil || *after < *before {
				return nil
			}
			n := float64(*after-*before) * 8 / elapsed
			if math.IsInf(n, 0) || math.IsNaN(n) {
				return nil
			}
			return &n
		}
		item.RXBitsPerSecond = rate(item.RXBytes, before.RXBytes)
		item.TXBitsPerSecond = rate(item.TXBytes, before.TXBytes)
	}
}

// Callers receive detached values so neither handlers nor tests can mutate cache.
func copyPointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
func clone(view View) View {
	out := view
	out.LastSuccessAt = copyPointer(view.LastSuccessAt)
	out.History = append([]Point{}, view.History...)
	for i := range out.History {
		out.History[i].CPUPercent = copyPointer(out.History[i].CPUPercent)
		out.History[i].MemoryPercent = copyPointer(out.History[i].MemoryPercent)
	}
	if view.Snapshot != nil {
		sample := *view.Snapshot
		sample.Observed = map[string]time.Time{}
		for source, at := range view.Snapshot.Observed {
			sample.Observed[source] = at
		}
		sample.Resources.CPUPercent = copyPointer(sample.Resources.CPUPercent)
		sample.Resources.MemoryTotal = copyPointer(sample.Resources.MemoryTotal)
		sample.Resources.MemoryUsed = copyPointer(sample.Resources.MemoryUsed)
		sample.Resources.Uptime = copyPointer(sample.Resources.Uptime)
		sample.HotspotSessions = copyPointer(sample.HotspotSessions)
		sample.DHCPLeases = copyPointer(sample.DHCPLeases)
		sample.Interfaces = append([]routeros.InterfaceTelemetry(nil), sample.Interfaces...)
		for i := range sample.Interfaces {
			row := &sample.Interfaces[i]
			row.Running = copyPointer(row.Running)
			row.Disabled = copyPointer(row.Disabled)
			row.RXBytes = copyPointer(row.RXBytes)
			row.TXBytes = copyPointer(row.TXBytes)
			row.RXBitsPerSecond = copyPointer(row.RXBitsPerSecond)
			row.TXBitsPerSecond = copyPointer(row.TXBitsPerSecond)
		}
		sample.Issues = append([]string{}, sample.Issues...)
		out.Snapshot = &sample
	}
	return out
}
