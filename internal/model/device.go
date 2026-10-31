package model

import (
	"net"
	"time"
)

type Device struct {
	DeviceID        string            `json:"device_id"`
	BusinessID      string            `json:"business_id"`
	ParentGatewayID *string           `json:"parent_gateway_id,omitempty"`
	SiteID          *string           `json:"site_id,omitempty"`
	HardwareName    string            `json:"hardware_name"`
	DeviceModel     *string           `json:"device_model,omitempty"`
	DeploymentLayer string            `json:"deployment_layer"`
	Vendor          string            `json:"vendor"`
	Category        string            `json:"category"`
	Transport       string            `json:"transport"`
	MACAddress      *net.HardwareAddr `json:"mac_address,omitempty"`
	ManagementIP    *net.IP           `json:"management_ip,omitempty"`
	IsOnline        bool              `json:"is_online"`
	LastSeenAt      *time.Time        `json:"last_seen_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// PeerKey identifies one managed peer/router for a tenant — the unit the
// state-sweep worker iterates. Lightweight (two columns) vs the full Device.
type PeerKey struct {
	BusinessID string `json:"business_id"`
	DeviceID   string `json:"device_id"`
}

type Voucher struct {
	ResellerID      string            `json:"reseller_id,omitempty"`
	ResellerShop    string            `json:"reseller_shop,omitempty"`
	ResellerPrefix  string            `json:"reseller_prefix,omitempty"`
	IssueBatchID    string            `json:"issue_batch_id,omitempty"`
	InstallState    string            `json:"install_state,omitempty"`
	ID              string            `json:"id"`
	BusinessID      string            `json:"business_id"`
	DeviceID        *string           `json:"device_id,omitempty"`
	PlanKey         string            `json:"plan_key"`
	PlanLabel       string            `json:"plan_label"`
	Code            string            `json:"code"`
	PriceMMK        int               `json:"price_mmk"`
	DurationMinutes *int              `json:"duration_minutes,omitempty"`
	DeviceMAC       *net.HardwareAddr `json:"device_mac,omitempty"`
	Status          string            `json:"status"`
	FirstSeenAt     *time.Time        `json:"first_seen_at,omitempty"`
	ExpiresAt       *time.Time        `json:"expires_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
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
	ID             string    `json:"id"`
	BusinessID     string    `json:"business_id"`
	DeviceID       *string   `json:"device_id,omitempty"`
	SubjectKind    string    `json:"subject_kind"`
	SubjectID      string    `json:"subject_id"`
	CPUPercent     *float64  `json:"cpu_pct,omitempty"`
	MemoryPercent  *float64  `json:"memory_pct,omitempty"`
	LatencyMs      *int      `json:"latency_ms,omitempty"`
	UptimeSeconds  *int64    `json:"uptime_seconds,omitempty"`
	WANState       *string   `json:"wan_state,omitempty"`
	ConnectorState *string   `json:"connector_state,omitempty"`
	TunnelState    *string   `json:"tunnel_state,omitempty"`
	ObservedAt     time.Time `json:"observed_at"`
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

// SuggestedAction is the optional operator-remediation hint carried on an AI
// finding / incident: what to do about it, in one tap.
type SuggestedAction struct {
	Kind       string `json:"kind"`                  // reboot|restore_backup|reprovision_peer|review|none
	TargetHint string `json:"target_hint,omitempty"` // e.g. a device id or subject
	OneLiner   string `json:"one_liner"`             // short human instruction
}

type Incident struct {
	ID              string           `json:"id"`
	BusinessID      string           `json:"business_id"`
	DeviceID        *string          `json:"device_id,omitempty"`
	PolicyID        *string          `json:"policy_id,omitempty"`
	Kind            string           `json:"kind"`
	Severity        string           `json:"severity"`
	SubjectID       string           `json:"subject_id"`
	SubjectLabel    string           `json:"subject_label"`
	Detail          *string          `json:"detail,omitempty"`
	SuggestedAction *SuggestedAction `json:"suggested_action,omitempty"`
	OpenedAt        time.Time        `json:"opened_at"`
	AcknowledgedAt  *time.Time       `json:"acknowledged_at,omitempty"`
	AcknowledgedBy  *string          `json:"acknowledged_by,omitempty"`
	ResolvedAt      *time.Time       `json:"resolved_at,omitempty"`
	ResolvedBy      *string          `json:"resolved_by,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
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
	BankName        string `json:"bank_name,omitempty"`
	BankAccount     string `json:"bank_account,omitempty"`
	BankHolder      string `json:"bank_holder,omitempty"`
	// MMQRURL is the owner's MMQR payment QR image (Myanmar QR) guests scan to
	// make the bank transfer. Served path or https URL.
	MMQRURL string `json:"mmqr_url,omitempty"`
	// PaymentInstructions is the owner's own free-text bank-transfer steps. The
	// platform never takes payment; guests pay the owner's account directly.
	PaymentInstructions string `json:"payment_instructions,omitempty"`
	// MediaHost is the owner's LAN media server, stored once per business so a
	// portal redeploy does not require retyping it. Only a private IPv4 address
	// on the business network is accepted.
	MediaHost string `json:"media_host,omitempty"`
	// MediaPort is the port guests use to reach MediaHost. Defaults to 8096.
	MediaPort string `json:"media_port,omitempty"`
	// Guest wifi (Option C): free online-payment window. RouterOS caps the
	// temporary guest by both time and data; reaching either limit ends access.
	GuestGraceMinutes int         `json:"guest_grace_minutes"`
	GuestGraceDataMB  int         `json:"guest_grace_data_mb"`
	GuestPlans        []GuestPlan `json:"guest_plans"`
	// Guest entry methods are independently selectable. At least one must stay
	// enabled; when both are enabled the portal opens on voucher login and links
	// to mobile payment as a separate screen.
	VoucherLoginEnabled  bool      `json:"voucher_login_enabled"`
	MobilePaymentEnabled bool      `json:"mobile_payment_enabled"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// IsHexColor reports whether s is a literal CSS hex colour — `#rgb`, `#rgba`,
// `#rrggbb` or `#rrggbbaa`.
//
// Owner-supplied brand colours are interpolated straight into the GUEST page's
// stylesheet, so this refuses anything that is not a plain hex literal rather
// than trying to escape it: a colour value is never a legitimate place for
// arbitrary CSS. The guest portal sanitises independently, because a public page
// must not assume the write path was the only way a value got there.
func IsHexColor(s string) bool {
	switch len(s) {
	case 4, 5, 7, 9: // #rgb, #rgba, #rrggbb, #rrggbbaa
	default:
		return false
	}
	if s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		ok := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !ok {
			return false
		}
	}
	return true
}

// GuestPlan is one wifi data plan the owner sets on the portal for guests to
// buy via online banking. Approved requests grant `minutes` of data use and,
// when DataMB > 0, a data allowance. DataMB is the plan's data limit in
// megabytes (0 = time-only plan). Editable per hotspot owner (Myanmar market:
// pricing and data vary by location, so this is owner-set, never global).
type GuestPlan struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Minutes int    `json:"minutes"`
	Amount  int    `json:"amount"`
	DataMB  int    `json:"data_mb,omitempty"`
}

