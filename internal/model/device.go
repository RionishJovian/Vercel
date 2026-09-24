package model

import (
	"net"
	"time"
)

type Device struct {
	DeviceID        string    `json:"device_id"`
	BusinessID      string    `json:"business_id"`
	ParentGatewayID *string   `json:"parent_gateway_id,omitempty"`
	HardwareName    string    `json:"hardware_name"`
	DeviceModel     *string   `json:"device_model,omitempty"`
	DeploymentLayer string    `json:"deployment_layer"`
	Vendor          string    `json:"vendor"`
	Category        string    `json:"category"`
	Transport       string    `json:"transport"`
	MACAddress      *net.HardwareAddr `json:"mac_address,omitempty"`
	ManagementIP    *net.IP   `json:"management_ip,omitempty"`
	IsOnline        bool      `json:"is_online"`
	LastSeenAt      *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// PeerKey identifies one managed peer/router for a tenant — the unit the
// state-sweep worker iterates. Lightweight (two columns) vs the full Device.
type PeerKey struct {
	BusinessID string `json:"business_id"`
	DeviceID   string `json:"device_id"`
}

type Voucher struct {
	ID              string     `json:"id"`
	BusinessID      string     `json:"business_id"`
	DeviceID        *string    `json:"device_id,omitempty"`
	PlanKey         string     `json:"plan_key"`
	PlanLabel       string     `json:"plan_label"`
	Code            string     `json:"code"`
	PriceMMK        int        `json:"price_mmk"`
	DurationMinutes *int       `json:"duration_minutes,omitempty"`
	DeviceMAC       *net.HardwareAddr `json:"device_mac,omitempty"`
	Status          string     `json:"status"`
	FirstSeenAt     *time.Time `json:"first_seen_at,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type PortalSession struct {
	ID              string     `json:"id"`
	BusinessID      string     `json:"business_id"`
	DeviceID        *string    `json:"device_id,omitempty"`
	VoucherID       *string    `json:"voucher_id,omitempty"`
	ClientMAC       string     `json:"client_mac"`
	ClientIP        *net.IP    `json:"client_ip,omitempty"`
	AuthenticatedAt time.Time  `json:"authenticated_at"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	IsActive        bool       `json:"is_active"`
	ConsentGiven    bool       `json:"consent_given"`
	ConsentAt       *time.Time `json:"consent_at,omitempty"`
}

type TelemetrySample struct {
	ID             string     `json:"id"`
	BusinessID     string     `json:"business_id"`
	DeviceID       *string    `json:"device_id,omitempty"`
	SubjectKind    string     `json:"subject_kind"`
	SubjectID      string     `json:"subject_id"`
	CPUPercent     *float64   `json:"cpu_pct,omitempty"`
	MemoryPercent  *float64   `json:"memory_pct,omitempty"`
	LatencyMs      *int       `json:"latency_ms,omitempty"`
	UptimeSeconds  *int64     `json:"uptime_seconds,omitempty"`
	WANState       *string    `json:"wan_state,omitempty"`
	ConnectorState *string    `json:"connector_state,omitempty"`
	TunnelState    *string    `json:"tunnel_state,omitempty"`
	ObservedAt     time.Time  `json:"observed_at"`
}

type PeerHeartbeat struct {
	ID          string     `json:"id"`
	BusinessID  string     `json:"business_id"`
	PeerID      string     `json:"peer_id"`
	DeviceID    *string    `json:"device_id,omitempty"`
	HandshakeAt *time.Time `json:"handshake_at,omitempty"`
	State       string     `json:"state"`
	RXBytes     int64      `json:"rx_bytes"`
	TXBytes     int64      `json:"tx_bytes"`
	ObservedAt  time.Time  `json:"observed_at"`
}

type Incident struct {
	ID              string     `json:"id"`
	BusinessID      string     `json:"business_id"`
	DeviceID        *string    `json:"device_id,omitempty"`
	PolicyID        *string    `json:"policy_id,omitempty"`
	Kind            string     `json:"kind"`
	Severity        string     `json:"severity"`
	SubjectID       string     `json:"subject_id"`
	SubjectLabel    string     `json:"subject_label"`
	Detail          *string    `json:"detail,omitempty"`
	OpenedAt        time.Time  `json:"opened_at"`
	AcknowledgedAt  *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy  *string    `json:"acknowledged_by,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy      *string    `json:"resolved_by,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type AuditEvent struct {
	ID          string    `json:"id"`
	BusinessID  string    `json:"business_id"`
	UserID      *string   `json:"user_id,omitempty"`
	UserRole    string    `json:"user_role"`
	Action      string    `json:"action"`
	EntityType  string    `json:"entity_type"`
	EntityID    string    `json:"entity_id"`
	BeforeState *string   `json:"before_state,omitempty"`
	AfterState  *string   `json:"after_state,omitempty"`
	Reason      string    `json:"reason"`
	IPAddress   *string   `json:"ip_address,omitempty"`
	UserAgent   *string   `json:"user_agent,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

type AlertPolicy struct {
	ID                   string    `json:"id"`
	BusinessID           string    `json:"business_id"`
	Kind                 string    `json:"kind"`
	Enabled              bool      `json:"enabled"`
	Severity             string    `json:"severity"`
	Threshold            float64   `json:"threshold"`
	CooldownMinutes      int       `json:"cooldown_minutes"`
	NotificationChannels []string  `json:"notification_channels"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type NATSSubject struct {
	BusinessID    string    `json:"business_id"`
	SubjectPrefix string    `json:"subject_prefix"`
	CreatedAt     time.Time `json:"created_at"`
}

type BusinessMeteredQuota struct {
	BusinessID          string     `json:"business_id"`
	PurchasedExtraSlots int        `json:"purchased_extra_slots"`
	WebfigPassExpiresAt *time.Time `json:"webfig_pass_expires_at,omitempty"`
	AITokenBalance      int        `json:"ai_token_balance"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type PortalConfig struct {
	BusinessID      string `json:"business_id"`
	BusinessName    string `json:"business_name"`
	WelcomeText     string `json:"welcome_text"`
	TermsText       string `json:"terms_text"`
	PrimaryHex      string `json:"primary_hex"`
	GlassTintHex    string `json:"glass_tint_hex"`
	ConsentRequired bool   `json:"consent_required"`
	ConsentText     string `json:"consent_text"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type WebhookEndpoint struct {
	ID         string    `json:"id"`
	BusinessID string    `json:"business_id"`
	URL        string    `json:"url"`
	Secret     *string   `json:"secret,omitempty"`
	EventTypes []string  `json:"event_types"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type WebhookDelivery struct {
	ID             string    `json:"id"`
	WebhookID      *string   `json:"webhook_id,omitempty"`
	BusinessID     string    `json:"business_id"`
	EventType      string    `json:"event_type"`
	Payload        []byte    `json:"payload"`
	Status         string    `json:"status"`
	Attempts       int       `json:"attempts"`
	LastStatusCode *int      `json:"last_status_code,omitempty"`
	LastError      *string   `json:"last_error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
}