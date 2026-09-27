package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Database    DatabaseConfig    `mapstructure:"database"`
	Redis       RedisConfig       `mapstructure:"redis"`
	NATS        NATSConfig        `mapstructure:"nats"`
	WireGuard   WireGuardConfig   `mapstructure:"wireguard"`
	VPS         VPSConfig         `mapstructure:"vps"`
	Telemetry   TelemetryConfig   `mapstructure:"telemetry"`
	Quotas      QuotasConfig      `mapstructure:"quotas"`
	Security    SecurityConfig    `mapstructure:"security"`
	Logging     LoggingConfig     `mapstructure:"logging"`
	Features    FeaturesConfig    `mapstructure:"features"`
	Monitoring  MonitoringConfig  `mapstructure:"monitoring"`
	Recovery    RecoveryConfig    `mapstructure:"recovery"`
	Provisioner ProvisionerConfig `mapstructure:"provisioner"`
	Billing     BillingConfig     `mapstructure:"billing"`
	AI          AIConfig          `mapstructure:"ai"`
	GuestWifi   GuestWifiConfig   `mapstructure:"guest_wifi"`
}

// AIConfig configures the AI/scanning service. Disabled by default.
type AIConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	Mode            string `mapstructure:"mode"` // "auto" | "fallback" | "ollama" | "openrouter"
	OllamaURL       string `mapstructure:"ollama_url"`
	CostPerAnalysis int    `mapstructure:"cost_per_analysis"`
	// OpenRouter (used only when mode: openrouter). The API key is a secret:
	// never logged or echoed; supply via config/env, do not commit.
	OpenRouterURL    string `mapstructure:"openrouter_url"`
	OpenRouterAPIKey string `mapstructure:"openrouter_api_key"`
	OpenRouterModel  string `mapstructure:"openrouter_model"`
	// Operator incident notification (Telegram). Disabled by default; the bot
	// token is a secret — inject via config/env, never commit.
	NotifyTelegramEnabled  bool   `mapstructure:"notify_telegram_enabled"`
	NotifyTelegramBotToken string `mapstructure:"notify_telegram_bot_token"`
	NotifyTelegramChatID   int64  `mapstructure:"notify_telegram_chat_id"`
}

// GuestWifiConfig controls the Option C owner-bot loops. Enabled off by
// default; when off the supervisor stays still and owners decide on the web
// endpoint (the shared core either way). BridgeScanInterval sets how often the
// supervisor re-scans for newly-enabled/disabled owner bots.
type GuestWifiConfig struct {
	Enabled            bool          `mapstructure:"enabled"`
	BridgeScanInterval time.Duration `mapstructure:"bridge_scan_interval"`
	ReceiptDir         string        `mapstructure:"receipt_dir"`
}

// BillingConfig configures the billing module. ReceiptDir is the tenant-scoped
// root where guest payment receipts are stored (never world-readable). Plans is
// the plan -> duration source (plan.<planKey>.duration_minutes); a plan with no
// duration FAILS CLOSED at request time.
type BillingConfig struct {
	ReceiptDir               string                `mapstructure:"receipt_dir"`
	Plans                    map[string]PlanConfig `mapstructure:"plans"`
	QuotaEnforcementInterval time.Duration         `mapstructure:"quota_enforcement_interval"`
	// Telegram carries the operator purchase loop (outbound notify + inbound
	// approve/reject). Disabled by default; live only when Enabled + BotToken set.
	Telegram BillingTelegramConfig `mapstructure:"telegram"`
}

// BillingTelegramConfig configures the operator purchase-approval bot.
type BillingTelegramConfig struct {
	Enabled     bool          `mapstructure:"enabled"`
	BotToken    string        `mapstructure:"bot_token"`
	ChatID      int64         `mapstructure:"chat_id"`
	PollTimeout time.Duration `mapstructure:"poll_timeout"`
}

// PlanConfig holds per-plan billing metadata. DurationMinutes is REQUIRED for a
// plan to be purchasable (0/unset = fail closed).
type PlanConfig struct {
	DurationMinutes int `mapstructure:"duration_minutes"`
}

type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	GRPCPort        int           `mapstructure:"grpc_port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Name            string        `mapstructure:"name"`
	SSLMode         string        `mapstructure:"sslmode"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	// RLSEnforce, when true, activates the restricted tenant pool + the
	// tenant-pinning middleware so protected requests run on a connection bound
	// to `authenticated` + app.current_business_id. Default FALSE: behaviour is
	// byte-identical to an owner pool until an operator flips it.
	RLSEnforce bool `mapstructure:"rls_enforce"`
}

type RedisConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size"`
	MinIdleConns int           `mapstructure:"min_idle_conns"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

type NATSConfig struct {
	URL                 string        `mapstructure:"url"`
	JetStream           bool          `mapstructure:"jetstream"`
	MaxReconnects       int           `mapstructure:"max_reconnects"`
	ReconnectWait       time.Duration `mapstructure:"reconnect_wait"`
	PingInterval        time.Duration `mapstructure:"ping_interval"`
	MaxOutstandingPings int           `mapstructure:"max_outstanding_pings"`
}

type WireGuardConfig struct {
	Interface        string        `mapstructure:"interface"`
	ListenPort       int           `mapstructure:"listen_port"`
	PeerKeepalive    time.Duration `mapstructure:"peer_keepalive"`
	AllowedIPs       []string      `mapstructure:"allowed_ips"`
	ManagementVLAN   int           `mapstructure:"management_vlan"`
	ManagementSubnet string        `mapstructure:"management_subnet"`
}

// VPSConfig configures the read-only peer-provisioning surface. EndpointHost is
// the public address routers connect to for the tunnel; empty omits the
// endpoint from responses (operator hasn't pinned one yet).
type VPSConfig struct {
	EndpointHost string `mapstructure:"endpoint_host"`
}

type TelemetryConfig struct {
	HardwareInterval  time.Duration `mapstructure:"hardware_interval"`
	HeartbeatInterval time.Duration `mapstructure:"heartbeat_interval"`
	RetentionDays     int           `mapstructure:"retention_days"`
	MaxBatchSize      int           `mapstructure:"max_batch_size"`
}

type QuotasConfig struct {
	SmallWISPDeviceLimit      int           `mapstructure:"small_wisp_device_limit"`
	EnterpriseISPDeviceLimit  int           `mapstructure:"enterprise_isp_device_limit"`
	GlobalAdminDeviceLimit    int           `mapstructure:"global_admin_device_limit"`
	AITokensMonthlySmall      int64         `mapstructure:"ai_tokens_monthly_small"`
	AITokensMonthlyEnterprise int64         `mapstructure:"ai_tokens_monthly_enterprise"`
	WebFigPassDuration        time.Duration `mapstructure:"webfig_pass_duration"`
}

type SecurityConfig struct {
	JWTSecret     string        `mapstructure:"jwt_secret"`
	JWTExpiry     time.Duration `mapstructure:"jwt_expiry"`
	RefreshExpiry time.Duration `mapstructure:"refresh_expiry"`
	BcryptCost    int           `mapstructure:"bcrypt_cost"`
	AESGCMKey     string        `mapstructure:"aes_gcm_key"`
	TLSCertPath   string        `mapstructure:"tls_cert_path"`
	TLSKeyPath    string        `mapstructure:"tls_key_path"`
}

type LoggingConfig struct {
	Level    string `mapstructure:"level"`
	Format   string `mapstructure:"format"`
	Output   string `mapstructure:"output"`
	Sampling bool   `mapstructure:"sampling"`
}

type FeaturesConfig struct {
	AIScanEnabled         bool `mapstructure:"ai_scan_enabled"`
	VoucherSystemEnabled  bool `mapstructure:"voucher_system_enabled"`
	PortalConsentEnabled  bool `mapstructure:"portal_consent_enabled"`
	TopologyCanvasEnabled bool `mapstructure:"topology_canvas_enabled"`
	MagicSwitchEnabled    bool `mapstructure:"magic_switch_enabled"`
	WebFigProxyEnabled    bool `mapstructure:"webfig_proxy_enabled"`
}

type MonitoringConfig struct {
	MetricsPort         int           `mapstructure:"metrics_port"`
	HealthCheckInterval time.Duration `mapstructure:"health_check_interval"`
	SLACheckInterval    time.Duration `mapstructure:"sla_check_interval"`
	StateSweepInterval  time.Duration `mapstructure:"state_sweep_interval"`
	AlertWebhookURL     string        `mapstructure:"alert_webhook_url"`
}

// RecoveryConfig tunes the peer self-healing loop: how stale a handshake must be
// before a peer is a recovery candidate (TDown), when repeated failures escalate
// to an incident (TEscalate), and how many attempts run inside a window before
// backoff suppresses further auto-recovery (MaxAttempts / BackoffWindow).
type RecoveryConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	TDown         time.Duration `mapstructure:"t_down"`
	TEscalate     time.Duration `mapstructure:"t_escalate"`
	MaxAttempts   int           `mapstructure:"max_attempts"`
	BackoffWindow time.Duration `mapstructure:"backoff_window"`
}

// ProvisionerConfig configures the real provisioner apply path.
type ProvisionerConfig struct {
	Applier ApplierConfig `mapstructure:"applier"`
}

// ApplierConfig configures the REAL SSH + RouterOS Applier/Prober. It is
// DISABLED by default; live router writes require Enabled=true. The target must
// never be a production host — the transport fails closed on prod-shaped
// addresses regardless of this flag.
type ApplierConfig struct {
	Enabled               bool          `mapstructure:"enabled"`
	Host                  string        `mapstructure:"host"`
	Port                  int           `mapstructure:"port"`
	User                  string        `mapstructure:"user"`
	Password              string        `mapstructure:"password"`
	PrivateKeyPath        string        `mapstructure:"private_key_path"`
	InsecureIgnoreHostKey bool          `mapstructure:"insecure_ignore_host_key"`
	AllowEmptyPassword    bool          `mapstructure:"allow_empty_password"`
	Timeout               time.Duration `mapstructure:"timeout"`
	Interface             string        `mapstructure:"interface"`
	ListenPortRotateTo    int           `mapstructure:"listen_port_rotate_to"`
	ResetHold             time.Duration `mapstructure:"reset_hold"`
	PingTarget            string        `mapstructure:"ping_target"`
	TDown                 time.Duration `mapstructure:"t_down"`

	// CredentialKey is the base64/hex 32-byte AES-256-GCM key used to encrypt
	// per-device SSH secrets at rest (internal/secretbox). EMPTY => the fleet
	// posture path cannot resolve any device and captures nothing. It is
	// deliberately not defaulted: a missing key must look unconfigured, never
	// like encryption is on.
	CredentialKey string `mapstructure:"credential_key"`

	// RFC 6238 TOTP gate for HIGH-RISK actions. Empty secret => high-risk stays
	// DENIED (fail closed). The secret is injected via config, never hardcoded.
	OtpSecret string        `mapstructure:"otp_secret"`
	OtpDigits int           `mapstructure:"otp_digits"`
	OtpStep   time.Duration `mapstructure:"otp_step"`
	OtpSkew   int           `mapstructure:"otp_skew"`

	// Telegram approve-bridge target. The real bot transport is an injected seam
	// (not implemented yet); with no messenger the bridge stays inert and
	// approvals time out to DENY.
	TelegramChatID string `mapstructure:"telegram_chat_id"`

	// TelegramBotToken gates the real Bot-API transport for the approve bridge
	// and billing notify. Empty => no live bot (fail closed, inert).
	TelegramBotToken string `mapstructure:"telegram_bot_token"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.grpc_port", 9090)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.shutdown_timeout", "10s")

	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "mega_admin")
	v.SetDefault("database.password", "MegaSecurePass123")
	v.SetDefault("billing.receipt_dir", "./data/receipts")
	v.SetDefault("billing.quota_enforcement_interval", "60s")
	v.SetDefault("billing.telegram.poll_timeout", "10s")
	v.SetDefault("ai.enabled", false)
	v.SetDefault("ai.mode", "auto")
	v.SetDefault("ai.ollama_url", "http://127.0.0.1:11434")
	v.SetDefault("ai.cost_per_analysis", 1)
	v.SetDefault("ai.openrouter_url", "https://openrouter.ai/api/v1")
	v.SetDefault("ai.openrouter_model", "nvidia/nemotron-3-super-120b-a12b:free")
	v.SetDefault("ai.notify_telegram_enabled", false)
	// Production database: the merged data. `mega_magic_dev` is the pristine
	// baseline and `mega_merge` the ETL working copy — neither should be the
	// default, or a fresh deploy would serve the empty baseline and silently
	// lose every migrated row. Build the promoted database with
	// scripts/migration/promote.sh.
	v.SetDefault("database.name", "mega_magic")
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", "300s")
	v.SetDefault("database.rls_enforce", false)

	v.SetDefault("redis.host", "localhost")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.pool_size", 10)
	v.SetDefault("redis.min_idle_conns", 2)
	v.SetDefault("redis.dial_timeout", "5s")
	v.SetDefault("redis.read_timeout", "3s")
	v.SetDefault("redis.write_timeout", "3s")

	v.SetDefault("nats.url", "nats://localhost:4222")
	v.SetDefault("nats.jetstream", true)
	v.SetDefault("nats.max_reconnects", -1)
	v.SetDefault("nats.reconnect_wait", "2s")
	v.SetDefault("nats.ping_interval", "20s")
	v.SetDefault("nats.max_outstanding_pings", 3)

	v.SetDefault("wireguard.interface", "wg-magichub")
	v.SetDefault("wireguard.listen_port", 13231)
	v.SetDefault("wireguard.peer_keepalive", "25s")
	v.SetDefault("wireguard.allowed_ips", []string{"10.255.0.1/32", "192.168.0.0/16"})
	v.SetDefault("wireguard.management_vlan", 99)
	v.SetDefault("wireguard.management_subnet", "192.168.99.0/24")

	v.SetDefault("telemetry.hardware_interval", "60s")
	v.SetDefault("telemetry.heartbeat_interval", "25s")
	v.SetDefault("telemetry.retention_days", 30)
	v.SetDefault("telemetry.max_batch_size", 100)

	v.SetDefault("quotas.small_wisp_device_limit", 10)
	v.SetDefault("quotas.enterprise_isp_device_limit", 100)
	v.SetDefault("quotas.global_admin_device_limit", 99999)
	v.SetDefault("quotas.ai_tokens_monthly_small", 50000)
	v.SetDefault("quotas.ai_tokens_monthly_enterprise", 2000000)
	v.SetDefault("quotas.webfig_pass_duration", "3h")

	v.SetDefault("security.jwt_secret", "CHANGE_ME_IN_PRODUCTION_USE_STRONG_RANDOM_32_BYTES")
	v.SetDefault("security.jwt_expiry", "24h")
	v.SetDefault("security.refresh_expiry", "168h")
	v.SetDefault("security.bcrypt_cost", 12)
	v.SetDefault("security.aes_gcm_key", "CHANGE_ME_32_BYTE_KEY_FOR_ENCRYPTION")
	v.SetDefault("security.tls_cert_path", "")
	v.SetDefault("security.tls_key_path", "")

	v.SetDefault("logging.level", "debug")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")
	v.SetDefault("logging.sampling", true)

	v.SetDefault("features.ai_scan_enabled", true)
	v.SetDefault("features.voucher_system_enabled", true)
	v.SetDefault("features.portal_consent_enabled", true)
	v.SetDefault("features.topology_canvas_enabled", true)
	v.SetDefault("features.magic_switch_enabled", true)
	v.SetDefault("features.webfig_proxy_enabled", true)

	v.SetDefault("monitoring.metrics_port", 9091)
	v.SetDefault("monitoring.health_check_interval", "30s")
	v.SetDefault("monitoring.sla_check_interval", "60s")
	v.SetDefault("monitoring.state_sweep_interval", "60s")
	v.SetDefault("monitoring.alert_webhook_url", "")

	v.SetDefault("recovery.enabled", true)
	v.SetDefault("recovery.t_down", "180s")
	v.SetDefault("recovery.t_escalate", "10m")
	v.SetDefault("recovery.max_attempts", 3)
	v.SetDefault("recovery.backoff_window", "15m")

	// Live provisioner applier: OFF by default. Never point at production.
	v.SetDefault("provisioner.applier.enabled", false)
	v.SetDefault("provisioner.applier.port", 22)
	v.SetDefault("provisioner.applier.insecure_ignore_host_key", true)
	v.SetDefault("provisioner.applier.allow_empty_password", false)
	v.SetDefault("provisioner.applier.timeout", "10s")
	v.SetDefault("provisioner.applier.interface", "wg-magichub")
	v.SetDefault("provisioner.applier.reset_hold", "30s")
	v.SetDefault("provisioner.applier.t_down", "180s")
	v.SetDefault("provisioner.applier.otp_secret", "")
	v.SetDefault("provisioner.applier.otp_digits", 6)
	v.SetDefault("provisioner.applier.otp_step", "30s")
	v.SetDefault("provisioner.applier.otp_skew", 1)
	v.SetDefault("provisioner.applier.telegram_chat_id", "")
}
