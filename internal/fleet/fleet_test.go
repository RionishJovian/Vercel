package fleet

import "testing"

func TestAggregateHealth_counts(t *testing.T) {
	peers := []PeerSummary{
		{DeviceID: "p1", State: StateUp},
		{DeviceID: "p2", State: StateUp},
		{DeviceID: "p3", State: StateStale},
		{DeviceID: "p4", State: StateOffline},
		{DeviceID: "p5", State: StateUnknown},
	}
	s := AggregateHealth("biz-1", peers)
	if s.Total != 5 {
		t.Fatalf("total = %d, want 5", s.Total)
	}
	if s.Up != 2 || s.Stale != 1 || s.Offline != 1 || s.Unknown != 1 {
		t.Fatalf("counts = up:%d stale:%d offline:%d unknown:%d, want 2/1/1/1", s.Up, s.Stale, s.Offline, s.Unknown)
	}
}

func TestAggregateHealth_overall_is_worst_state(t *testing.T) {
	cases := []struct {
		name  string
		peers []PeerSummary
		want  string
	}{
		{"empty fleet is unknown", nil, StateUnknown},
		{"all up", []PeerSummary{{"a", "", StateUp}}, StateUp},
		{"offline dominates", []PeerSummary{{"a", "", StateUp}, {"b", "", StateOffline}}, StateOffline},
		{"stale dominates up", []PeerSummary{{"a", "", StateUp}, {"b", "", StateStale}}, StateStale},
		{"unknown does not mask up", []PeerSummary{{"a", "", StateUp}, {"b", "", StateUnknown}}, StateUp},
		{"offline over stale over up", []PeerSummary{{"a", "", StateUp}, {"b", "", StateStale}, {"c", "", StateOffline}}, StateOffline},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := AggregateHealth("biz", tc.peers).Overall
			if got != tc.want {
				t.Fatalf("overall = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAggregateHealth_peers_preserved(t *testing.T) {
	peers := []PeerSummary{
		{DeviceID: "p1", HardwareName: "RB5009", State: StateUp},
		{DeviceID: "p2", HardwareName: "hEX S", State: StateOffline},
	}
	s := AggregateHealth("biz", peers)
	if len(s.Peers) != 2 {
		t.Fatalf("peers len = %d, want 2", len(s.Peers))
	}
	if s.Peers[1].State != StateOffline || s.Peers[1].HardwareName != "hEX S" {
		t.Fatalf("peer[1] not preserved: %+v", s.Peers[1])
	}
}

func TestSeverity_ordering(t *testing.T) {
	if severity(StateUp) <= severity(StateUnknown) {
		t.Fatal("up should be more severe than unknown")
	}
	if severity(StateStale) <= severity(StateUp) {
		t.Fatal("stale should be more severe than up")
	}
	if severity(StateOffline) <= severity(StateStale) {
		t.Fatal("offline should be most severe")
	}
}