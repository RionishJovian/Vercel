package topology

import (
	"testing"

	"mega-magic/internal/model"
)

func dev(id, parent string, layer string, online bool) *model.Device {
	var p *string
	if parent != "" {
		p = &parent
	}
	cat := "router"
	if layer == "DOWNSTREAM_SWITCH" {
		cat = "switch"
	}
	if layer == "ACCESS_POINT" {
		cat = "ap"
	}
	return &model.Device{
		DeviceID:        id,
		BusinessID:      "biz",
		ParentGatewayID: p,
		HardwareName:    id + "-hw",
		DeploymentLayer: layer,
		Category:        cat,
		IsOnline:        online,
	}
}

func TestBuildMap_buildsTree(t *testing.T) {
	devices := []*model.Device{
		dev("gw", "", "MAIN_GATEWAY", true),
		dev("sw", "gw", "DOWNSTREAM_SWITCH", true),
		dev("ap", "sw", "ACCESS_POINT", false),
	}
	m := BuildMap("biz", devices)
	if len(m.Roots) != 1 || m.Roots[0].Device.DeviceID != "gw" {
		t.Fatalf("roots = %d, want single gw root", len(m.Roots))
	}
	sw := m.Roots[0].Children
	if len(sw) != 1 || sw[0].Device.DeviceID != "sw" {
		t.Fatalf("gw children wrong: %+v", m.Roots[0].Children)
	}
	ap := sw[0].Children
	if len(ap) != 1 || ap[0].Device.DeviceID != "ap" {
		t.Fatalf("sw children wrong: %+v", sw[0].Children)
	}
	if len(m.Orphans) != 0 || len(m.Cycles) != 0 {
		t.Fatalf("unexpected orphans/cycles: %+v / %+v", m.Orphans, m.Cycles)
	}
	if m.Summary.TotalDevices != 3 || m.Summary.InTree != 3 || m.Summary.Online != 2 {
		t.Fatalf("summary wrong: %+v", m.Summary)
	}
}

func TestBuildMap_multipleRoots_sorted(t *testing.T) {
	devices := []*model.Device{
		dev("gwB", "", "MAIN_GATEWAY", true),
		dev("gwA", "", "MAIN_GATEWAY", true),
	}
	m := BuildMap("biz", devices)
	if len(m.Roots) != 2 {
		t.Fatalf("roots = %d, want 2", len(m.Roots))
	}
	if m.Roots[0].Device.DeviceID != "gwA" || m.Roots[1].Device.DeviceID != "gwB" {
		t.Fatalf("roots not sorted: %s, %s", m.Roots[0].Device.DeviceID, m.Roots[1].Device.DeviceID)
	}
}

func TestBuildMap_orphan_foreignParent(t *testing.T) {
	// ap's parent (foreign-switch) is NOT in this tenant's device set -> orphan.
	devices := []*model.Device{
		dev("gw", "", "MAIN_GATEWAY", true),
		dev("ap", "not-in-tenant", "ACCESS_POINT", false),
	}
	m := BuildMap("biz", devices)
	if len(m.Orphans) != 1 || m.Orphans[0].DeviceID != "ap" {
		t.Fatalf("orphans = %+v, want [ap]", m.Orphans)
	}
	if len(m.Roots) != 1 || m.Roots[0].Device.DeviceID != "gw" {
		t.Fatalf("roots = %+v, want only gw in tree", m.Roots)
	}
	if m.Summary.InTree != 1 {
		t.Fatalf("in-tree = %d, want 1 (orphan excluded)", m.Summary.InTree)
	}
}

func TestBuildMap_cycle_selfLoop(t *testing.T) {
	devices := []*model.Device{
		dev("gw", "", "MAIN_GATEWAY", true),
		dev("ap", "ap", "ACCESS_POINT", false), // self-loop
	}
	m := BuildMap("biz", devices)
	if len(m.Cycles) != 1 {
		t.Fatalf("cycles = %+v, want 1 self-loop", m.Cycles)
	}
	// self-loop flagged, ap excluded from tree, gw unaffected.
	if len(m.Roots) != 1 || m.Roots[0].Device.DeviceID != "gw" {
		t.Fatalf("roots = %+v, want only gw", m.Roots)
	}
	if m.Summary.InTree != 1 {
		t.Fatalf("in-tree = %d, want 1 (self-loop excluded)", m.Summary.InTree)
	}
}

func TestBuildMap_cycle_twoNode_enteredAtAnyPoint(t *testing.T) {
	// a -> b -> a is a 2-cycle. Detection must yield ONE cycle regardless of the
	// traversal entry point (order-independence of the device slice).
	devices := []*model.Device{
		dev("a", "b", "MAIN_GATEWAY", true),
		dev("b", "a", "DOWNSTREAM_SWITCH", true),
	}
	m := BuildMap("biz", devices)
	if len(m.Cycles) != 1 {
		t.Fatalf("cycles = %+v, want exactly 1", m.Cycles)
	}
	if len(m.Roots) != 0 {
		t.Fatalf("roots = %+v, want 0 (both on cycle)", m.Roots)
	}
	if m.Summary.InTree != 0 {
		t.Fatalf("in-tree = %d, want 0", m.Summary.InTree)
	}
}

func TestBuildMap_empty(t *testing.T) {
	m := BuildMap("biz", nil)
	if len(m.Roots) != 0 || len(m.Orphans) != 0 || len(m.Cycles) != 0 {
		t.Fatalf("empty map should be empty")
	}
	if m.Summary.TotalDevices != 0 || m.Summary.InTree != 0 || m.Summary.Online != 0 {
		t.Fatalf("empty summary wrong: %+v", m.Summary)
	}
	// maps must be initialized (never nil) so JSON serializes {} not null.
	if m.Summary.ByLayer == nil || m.Summary.ByCategory == nil {
		t.Fatal("summary maps must be non-nil")
	}
}

func TestBuildMap_noMutation(t *testing.T) {
	devices := []*model.Device{
		dev("gw", "", "MAIN_GATEWAY", true),
		dev("sw", "gw", "DOWNSTREAM_SWITCH", true),
	}
	snapshot := make([]*model.Device, len(devices))
	copy(snapshot, devices)
	BuildMap("biz", devices)
	for i := range snapshot {
		if snapshot[i].DeviceID != devices[i].DeviceID {
			t.Fatal("BuildMap mutated input slice")
		}
	}
}