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

// Collector groups observations by MAC. Safe for concurrent use.
type Collector struct {
	mu      sync.Mutex
	byMAC   map[string]*Observation
	dropped int
}

func NewCollector() *Collector { return &Collector{byMAC: map[string]*Observation{}} }

// Add merges an observation. It reports whether it was kept.
func (c *Collector) Add(o Observation) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	cur, ok := c.byMAC[o.MAC]
	if !ok {
		if len(c.byMAC) >= MaxCandidates {
			c.dropped++
			return false
		}
		cp := o
		c.byMAC[o.MAC] = &cp
		return true
	}
	if o.LastSeen.Sub(cur.LastSeen) < MinUpdateInterval {
		return false
	}
	cur.Count++
	cur.LastSeen = o.LastSeen
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
	for mac, o := range c.byMAC {
		if now.Sub(o.LastSeen) > ttl {
			delete(c.byMAC, mac)
		}
	}
}

// Dropped counts observations refused because the candidate limit was reached.
func (c *Collector) Dropped() int { c.mu.Lock(); defer c.mu.Unlock(); return c.dropped }

// Snapshot returns copies sorted by MAC.
func (c *Collector) Snapshot() []Observation {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Observation, 0, len(c.byMAC))
	for _, o := range c.byMAC {
		out = append(out, *o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MAC < out[j].MAC })
	return out
}
