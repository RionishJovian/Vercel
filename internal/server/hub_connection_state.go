package server

import (
	"mega-magic/internal/database"
	"mega-magic/internal/handler"
)

func hubConnectionState(metadata database.PendingEnrollmentStatus) handler.HubConnectionState {
	if metadata.ManagementSaved {
		return handler.HubConnectionState{Management: "saved", Setup: "not_required"}
	}
	state := handler.HubConnectionState{Management: "missing", Setup: "not_started"}
	switch {
	case metadata.RouterKey == "ready" && metadata.Management == "ready":
		state.Setup = "pending"
	case metadata.Management == "ready":
		state.Setup = "repair_pending"
	case metadata.RouterKey == "ready":
		state.Setup = "incomplete"
	case metadata.RouterKey == "expired" || metadata.Management == "expired":
		state.Setup = "expired"
	}
	return state
}
