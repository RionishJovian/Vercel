// Package topology builds and serves the per-tenant network map derived from
// the devices table's parent_gateway_id links.
//
// The map itself is a pure function of a device slice — no I/O. Keeping the
// tree/cycle/orphan math here makes it directly unit-testable without a DB,
// mirroring the fleet aggregate pattern. The handler is the only I/O boundary.
package topology

import (
	"sort"

	"mega-magic/internal/model"
)

// Map is the per-tenant network graph. Roots are the top of the parent/child
// tree (devices with no in-tenant parent, i.e. gateways/uplinks). Orphans are
// devices whose parent_gateway_id points at a device NOT present in the
// tenant's device set (a broken/foreign link). Cycles are parent chains that
// loop back on themselves — a data-integrity fault surfaced rather than a crash.
type Map struct {
	BusinessID string          `json:"business_id"`
	Roots      []*Node         `json:"roots"`
	Orphans    []*model.Device `json:"orphans,omitempty"`
	Cycles     [][]string      `json:"cycles,omitempty"`
	Summary    Summary         `json:"summary"`
}

// Node is one device in the tree with its in-tenant direct children.
type Node struct {
	Device   *model.Device `json:"device"`
	Children []*Node       `json:"children,omitempty"`
}

// Summary is the map's quick-glance counts for the fleet-dashboard header.
type Summary struct {
	TotalDevices int            `json:"total_devices"`
	InTree       int            `json:"in_tree"`
	Online       int            `json:"online"`
	ByLayer      map[string]int `json:"by_layer"`
	ByCategory   map[string]int `json:"by_category"`
}

// BuildMap folds a tenant's device slice into a Map. devices must already be
// tenant-scoped (the caller enforces RLS). It never mutates its input.
func BuildMap(businessID string, devices []*model.Device) *Map {
	m := &Map{
		BusinessID: businessID,
		Summary: Summary{
			TotalDevices: len(devices),
			ByLayer:      map[string]int{},
			ByCategory:   map[string]int{},
		},
	}

	byID := make(map[string]*model.Device, len(devices))
	for _, d := range devices {
		byID[d.DeviceID] = d
	}

	for _, d := range devices {
		m.Summary.ByLayer[d.DeploymentLayer]++
		m.Summary.ByCategory[d.Category]++
		if d.IsOnline {
			m.Summary.Online++
		}
	}

	// Cycle detection — a chain of parent links that loops back on itself.
	// Because each device has at most one parent, a cycle is detected by walking
	// the parent chain and seeing a node twice. Nodes on a cycle are excluded
	// from the tree and surfaced under Cycles (canonical order).
	cycles := detectCycles(devices, byID)
	cycleSet := map[string]bool{}
	for _, cyc := range cycles {
		sorted := append([]string{}, cyc...)
		sort.Strings(sorted)
		for _, id := range sorted {
			cycleSet[id] = true
		}
	}
	m.Cycles = cycles

	// Classify: orphan (parent missing/foreign/self) vs in-tree node.
	orphanSet := map[string]bool{}
	var orphans []*model.Device
	for _, d := range devices {
		if cycleSet[d.DeviceID] {
			continue
		}
		if d.ParentGatewayID == nil || *d.ParentGatewayID == "" {
			continue // root candidate
		}
		if _, ok := byID[*d.ParentGatewayID]; !ok {
			orphanSet[d.DeviceID] = true
			orphans = append(orphans, d)
		}
	}
	m.Orphans = orphans

	// Build the tree from nodes not on a cycle and not orphans.
	built := map[string]*Node{}
	for _, d := range devices {
		if cycleSet[d.DeviceID] || orphanSet[d.DeviceID] {
			continue
		}
		built[d.DeviceID] = &Node{Device: d}
	}
	for _, node := range built {
		m.Summary.InTree++
		if pID := node.Device.ParentGatewayID; pID != nil {
			if parentNode, ok := built[*pID]; ok {
				parentNode.Children = append(parentNode.Children, node)
			}
		}
	}
	for _, node := range built {
		if node.Device.ParentGatewayID == nil || built[*node.Device.ParentGatewayID] == nil {
			m.Roots = append(m.Roots, node)
		}
	}
	sortNodes(m.Roots)
	return m
}

// detectCycles returns the parent-link loops as [][]string of device_ids, each
// entry being one distinct cycle's member IDs. Deterministic: purely functional.
func detectCycles(devices []*model.Device, byID map[string]*model.Device) [][]string {
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := map[string]int{}
	var path []string
	var cycles [][]string

	var visit func(string)
	visit = func(id string) {
		switch state[id] {
		case visiting:
			// id is already on the current path -> a cycle. The loop is the path
			// segment from id's first occurrence to the end (cycle closes to id).
			start := indexOf(path, id)
			loop := append([]string{}, path[start:]...)
			loop = append(loop, id)
			if !hasCycle(cycles, loop) {
				cycles = append(cycles, loop)
			}
			return
		case done:
			return
		}
		if _, missing := byID[id]; !missing {
			state[id] = done
			return
		}
		state[id] = visiting
		path = append(path, id)
		d := byID[id]
		if d.ParentGatewayID != nil {
			pid := *d.ParentGatewayID
			if pid == id {
				if !hasCycle(cycles, []string{id}) {
					cycles = append(cycles, []string{id}) // self-loop
				}
			} else if parent, ok := byID[pid]; ok {
				visit(parent.DeviceID)
			}
		}
		path = path[:len(path)-1]
		state[id] = done
	}

	for _, d := range devices {
		visit(d.DeviceID)
	}
	return cycles
}

func hasCycle(cycles [][]string, cand []string) bool {
	for _, c := range cycles {
		if sameSet(c, cand) {
			return true
		}
	}
	return false
}

// sameSet reports whether two loops are the SAME cycle by their set of member
// IDs (order-agnostic, duplicate-tolerant). A loop entered at a different point
// carries the same nodes, so identity is by members — not by traversal order.
func sameSet(a, b []string) bool {
	ma := memberSet(a)
	mb := memberSet(b)
	if len(ma) != len(mb) {
		return false
	}
	for i := range ma {
		if ma[i] != mb[i] {
			return false
		}
	}
	return true
}

func memberSet(loop []string) []string {
	m := map[string]bool{}
	for _, id := range loop {
		m[id] = true
	}
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func indexOf(path []string, id string) int {
	for i, p := range path {
		if p == id {
			return i
		}
	}
	return 0
}

func sortNodes(nodes []*Node) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Device.DeviceID < nodes[j].Device.DeviceID
	})
	for _, n := range nodes {
		sortNodes(n.Children)
	}
}
