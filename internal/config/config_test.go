package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validCredentialKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

func TestLoadGeminiEnvironmentAndMissingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("environment: development\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("AI_MAGIC_DUDE_PROVIDER", "gemini")
	t.Setenv("AI_GEMINI_MODEL", "gemini-3.8-flash")
	t.Setenv("AI_GEMINI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "test-alias-key")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AI.MagicDudeProvider != "gemini" || cfg.AI.GeminiAPIKey != "test-alias-key" {
		t.Fatal("Gemini environment settings not loaded")
	}
	t.Setenv("GEMINI_API_KEY", "")
	if _, err = Load(path); err == nil || !strings.Contains(err.Error(), "ai.gemini_api_key") {
		t.Fatalf("missing key should fail validation: %v", err)
	}
}

// TestLoad_provisionerApplierDefaultsOff proves the live applier is DISABLED by
// default (deliverable 4) even when the config file says nothing about it.
func TestLoad_provisionerApplierDefaultsOff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("environment: development\n# no provisioner section on purpose\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ap := cfg.Provisioner.Applier
	if ap.Enabled {
		t.Fatal("provisioner.applier.enabled must default to false")
	}
	if ap.Port != 22 {
		t.Fatalf("applier port default = %d, want 22", ap.Port)
	}
	if ap.InsecureIgnoreHostKey {
		t.Fatal("insecure_ignore_host_key must default to false so SSH host keys are verified")
	}
	if ap.AllowEmptyPassword {
		t.Fatal("allow_empty_password must default to false")
	}
	if ap.Interface != "wg-magichub" {
		t.Fatalf("interface default = %q, want wg-magichub", ap.Interface)
	}
	if ap.Host != "" {
		t.Fatalf("host must default empty, got %q", ap.Host)
	}
}

// TestLoad_vpsConfigParses proves the committed example config's vps section loads.
func TestLoad_vpsConfigParses(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")
	cfg, err := Load("../../config.yaml.example")
	if err != nil {
		t.Fatalf("Load config.yaml: %v", err)
	}
	if cfg.VPS.EndpointHost != "" {
		t.Fatalf("vps.endpoint_host should default empty, got %q", cfg.VPS.EndpointHost)
	}
	if cfg.WireGuard.Interface != "wg0" || cfg.WireGuard.ListenPort != 13231 {
		t.Fatalf("wireguard config not loaded: %+v", cfg.WireGuard)
	}
}

// TestLoad_billingTelegramDefaultsOff proves the operator-purchase bot is
// DISABLED by default (fail closed): no token, no chat, so the bridge stays
// inert until explicitly enabled.
func TestLoad_billingTelegramDefaultsOff(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")
	cfg, err := Load("../../config.yaml.example")
	if err != nil {
		t.Fatalf("Load config.yaml: %v", err)
	}
	tg := cfg.Billing.Telegram
	if tg.Enabled {
		t.Fatal("billing.telegram.enabled must default to false")
	}
	if tg.BotToken != "" {
		t.Fatal("billing.telegram.bot_token must default empty")
	}
	if tg.ChatID != 0 {
		t.Fatalf("billing.telegram.chat_id must default 0, got %d", tg.ChatID)
	}
}

func TestLoad_openRouterKeyCompatibilityAlias(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("environment: development\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	t.Setenv("AI_OPENROUTER_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "alias-test-key")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AI.OpenRouterAPIKey != "alias-test-key" {
		t.Fatalf("compatibility key was not loaded")
	}
}

func TestValidateDevelopmentAllowsLocalDefaults(t *testing.T) {
	cfg := &Config{Environment: "development"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("development config should allow local defaults: %v", err)
	}
}

func TestValidateProductionRejectsUnsafeDefaults(t *testing.T) {
	cfg := &Config{
		Environment: "production",
		Database: DatabaseConfig{
			Password:   defaultDatabasePassword,
			RLSEnforce: false,
		},
		Security: SecurityConfig{
			JWTSecret: defaultJWTSecret,
			AESGCMKey: defaultAESGCMKey,
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("unsafe production defaults must be rejected")
	}
	for _, want := range []string{"database.rls_enforce", "database.password", "security.jwt_secret", "security.aes_gcm_key"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("validation error %q does not mention %s", err, want)
		}
	}
}

func TestValidateProductionAcceptsHardenedConfig(t *testing.T) {
	cfg := &Config{
		Environment: "prod",
		Database: DatabaseConfig{
			Password:   "unique-database-password-2026",
			RLSEnforce: true,
		},
		Security: SecurityConfig{
			JWTSecret: "unique-jwt-secret-at-least-32-bytes-long",
			AESGCMKey: "unique-aes-key-at-least-32-bytes-long",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("hardened production config rejected: %v", err)
	}
}

func TestValidateProductionRejectsIgnoredSSHHostKey(t *testing.T) {
	cfg := &Config{
		Environment: "prod",
		Database:    DatabaseConfig{Password: "unique-database-password-2026", RLSEnforce: true},
		Security: SecurityConfig{
			JWTSecret: "unique-jwt-secret-at-least-32-bytes-long",
			AESGCMKey: "unique-aes-key-at-least-32-bytes-long",
		},
	}
	cfg.Provisioner.Applier.InsecureIgnoreHostKey = true
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "insecure_ignore_host_key") {
		t.Fatalf("production must refuse an ignored SSH host key, got %v", err)
	}
}

