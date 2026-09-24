package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Database   DatabaseConfig   `mapstructure:"database"`
	Redis      RedisConfig      `mapstructure:"redis"`
	NATS       NATSConfig       `mapstructure:"nats"`
	WireGuard  WireGuardConfig  `mapstructure:"wireguard"`
	Telemetry  TelemetryConfig  `mapstructure:"telemetry"`
	Quotas     QuotasConfig     `mapstructure:"quotas"`
	Security   SecurityConfig   `mapstructure:"security"`
	Logging    LoggingConfig    `mapstructure:"logging"`
	Features   FeaturesConfig   `mapstructure:"features"`
	Monitoring MonitoringConfig `mapstructure:"monitoring"`
	Recovery   RecoveryConfig   `mapstructure:"recovery"`
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
	URL                string        `mapstructure:"url"`
	JetStream          bool          `mapstructure:"jetstream"`
	MaxReconnects      int           `mapstructure:"max_reconnects"`
	ReconnectWait      time.Duration `mapstructure:"reconnect_wait"`
	PingInterval       time.Duration `mapstructure:"ping_interval"`
	MaxOutstandingPings int          `mapstructure:"max_outstanding_pings"`
}

type WireGuardConfig struct {
	Interface        string        `mapstructure:"interface"`
	ListenPort       int           `mapstructure:"listen_port"`
	PeerKeepalive    time.Duration `mapstructure:"peer_keepalive"`
	AllowedIPs       []string      `mapstructure:"allowed_ips"`
	ManagementVLAN   int           `mapstructure:"management_vlan"`
	ManagementSubnet string        `mapstructure:"management_subnet"`
}

type TelemetryConfig struct {
	HardwareInterval   time.Duration `mapstructure:"hardware_interval"`
	HeartbeatInterval  time.Duration `mapstructure:"heartbeat_interval"`
	RetentionDays      int           `mapstructure:"retention_days"`
	MaxBatchSize       int           `mapstructure:"max_batch_size"`
}

type QuotasConfig struct {
	SmallWISPDeviceLimit      int           `mapstructure:"small_wisp_device_limit"`
	EnterpriseISPDeviceLimit  int           `mapstructure:"enterprise_isp_device_limit"`
	GlobalAdminDeviceLimit    int           `mapstructure:"global_admin_device_limit"`
	AITokensMonthlySmall      int64         `mapstructure:"ai_tokens_monthly_small"`
	AITokensMonthlyEnterprise int64          `mapstructure:"ai_tokens_monthly_enterprise"`
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
	MetricsPort          int           `mapstructure:"metrics_port"`
	HealthCheckInterval  time.Duration `mapstructure:"health_check_interval"`
	SLACheckInterval     time.Duration `mapstructure:"sla_check_interval"`
	StateSweepInterval   time.Duration `mapstructure:"state_sweep_interval"`
	AlertWebhookURL      string        `mapstructure:"alert_webhook_url"`
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
	v.SetDefault("database.name", "mega_magic_dev")
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", "300s")

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
}