// BusinessTelegramBot is the Telegram bot that carries a business's GUEST wifi
// loop (notify + approve/reject). The owner binds it under their platform
// account; the platform never uses their token for anything else.
type BusinessTelegramBot struct {
	BusinessID     string    `json:"business_id"`
	BotToken       string    `json:"bot_token,omitempty"` // masked on read-back
	TargetChatID   int64     `json:"target_chat_id"`
	ApproverUserID int64     `json:"approver_user_id"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// GuestPurchaseRequest is an owner-approved online-banking wifi submit from a
// guest on the captive portal. pending -> approved|rejected, exactly once.
type GuestPurchaseRequest struct {
	PurchasedMinutes *int    `json:"purchased_minutes,omitempty"`
	ID               string  `json:"id"`
	BusinessID       string  `json:"business_id"`
	DeviceID         *string `json:"device_id,omitempty"`
	ClientMAC        string  `json:"client_mac"`
	ClientIP         *net.IP `json:"client_ip,omitempty"`
	PlanKey          string  `json:"plan_key"`
	Amount           int     `json:"amount"`
	DataMB           int     `json:"data_mb,omitempty"`
	// MMQR flow: the payer's phone number and last 6 digits of the bank/MMQR
	// transaction number, submitted after paying so the owner can verify.
	PaidPhone      string     `json:"paid_phone,omitempty"`
	TxLast6        string     `json:"tx_last6,omitempty"`
	BankReference  *string    `json:"bank_reference,omitempty"`
	ReceiptPath    *string    `json:"receipt_path,omitempty"`
	GraceStartedAt *time.Time `json:"grace_started_at,omitempty"`
	GraceExpiresAt *time.Time `json:"grace_expires_at,omitempty"`
	GraceDataMB    int        `json:"grace_data_mb"`
	Status         string     `json:"status"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
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
	ID             string     `json:"id"`
	WebhookID      *string    `json:"webhook_id,omitempty"`
	BusinessID     string     `json:"business_id"`
	EventType      string     `json:"event_type"`
	Payload        []byte     `json:"payload"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	LastStatusCode *int       `json:"last_status_code,omitempty"`
	LastError      *string    `json:"last_error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
}
