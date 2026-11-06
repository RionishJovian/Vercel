package discovery

import (
	"sort"
	"sync"
	"time"
)

const (
	// MaxCandidates bounds memory against a flood of forged MAC addresses.
	MaxCandidates = 64
	// MinUpdateInterval stops one MAC from being re-processed faster than a real router announces.
	MinUpdateInterval = time.Second
)

// Collector groups observations by device (software-id hash), falling back to MAC. Safe for concurrent use.
type Collector struct {
	mu      sync.Mutex
	byKey   map[string]*Observation
	dropped int
}

func NewCollector() *Collector { return &Collector{byKey: map[string]*Observation{}} }

func keyOf(o Observation) string {
	if o.DeviceKey != "" {
		return o.DeviceKey
	}
	return "mac:" + o.MAC
}

func addMAC(list []string, mac string) []string {
	for _, m := range list {
		if m == mac {
			return list
		}
	}
	return append(list, mac)
}

// Add merges an observation. It reports whether it was kept.
func (c *Collector) Add(o Observation) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := keyOf(o)
	cur, ok := c.byKey[key]
	if !ok {
		if len(c.byKey) >= MaxCandidates {
			c.dropped++
			return false
		}
		cp := o
		c.byKey[key] = &cp
		return true
	}
	if o.LastSeen.Sub(cur.LastSeen) < MinUpdateInterval {
		return false
	}
	cur.Count++
	cur.LastSeen = o.LastSeen
	cur.MAC = o.MAC
	cur.MACs = addMAC(cur.MACs, o.MAC)
	if ip := o.SourceIP; ip != "" && cur.SourceIP != "" && ip != cur.SourceIP {
		cur.IPChanged = true
	}
	// The newest announcement is the best evidence of the current state.
	cur.Identity, cur.Board, cur.Platform, cur.Version, cur.Interface = o.Identity, o.Board, o.Platform, o.Version, o.Interface
	cur.ClaimedIP, cur.SourceIP, cur.IPMismatch = o.ClaimedIP, o.SourceIP, o.IPMismatch
	return true
}

// Expire removes observations not heard within ttl.
func (c *Collector) Expire(now time.Time, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, o := range c.byKey {
		if now.Sub(o.LastSeen) > ttl {
			delete(c.byKey, key)
		}
	}
}

// Dropped counts observations refused because the candidate limit was reached.
func (c *Collector) Dropped() int { c.mu.Lock(); defer c.mu.Unlock(); return c.dropped }

// Snapshot returns copies sorted by MAC.
func (c *Collector) Snapshot() []Observation {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Observation, 0, len(c.byKey))
	for _, o := range c.byKey {
		out = append(out, *o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MAC < out[j].MAC })
	return out
}