func TestValidateRejectsPaidOpenRouterModel(t *testing.T) {
	cfg := &Config{Environment: "development", AI: AIConfig{OpenRouterModel: "provider/paid-model"}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "ai.openrouter_model") {
		t.Fatalf("paid OpenRouter model should be rejected, got %v", err)
	}
	cfg.AI.OpenRouterModel = "provider/free-model:free"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("free OpenRouter model rejected: %v", err)
	}
}

func TestValidateProductionRouterAccessRequiresTargetAllowlist(t *testing.T) {
	cfg := &Config{
		Environment: "production",
		Database: DatabaseConfig{
			Password:   "unique-database-password-2026",
			RLSEnforce: true,
		},
		Security: SecurityConfig{
			JWTSecret: "unique-jwt-secret-at-least-32-bytes-long",
			AESGCMKey: "unique-aes-key-at-least-32-bytes-long",
		},
		Provisioner: ProvisionerConfig{Applier: ApplierConfig{
			Enabled:       true,
			PingTarget:    "10.77.0.1",
			CredentialKey: validCredentialKey,
		}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "provisioner.allowed_target_cidrs") {
		t.Fatalf("enabled production router access without allowlist must fail, got %v", err)
	}
	cfg.Provisioner.AllowedTargetCIDRs = []string{"10.77.0.0/24"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("explicit production target allowlist rejected: %v", err)
	}
}

func TestValidateRouterAccessRejectsUnsafeInputsInEveryEnvironment(t *testing.T) {
	tests := []struct {
		name string
		cfg  ProvisionerConfig
		want string
	}{
		{
			name: "missing ping target",
			cfg: ProvisionerConfig{
				AllowedTargetCIDRs: []string{"10.77.0.0/24"},
				Applier:            ApplierConfig{Enabled: true, CredentialKey: validCredentialKey},
			},
			want: "ping_target",
		},
		{
			name: "ping target is not an IP",
			cfg: ProvisionerConfig{
				AllowedTargetCIDRs: []string{"10.77.0.0/24"},
				Applier:            ApplierConfig{Enabled: true, CredentialKey: validCredentialKey, PingTarget: "hub.example.com"},
			},
			want: "ping_target",
		},
		{
			name: "invalid credential key",
			cfg: ProvisionerConfig{
				AllowedTargetCIDRs: []string{"10.77.0.0/24"},
				Applier:            ApplierConfig{Enabled: true, CredentialKey: "too-short", PingTarget: "10.77.0.1"},
			},
			want: "credential_key",
		},
		{
			name: "malformed CIDR",
			cfg: ProvisionerConfig{
				AllowedTargetCIDRs: []string{"not-a-cidr"},
				PostureCapture:     PostureCaptureConfig{Enabled: true, CredentialKey: validCredentialKey},
			},
			want: "invalid CIDR",
		},
		{
			name: "default route",
			cfg: ProvisionerConfig{
				AllowedTargetCIDRs: []string{"0.0.0.0/0"},
				PostureCapture:     PostureCaptureConfig{Enabled: true, CredentialKey: validCredentialKey},
			},
			want: "default route",
		},
		{
			name: "noncanonical CIDR",
			cfg: ProvisionerConfig{
				AllowedTargetCIDRs: []string{"10.77.0.4/24"},
				PostureCapture:     PostureCaptureConfig{Enabled: true, CredentialKey: validCredentialKey},
			},
			want: "canonical",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := (&Config{Environment: "development", Provisioner: tc.cfg}).Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want mention of %q", err, tc.want)
			}
		})
	}
}

func TestValidateRouterAccessAcceptsCompleteConfiguration(t *testing.T) {
	cfg := &Config{
		Environment: "development",
		Provisioner: ProvisionerConfig{
			AllowedTargetCIDRs: []string{"10.77.0.0/24", "fd00:77::/64"},
			Applier:            ApplierConfig{Enabled: true, CredentialKey: validCredentialKey, PingTarget: "10.77.0.1"},
			PostureCapture:     PostureCaptureConfig{Enabled: true}, // valid applier-key fallback
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("complete router access configuration rejected: %v", err)
	}
}

func TestLoadEnvironmentOverrideActivatesProductionValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("# environment supplied below; other defaults are intentionally unsafe\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	t.Setenv("ENVIRONMENT", "production")

	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unsafe production configuration") {
		t.Fatalf("production environment override must activate validation, got %v", err)
	}
}

func TestLoadCredentialKeysFromEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("environment: development\n"), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	t.Setenv("PROVISIONER_APPLIER_CREDENTIAL_KEY", "applier-key-from-environment")
	t.Setenv("PROVISIONER_POSTURE_CAPTURE_CREDENTIAL_KEY", "posture-key-from-environment")
	t.Setenv("PROVISIONER_ALLOWED_TARGET_CIDRS", "10.77.0.0/24,10.88.0.0/24")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Provisioner.Applier.CredentialKey != "applier-key-from-environment" {
		t.Fatal("applier credential key environment override was not loaded")
	}
	if cfg.Provisioner.PostureCapture.CredentialKey != "posture-key-from-environment" {
		t.Fatal("posture credential key environment override was not loaded")
	}
	if got := cfg.Provisioner.AllowedTargetCIDRs; len(got) != 2 || got[0] != "10.77.0.0/24" || got[1] != "10.88.0.0/24" {
		t.Fatalf("allowed target CIDR environment override = %v, want two CIDRs", got)
	}
}

func TestLoadRejectsMissingOrUnknownEnvironment(t *testing.T) {
	dir := t.TempDir()
	for name, contents := range map[string]string{
		"missing": "{}\n",
		"unknown": "environment: prodution\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".yaml")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("unsafe environment value was accepted")
			}
		})
	}
}
