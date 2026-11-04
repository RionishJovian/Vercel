package server

import (
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
	"testing"
)

func TestHubConnectionStateLabelsAreNotConnectivityProof(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta database.PendingEnrollmentStatus
		want handler.HubConnectionState
	}{
		{"saved credentials", database.PendingEnrollmentStatus{ManagementSaved: true, RouterKey: "expired", Management: "expired"}, handler.HubConnectionState{Management: "saved", Setup: "not_required"}},
		{"no attempt", database.PendingEnrollmentStatus{RouterKey: "missing", Management: "missing"}, handler.HubConnectionState{Management: "missing", Setup: "not_started"}},
		{"pending installation", database.PendingEnrollmentStatus{RouterKey: "ready", Management: "ready"}, handler.HubConnectionState{Management: "missing", Setup: "pending"}},
		{"expired installation", database.PendingEnrollmentStatus{RouterKey: "expired", Management: "expired"}, handler.HubConnectionState{Management: "missing", Setup: "expired"}},
		{"repair awaiting verification", database.PendingEnrollmentStatus{RouterKey: "expired", Management: "ready"}, handler.HubConnectionState{Management: "missing", Setup: "repair_pending"}},
		{"incomplete preparation", database.PendingEnrollmentStatus{RouterKey: "ready", Management: "missing"}, handler.HubConnectionState{Management: "missing", Setup: "incomplete"}},
		{"expired management step", database.PendingEnrollmentStatus{RouterKey: "ready", Management: "expired"}, handler.HubConnectionState{Management: "missing", Setup: "incomplete"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hubConnectionState(tc.meta); got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}
