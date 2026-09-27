package model

import "time"

// DeviceAllowanceKinds are the capacity dimensions an allowance and a request can
// refer to. They map 1:1 to the allowance columns, so a request can never name a
// dimension that does not exist.
var DeviceAllowanceKinds = []string{"routers", "controllers", "sites"}

// IsDeviceAllowanceKind reports whether k is a known capacity dimension.
func IsDeviceAllowanceKind(k string) bool {
	for _, v := range DeviceAllowanceKinds {
		if v == k {
			return true
		}
	}
	return false
}

// DeviceRequestStatuses are the terminal/initial states of a capacity request.
var DeviceRequestStatuses = []string{"pending", "approved", "rejected"}

// DeviceAllowance is a business's granted capacity, paired with what it is
// actually using. The `granted` figures come from Mikromagic's device_allowances;
// the usage figures are counted live.
//
// NOTE ON USAGE SEMANTICS: "routers" and "controllers" here are derived from the
// device deployment layer, and ACCESS_POINT traffic is reported separately rather
// than being folded into either — the ported data never defined which layers the
// grant covered, so inventing that mapping would misreport capacity.
type DeviceAllowance struct {
	BusinessID string `json:"business_id"`

	// Granted capacity.
	Routers     int `json:"routers"`
	Controllers int `json:"controllers"`
	Sites       int `json:"sites"`

	// Live usage.
	UsedSites          int            `json:"used_sites"`
	UsedControllers    int            `json:"used_controllers"`
	UsedRouters        int            `json:"used_routers"`
	UsedAccessPoints   int            `json:"used_access_points"`
	UsedDevicesByLayer map[string]int `json:"used_devices_by_layer"`

	// IncludedRouter is a router connection bundled into the grant, if any.
	IncludedRouterID   *string `json:"included_router_id,omitempty"`
	IncludedRouterName *string `json:"included_router_name,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DeviceRequest is a request for more capacity (or another site/controller),
// ported from Mikromagic. It is a workflow record: a user asks, an ADMIN+ decides.
type DeviceRequest struct {
	ID         string     `json:"id"`
	BusinessID string     `json:"business_id"`
	UserID     *string    `json:"user_id,omitempty"`
	UserEmail  *string    `json:"user_email,omitempty"`
	Kind       string     `json:"kind"`
	Reason     *string    `json:"reason,omitempty"`
	Status     string     `json:"status"`
	DecidedBy  *string    `json:"decided_by,omitempty"`
	DecidedAt  *time.Time `json:"decided_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// DeviceRequestCreate is the body for opening a request. Any authenticated role
// may ask; the decision is separately gated to ADMIN+.
type DeviceRequestCreate struct {
	Kind   string  `json:"kind"   binding:"required,min=1,max=32"`
	Reason *string `json:"reason" binding:"omitempty,max=500"`
}

// DeviceRequestDecision is the body for approving/rejecting. Only the terminal
// states are accepted — a request cannot be "decided" back to pending.
type DeviceRequestDecision struct {
	Status string  `json:"status" binding:"required,oneof=approved rejected"`
	Reason *string `json:"reason" binding:"omitempty,max=500"`
}

// Pending reports whether the request still awaits a decision.
func (r *DeviceRequest) Pending() bool { return r.Status == "pending" }
