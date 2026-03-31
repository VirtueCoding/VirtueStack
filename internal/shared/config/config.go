// Package config provides configuration loading for VirtueStack components.
// It supports loading from environment variables (primary) and YAML config files.
// Environment variables take precedence over YAML values.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Secret is a string type whose String() and MarshalJSON() methods always return
// "[REDACTED]" to prevent accidental logging or serialisation of sensitive values.
// Use Value() to retrieve the underlying secret string.
type Secret string

// String implements fmt.Stringer and returns a redacted placeholder.
func (s Secret) String() string { return "[REDACTED]" }

// MarshalJSON returns a JSON-encoded "[REDACTED]" string so the secret is not
// exposed when a config struct is marshalled to JSON (e.g. for debug endpoints).
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }

// Value returns the underlying secret string for use in cryptographic operations.
func (s Secret) Value() string { return string(s) }

// Weak passwords that should be rejected.
var weakPasswords = map[string]bool{
	"changeme": true,
	"password": true,
	"123456":   true,
	"admin":    true,
	"root":     true,
}

// Default configuration values.
const (
	defaultListenAddr     = ":8080"
	defaultLogLevel       = "info"
	defaultConsoleBaseURL = "https://console.virtuestack.io"
	defaultCephPool       = "vs-vms"
	defaultCephUser       = "virtuestack"
	defaultCephConf       = "/etc/ceph/ceph.conf"
	defaultCloudInitPath  = "/var/lib/virtuestack/cloud-init"
	defaultISOStoragePath = "/var/lib/virtuestack/iso"
	defaultDNSNameservers = "8.8.8.8,8.8.4.4"
	defaultStorageBackend = "ceph"
	defaultStoragePath    = "/var/lib/virtuestack"

	// LVM thin pool monitoring thresholds
	defaultLVMDataPercentThreshold     = 95
	defaultLVMMetadataPercentThreshold = 70

	// Directory structure constants for file-based storage
	DefaultVmsDir       = "vms"
	DefaultTemplatesDir = "templates"
	DefaultBackupsDir   = "backups"
)

// NATSConfig holds NATS connection configuration.
type NATSConfig struct {
	URL       string `yaml:"url" env:"NATS_URL"`
	AuthToken string `yaml:"auth_token" env:"NATS_AUTH_TOKEN"`
}

// SMTPConfig holds SMTP email configuration.
type SMTPConfig struct {
	Host       string `yaml:"host" env:"SMTP_HOST"`
	Port       int    `yaml:"port" env:"SMTP_PORT"`
	User       string `yaml:"user" env:"SMTP_USER"`
	Password   string `yaml:"password" env:"SMTP_PASSWORD"`
	From       string `yaml:"from" env:"SMTP_FROM"`
	Enabled    bool   `yaml:"enabled" env:"SMTP_ENABLED"`
	RequireTLS bool   `yaml:"require_tls" env:"SMTP_REQUIRE_TLS"` // When true, enforce STARTTLS for non-465 ports
}

// TelegramConfig holds Telegram notification configuration.
type TelegramConfig struct {
	BotToken string `yaml:"bot_token" env:"TELEGRAM_BOT_TOKEN"`
	ChatID   string `yaml:"chat_id" env:"TELEGRAM_CHAT_ID"`
}

// BackupConfig holds backup configuration.
type BackupConfig struct {
	Enabled     bool   `yaml:"enabled" env:"BACKUP_ENABLED"`
	Schedule    string `yaml:"schedule" env:"BACKUP_SCHEDULE"`
	Retention   int    `yaml:"retention" env:"BACKUP_RETENTION"`
	StoragePath string `yaml:"storage_path" env:"BACKUP_STORAGE_PATH"`
	RemoteHost  string `yaml:"remote_host" env:"BACKUP_REMOTE_HOST"`
	RemoteUser  string `yaml:"remote_user" env:"BACKUP_REMOTE_USER"`
	RemotePath  string `yaml:"remote_path" env:"BACKUP_REMOTE_PATH"`
}

// FileStorageConfig holds file-based storage configuration.
type FileStorageConfig struct {
	TemplateImportPaths []string `yaml:"template_import_paths" env:"TEMPLATE_IMPORT_PATHS"`
	BackupRetentionDays int      `yaml:"backup_retention_days" env:"BACKUP_RETENTION_DAYS"`
	MaxTemplateSizeGB   int      `yaml:"max_template_size_gb" env:"MAX_TEMPLATE_SIZE_GB"`
	ISOStoragePath      string   `yaml:"iso_storage_path" env:"ISO_STORAGE_PATH"`
}

// PowerDNSConfig holds PowerDNS integration configuration.
type PowerDNSConfig struct {
	APIURL   string `yaml:"api_url" env:"POWERDNS_API_URL"`
	APIKey   string `yaml:"api_key" env:"POWERDNS_API_KEY"`
	ServerID string `yaml:"server_id" env:"POWERDNS_SERVER_ID"`
	ZoneName string `yaml:"zone_name" env:"POWERDNS_ZONE_NAME"`
	MySQLURL string `yaml:"mysql_url" env:"POWERDNS_MYSQL_URL"` // MySQL connection string for direct database integration
}

// RedisConfig holds Redis connection settings used for distributed rate limiting.
type RedisConfig struct {
	URL string `yaml:"url" env:"REDIS_URL"`
}

// BillingProviderConfig holds toggle/priority settings for a single billing provider.
type BillingProviderConfig struct {
	Enabled bool `yaml:"enabled"`
	Primary bool `yaml:"primary"`
}

// BillingProvidersConfig holds per-provider billing settings.
type BillingProvidersConfig struct {
	WHMCS  BillingProviderConfig `yaml:"whmcs"`
	Native BillingProviderConfig `yaml:"native"`
	Blesta BillingProviderConfig `yaml:"blesta"`
}

// BillingConfig holds billing system configuration.
type BillingConfig struct {
	Providers            BillingProvidersConfig `yaml:"providers"`
	GracePeriodHours     int                    `yaml:"grace_period_hours"`
	WarningIntervalHours int                    `yaml:"warning_interval_hours"`
	AutoDeleteDays       int                    `yaml:"auto_delete_days"`
}

// StripeConfig holds Stripe payment gateway settings.
type StripeConfig struct {
	SecretKey      Secret `yaml:"secret_key"`
	WebhookSecret  Secret `yaml:"webhook_secret"`
	PublishableKey string `yaml:"publishable_key"`
}

// PayPalConfig holds PayPal payment gateway settings.
type PayPalConfig struct {
	ClientID     Secret `yaml:"client_id"`
	ClientSecret Secret `yaml:"client_secret"`
	Mode         string `yaml:"mode"` // "sandbox" or "production"
}

// CryptoConfig holds cryptocurrency payment settings.
type CryptoConfig struct {
	Provider          string `yaml:"provider"` // "btcpay", "nowpayments", or "disabled"
	BTCPayServerURL   string `yaml:"btcpay_server_url"`
	BTCPayAPIKey      Secret `yaml:"btcpay_api_key"`
	BTCPayStoreID     string `yaml:"btcpay_store_id"`
	NOWPaymentsAPIKey Secret `yaml:"nowpayments_api_key"`
	NOWPaymentsIPNSecret Secret `yaml:"nowpayments_ipn_secret"`
}

// OAuthProviderConfig holds settings for a single OAuth provider.
type OAuthProviderConfig struct {
	Enabled      bool   `yaml:"enabled"`
	ClientID     string `yaml:"client_id"`
	ClientSecret Secret `yaml:"client_secret"`
}

// OAuthConfig holds OAuth provider settings.
type OAuthConfig struct {
	Google OAuthProviderConfig `yaml:"google"`
	GitHub OAuthProviderConfig `yaml:"github"`
}

// ControllerConfig holds all configuration for the VirtueStack Controller.
type ControllerConfig struct {
	DatabaseURL    string   `yaml:"database_url" env:"DATABASE_URL"`
	JWTSecret      Secret   `yaml:"jwt_secret" env:"JWT_SECRET"`
	EncryptionKey  Secret   `yaml:"encryption_key" env:"ENCRYPTION_KEY"`
	ListenAddr     string   `yaml:"listen_addr" env:"LISTEN_ADDR"`
	LogLevel       string   `yaml:"log_level" env:"LOG_LEVEL"`
	Environment    string   `yaml:"environment" env:"APP_ENV"`
	ConsoleBaseURL string   `yaml:"console_base_url" env:"CONSOLE_BASE_URL"`
	CORSOrigins    []string `yaml:"cors_origins" env:"CORS_ORIGINS"`
	DNSNameservers []string `yaml:"dns_nameservers" env:"DNS_NAMESERVERS"`
	CephUser       string   `yaml:"ceph_user" env:"CEPH_USER"`
	CephSecretUUID string   `yaml:"ceph_secret_uuid" env:"CEPH_SECRET_UUID"`
	CephMonitors   []string `yaml:"ceph_monitors" env:"CEPH_MONITORS"`
	AllowSelfRegistration      bool `yaml:"allow_self_registration" env:"ALLOW_SELF_REGISTRATION"`
	RegistrationEmailVerification bool `yaml:"registration_email_verification" env:"REGISTRATION_EMAIL_VERIFICATION"`

	// NATS configuration
	NATS NATSConfig `yaml:"nats"`

	// Optional configurations
	SMTP        SMTPConfig        `yaml:"smtp"`
	Telegram    TelegramConfig    `yaml:"telegram"`
	Backup      BackupConfig      `yaml:"backup"`
	PowerDNS    PowerDNSConfig    `yaml:"powerdns"`
	Redis       RedisConfig       `yaml:"redis"`
	FileStorage FileStorageConfig `yaml:"file_storage"`

	// Billing configuration
	Billing BillingConfig `yaml:"billing"`

	// Payment gateway configuration
	Stripe StripeConfig `yaml:"stripe"`
	PayPal PayPalConfig `yaml:"paypal"`
	Crypto CryptoConfig `yaml:"crypto"`

	// OAuth configuration
	OAuth OAuthConfig `yaml:"oauth"`
}

// AnyBillingProviderEnabled returns true if at least one billing provider is enabled.
func (c *ControllerConfig) AnyBillingProviderEnabled() bool {
	return c.Billing.Providers.WHMCS.Enabled ||
		c.Billing.Providers.Native.Enabled ||
		c.Billing.Providers.Blesta.Enabled
}

// PrimaryBillingProvider returns the name of the primary billing provider,
// or an empty string if none is marked primary.
func (c *ControllerConfig) PrimaryBillingProvider() string {
	if c.Billing.Providers.WHMCS.Primary {
		return "whmcs"
	}
	if c.Billing.Providers.Native.Primary {
		return "native"
	}
	if c.Billing.Providers.Blesta.Primary {
		return "blesta"
	}
	return ""
}

// HasPaymentGateway returns true if any payment gateway (Stripe, PayPal, or crypto)
// is configured.
func (c *ControllerConfig) HasPaymentGateway() bool {
	if c.Stripe.SecretKey != "" {
		return true
	}
	if c.PayPal.ClientID != "" {
		return true
	}
	if c.Crypto.Provider != "" && c.Crypto.Provider != "disabled" {
		return true
	}
	return false
}

// NodeAgentConfig holds all configuration for the VirtueStack Node Agent.
type NodeAgentConfig struct {
	ControllerGRPCAddr string `yaml:"controller_grpc_addr" env:"CONTROLLER_GRPC_ADDR"`
	NodeID             string `yaml:"node_id" env:"NODE_ID"`

	// Libvirt configuration
	LibvirtURI string `yaml:"libvirt_uri" env:"LIBVIRT_URI"`

	// VNC configuration
	VNCHost string `yaml:"vnc_host" env:"VNC_HOST"`

	// Storage configuration
	StorageBackend string `yaml:"storage_backend" env:"STORAGE_BACKEND"` // "ceph", "qcow", or "lvm"
	StoragePath    string `yaml:"storage_path" env:"STORAGE_PATH"`       // Base path for file storage (e.g., /var/lib/virtuestack)

	// Ceph storage configuration (used when StorageBackend == "ceph")
	CephPool string `yaml:"ceph_pool" env:"CEPH_POOL"`
	CephUser string `yaml:"ceph_user" env:"CEPH_USER"`
	CephConf string `yaml:"ceph_conf" env:"CEPH_CONF"`

	// LVM thin-provisioned storage configuration (used when StorageBackend == "lvm").
	// Both fields are required; there is no thick-LVM fallback.
	// LVMThinPool must be the name of a pre-existing thin-pool LV within the VG.
	LVMVolumeGroup string `yaml:"lvm_volume_group" env:"LVM_VOLUME_GROUP"` // e.g. "vgvs"
	LVMThinPool    string `yaml:"lvm_thin_pool"    env:"LVM_THIN_POOL"`    // e.g. "thinpool"

	// LVM threshold configuration for thin pool monitoring.
	// Alerts are triggered when usage exceeds these percentages.
	LVMDataPercentThreshold     int `yaml:"lvm_data_percent_threshold"      env:"LVM_DATA_PERCENT_THRESHOLD"`     // Default: 95
	LVMMetadataPercentThreshold int `yaml:"lvm_metadata_percent_threshold"  env:"LVM_METADATA_PERCENT_THRESHOLD"` // Default: 70

	// TLS configuration for gRPC
	TLSCertFile string `yaml:"tls_cert_file" env:"TLS_CERT_FILE"`
	TLSKeyFile  string `yaml:"tls_key_file" env:"TLS_KEY_FILE"`
	TLSCAFile   string `yaml:"tls_ca_file" env:"TLS_CA_FILE"`

	// Paths
	DataDir        string `yaml:"data_dir" env:"DATA_DIR"`
	CloudInitPath  string `yaml:"cloudinit_path" env:"CLOUDINIT_PATH"`
	ISOStoragePath string `yaml:"iso_storage_path" env:"ISO_STORAGE_PATH"`

	// Logging
	LogLevel string `yaml:"log_level" env:"LOG_LEVEL"`

	// Metrics
	MetricsAddr            string `yaml:"metrics_addr" env:"METRICS_ADDR"`
	MetricsCollectInterval string `yaml:"metrics_collect_interval" env:"METRICS_COLLECT_INTERVAL"` // e.g., "60s", "5m"

	// Health HTTP server for storage backend health checks
	// Binds to localhost only for security (no external access)
	HealthAddr string `yaml:"health_addr" env:"HEALTH_ADDR"` // e.g., "127.0.0.1:8081"

	// Shutdown timeout for graceful termination (e.g., "30s", "1m")
	ShutdownTimeout string `yaml:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT"`

	// GuestOpHMACSecret is used to verify per-operation HMAC tokens sent by the
	// controller for sensitive guest operations (e.g., GuestSetPassword).
	// Must be at least 32 bytes when non-empty.
	GuestOpHMACSecret Secret `yaml:"guest_op_hmac_secret" env:"GUEST_OP_HMAC_SECRET"`
}

// LoadControllerConfig loads the controller configuration from environment variables
// and optionally from a YAML file if VS_CONFIG_FILE is set.
// Environment variables take precedence over YAML values.
// Required fields: DatabaseURL, NATS.URL, JWTSecret, EncryptionKey.
func LoadControllerConfig() (*ControllerConfig, error) {
	cfg := &ControllerConfig{
		ListenAddr:     defaultListenAddr,
		LogLevel:       defaultLogLevel,
		ConsoleBaseURL: defaultConsoleBaseURL,
		DNSNameservers: splitAndTrimCSV(defaultDNSNameservers),
		CephUser:       defaultCephUser,
		AllowSelfRegistration: false,
		RegistrationEmailVerification: true,
		Billing: BillingConfig{
			Providers: BillingProvidersConfig{
				WHMCS: BillingProviderConfig{Enabled: true},
			},
			GracePeriodHours:     12,
			WarningIntervalHours: 24,
		},
		PayPal: PayPalConfig{Mode: "sandbox"},
		Crypto: CryptoConfig{Provider: "disabled"},
	}

	// Load from YAML file if specified
	configFile := os.Getenv("VS_CONFIG_FILE")
	if configFile != "" {
		if err := loadYAMLFile(configFile, cfg); err != nil {
			return nil, fmt.Errorf("loading YAML config file: %w", err)
		}
	}

	// Override with environment variables
	applyEnvOverrides(cfg)

	// Validate required fields
	if err := validateControllerConfig(cfg); err != nil {
		return nil, fmt.Errorf("validating controller config: %w", err)
	}

	// Validate passwords
	if err := validatePasswords(); err != nil {
		return nil, fmt.Errorf("password validation failed: %w", err)
	}

	// Validate billing config
	if err := validateBillingConfig(cfg); err != nil {
		return nil, fmt.Errorf("billing config validation: %w", err)
	}

	// Check for weak default passwords (fatal in production)
	if err := validateDefaultPasswords(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadNodeAgentConfig loads the node agent configuration from environment variables
// and optionally from a YAML file if VS_CONFIG_FILE is set.
// Environment variables take precedence over YAML values.
// Required fields: ControllerGRPCAddr, NodeID, TLSCertFile, TLSKeyFile, TLSCAFile.
func LoadNodeAgentConfig() (*NodeAgentConfig, error) {
	cfg := &NodeAgentConfig{
		StorageBackend:              defaultStorageBackend,
		StoragePath:                 defaultStoragePath,
		CephPool:                    defaultCephPool,
		CephUser:                    defaultCephUser,
		CephConf:                    defaultCephConf,
		CloudInitPath:               defaultCloudInitPath,
		ISOStoragePath:              defaultISOStoragePath,
		LogLevel:                    defaultLogLevel,
		MetricsAddr:                 ":9091",
		HealthAddr:                  "127.0.0.1:8081",
		LVMDataPercentThreshold:     defaultLVMDataPercentThreshold,
		LVMMetadataPercentThreshold: defaultLVMMetadataPercentThreshold,
	}

	// Load from YAML file if specified
	configFile := os.Getenv("VS_CONFIG_FILE")
	if configFile != "" {
		if err := loadYAMLFile(configFile, cfg); err != nil {
			return nil, fmt.Errorf("loading YAML config file: %w", err)
		}
	}

	// Override with environment variables
	applyEnvOverridesNodeAgent(cfg)

	// Validate required fields
	if err := validateNodeAgentConfig(cfg); err != nil {
		return nil, fmt.Errorf("validating node agent config: %w", err)
	}

	return cfg, nil
}

// loadYAMLFile reads and unmarshals a YAML configuration file into cfg.
// The type parameter T constrains callers to the known config struct types,
// preventing accidental use with arbitrary types.
func loadYAMLFile[T *ControllerConfig | *NodeAgentConfig](filename string, cfg T) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("reading config file %q: %w", filename, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parsing YAML config: %w", err)
	}

	return nil
}

// applyEnvOverrides applies environment variable values to ControllerConfig.
// Environment variables take precedence over YAML values.
// It delegates to focused sub-functions for each configuration domain.
func applyEnvOverrides(cfg *ControllerConfig) {
	applyEnvOverridesCore(cfg)
	applyEnvOverridesNATS(cfg)
	applyEnvOverridesSMTP(cfg)
	applyEnvOverridesTelegram(cfg)
	applyEnvOverridesStorage(cfg)
	applyEnvOverridesBilling(cfg)
	applyEnvOverridesPayments(cfg)
	applyEnvOverridesOAuth(cfg)
}

// applyEnvOverridesCore applies DB, JWT, encryption, listen addr, log level, and
// other top-level controller environment variables.
func applyEnvOverridesCore(cfg *ControllerConfig) {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWTSecret = Secret(v)
	}
	if v := os.Getenv("ENCRYPTION_KEY"); v != "" {
		cfg.EncryptionKey = Secret(v)
	}
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("APP_ENV"); v != "" {
		cfg.Environment = v
	}
	if v := os.Getenv("CONSOLE_BASE_URL"); v != "" {
		cfg.ConsoleBaseURL = v
	}
	if v := os.Getenv("CORS_ORIGINS"); v != "" {
		cfg.CORSOrigins = splitAndTrimCSV(v)
	}
	if v := os.Getenv("DNS_NAMESERVERS"); v != "" {
		cfg.DNSNameservers = splitAndTrimCSV(v)
	}
	if v := os.Getenv("CEPH_USER"); v != "" {
		cfg.CephUser = v
	}
	if v := os.Getenv("CEPH_SECRET_UUID"); v != "" {
		cfg.CephSecretUUID = v
	}
	if v := os.Getenv("CEPH_MONITORS"); v != "" {
		cfg.CephMonitors = splitAndTrimCSV(v)
	}
	if v := os.Getenv("ALLOW_SELF_REGISTRATION"); v != "" {
		cfg.AllowSelfRegistration = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("REGISTRATION_EMAIL_VERIFICATION"); v != "" {
		cfg.RegistrationEmailVerification = strings.EqualFold(v, "true") || v == "1"
	}

	// PowerDNS config
	if v := os.Getenv("POWERDNS_API_URL"); v != "" {
		cfg.PowerDNS.APIURL = v
	}
	if v := os.Getenv("POWERDNS_SERVER_ID"); v != "" {
		cfg.PowerDNS.ServerID = v
	}
	if v := os.Getenv("POWERDNS_ZONE_NAME"); v != "" {
		cfg.PowerDNS.ZoneName = v
	}
	if v := os.Getenv("PDNS_MYSQL_DSN"); v != "" {
		cfg.PowerDNS.MySQLURL = v
	}
	if v := os.Getenv("POWERDNS_MYSQL_URL"); v != "" {
		cfg.PowerDNS.MySQLURL = v
	}
	if v := os.Getenv("POWERDNS_API_KEY"); v != "" {
		cfg.PowerDNS.APIKey = v
	}
	if v := os.Getenv("REDIS_URL"); v != "" {
		cfg.Redis.URL = v
	}
}

// applyEnvOverridesNATS applies NATS-related environment variables.
func applyEnvOverridesNATS(cfg *ControllerConfig) {
	if v := os.Getenv("NATS_URL"); v != "" {
		cfg.NATS.URL = v
	}
	if v := os.Getenv("NATS_AUTH_TOKEN"); v != "" {
		cfg.NATS.AuthToken = v
	}
}

// applyEnvOverridesSMTP applies all SMTP-related environment variables.
func applyEnvOverridesSMTP(cfg *ControllerConfig) {
	if v := os.Getenv("SMTP_HOST"); v != "" {
		cfg.SMTP.Host = v
	}
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.SMTP.Port = port
		}
	}
	if v := os.Getenv("SMTP_USER"); v != "" {
		cfg.SMTP.User = v
	}
	if v := os.Getenv("SMTP_PASSWORD"); v != "" {
		cfg.SMTP.Password = v
	}
	if v := os.Getenv("SMTP_FROM"); v != "" {
		cfg.SMTP.From = v
	}
	if v := os.Getenv("SMTP_ENABLED"); v != "" {
		cfg.SMTP.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("SMTP_REQUIRE_TLS"); v != "" {
		cfg.SMTP.RequireTLS = strings.EqualFold(v, "true") || v == "1"
	}
}

// applyEnvOverridesTelegram applies all Telegram-related environment variables.
func applyEnvOverridesTelegram(cfg *ControllerConfig) {
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Telegram.BotToken = v
	}
	if v := os.Getenv("TELEGRAM_CHAT_ID"); v != "" {
		cfg.Telegram.ChatID = v
	}
}

// applyEnvOverridesStorage applies file-storage-related environment variables.
func applyEnvOverridesStorage(cfg *ControllerConfig) {
	if v := os.Getenv("TEMPLATE_IMPORT_PATHS"); v != "" {
		cfg.FileStorage.TemplateImportPaths = splitAndTrimCSV(v)
	}
	if v := os.Getenv("BACKUP_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.FileStorage.BackupRetentionDays = n
		} else {
			slog.Warn("invalid BACKUP_RETENTION_DAYS value, ignoring", "value", v, "error", err)
		}
	}
	if v := os.Getenv("MAX_TEMPLATE_SIZE_GB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.FileStorage.MaxTemplateSizeGB = n
		} else {
			slog.Warn("invalid MAX_TEMPLATE_SIZE_GB value, ignoring", "value", v, "error", err)
		}
	}
	if v := os.Getenv("ISO_STORAGE_PATH"); v != "" {
		cfg.FileStorage.ISOStoragePath = v
	}
}

// applyEnvOverridesOAuth applies OAuth-related environment variables.
func applyEnvOverridesOAuth(cfg *ControllerConfig) {
	if v := os.Getenv("OAUTH_GOOGLE_ENABLED"); v != "" {
		cfg.OAuth.Google.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("OAUTH_GOOGLE_CLIENT_ID"); v != "" {
		cfg.OAuth.Google.ClientID = v
	}
	if v := os.Getenv("OAUTH_GOOGLE_CLIENT_SECRET"); v != "" {
		cfg.OAuth.Google.ClientSecret = Secret(v)
	}
	if v := os.Getenv("OAUTH_GITHUB_ENABLED"); v != "" {
		cfg.OAuth.GitHub.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("OAUTH_GITHUB_CLIENT_ID"); v != "" {
		cfg.OAuth.GitHub.ClientID = v
	}
	if v := os.Getenv("OAUTH_GITHUB_CLIENT_SECRET"); v != "" {
		cfg.OAuth.GitHub.ClientSecret = Secret(v)
	}
}

// applyEnvOverridesPayments applies payment gateway environment variables.
func applyEnvOverridesPayments(cfg *ControllerConfig) {
	// Stripe
	if v := os.Getenv("STRIPE_SECRET_KEY"); v != "" {
		cfg.Stripe.SecretKey = Secret(v)
	}
	if v := os.Getenv("STRIPE_WEBHOOK_SECRET"); v != "" {
		cfg.Stripe.WebhookSecret = Secret(v)
	}
	if v := os.Getenv("STRIPE_PUBLISHABLE_KEY"); v != "" {
		cfg.Stripe.PublishableKey = v
	}
	// PayPal
	if v := os.Getenv("PAYPAL_CLIENT_ID"); v != "" {
		cfg.PayPal.ClientID = Secret(v)
	}
	if v := os.Getenv("PAYPAL_CLIENT_SECRET"); v != "" {
		cfg.PayPal.ClientSecret = Secret(v)
	}
	if v := os.Getenv("PAYPAL_MODE"); v != "" {
		cfg.PayPal.Mode = v
	}
	// Crypto
	if v := os.Getenv("CRYPTO_PROVIDER"); v != "" {
		cfg.Crypto.Provider = v
	}
	if v := os.Getenv("BTCPAY_SERVER_URL"); v != "" {
		cfg.Crypto.BTCPayServerURL = v
	}
	if v := os.Getenv("BTCPAY_API_KEY"); v != "" {
		cfg.Crypto.BTCPayAPIKey = Secret(v)
	}
	if v := os.Getenv("BTCPAY_STORE_ID"); v != "" {
		cfg.Crypto.BTCPayStoreID = v
	}
	if v := os.Getenv("NOWPAYMENTS_API_KEY"); v != "" {
		cfg.Crypto.NOWPaymentsAPIKey = Secret(v)
	}
	if v := os.Getenv("NOWPAYMENTS_IPN_SECRET"); v != "" {
		cfg.Crypto.NOWPaymentsIPNSecret = Secret(v)
	}
}

// applyEnvOverridesBilling applies billing-related environment variables.
func applyEnvOverridesBilling(cfg *ControllerConfig) {
	if v := os.Getenv("BILLING_WHMCS_ENABLED"); v != "" {
		cfg.Billing.Providers.WHMCS.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("BILLING_WHMCS_PRIMARY"); v != "" {
		cfg.Billing.Providers.WHMCS.Primary = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("BILLING_NATIVE_ENABLED"); v != "" {
		cfg.Billing.Providers.Native.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("BILLING_NATIVE_PRIMARY"); v != "" {
		cfg.Billing.Providers.Native.Primary = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("BILLING_BLESTA_ENABLED"); v != "" {
		cfg.Billing.Providers.Blesta.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("BILLING_BLESTA_PRIMARY"); v != "" {
		cfg.Billing.Providers.Blesta.Primary = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("BILLING_GRACE_PERIOD_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Billing.GracePeriodHours = n
		}
	}
	if v := os.Getenv("BILLING_WARNING_INTERVAL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Billing.WarningIntervalHours = n
		}
	}
	if v := os.Getenv("BILLING_NATIVE_AUTO_DELETE_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Billing.AutoDeleteDays = n
		}
	}
}

// applyEnvOverridesNodeAgent applies environment variable values to NodeAgentConfig.
// Environment variables take precedence over YAML values.
// It delegates storage-related overrides to applyEnvOverridesNodeAgentStorage.
func applyEnvOverridesNodeAgent(cfg *NodeAgentConfig) {
	if v := os.Getenv("CONTROLLER_GRPC_ADDR"); v != "" {
		cfg.ControllerGRPCAddr = v
	}
	if v := os.Getenv("NODE_ID"); v != "" {
		cfg.NodeID = v
	}
	if v := os.Getenv("LIBVIRT_URI"); v != "" {
		cfg.LibvirtURI = v
	}
	if v := os.Getenv("VNC_HOST"); v != "" {
		cfg.VNCHost = v
	}
	if v := os.Getenv("TLS_CERT_FILE"); v != "" {
		cfg.TLSCertFile = v
	}
	if v := os.Getenv("TLS_KEY_FILE"); v != "" {
		cfg.TLSKeyFile = v
	}
	if v := os.Getenv("TLS_CA_FILE"); v != "" {
		cfg.TLSCAFile = v
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("CLOUDINIT_PATH"); v != "" {
		cfg.CloudInitPath = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("METRICS_ADDR"); v != "" {
		cfg.MetricsAddr = v
	}
	if v := os.Getenv("METRICS_COLLECT_INTERVAL"); v != "" {
		cfg.MetricsCollectInterval = v
	}
	if v := os.Getenv("HEALTH_ADDR"); v != "" {
		cfg.HealthAddr = v
	}
	if v := os.Getenv("SHUTDOWN_TIMEOUT"); v != "" {
		cfg.ShutdownTimeout = v
	}
	if v := os.Getenv("GUEST_OP_HMAC_SECRET"); v != "" {
		cfg.GuestOpHMACSecret = Secret(v)
	}

	applyEnvOverridesNodeAgentStorage(cfg)
}

// applyEnvOverridesNodeAgentStorage applies storage-related environment variables
// to NodeAgentConfig (storage backend, paths, and Ceph settings).
func applyEnvOverridesNodeAgentStorage(cfg *NodeAgentConfig) {
	if v := os.Getenv("STORAGE_BACKEND"); v != "" {
		cfg.StorageBackend = v
	}
	if v := os.Getenv("STORAGE_PATH"); v != "" {
		cfg.StoragePath = v
	}
	if v := os.Getenv("ISO_STORAGE_PATH"); v != "" {
		cfg.ISOStoragePath = v
	}
	if v := os.Getenv("CEPH_POOL"); v != "" {
		cfg.CephPool = v
	}
	if v := os.Getenv("CEPH_USER"); v != "" {
		cfg.CephUser = v
	}
	if v := os.Getenv("CEPH_CONF"); v != "" {
		cfg.CephConf = v
	}
}

// validateControllerConfig validates that all required fields are set.
// For production deployments it additionally calls validateProductionConfig.
func validateControllerConfig(cfg *ControllerConfig) error {
	var missing []string

	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.NATS.URL == "" {
		missing = append(missing, "NATS_URL")
	}
	if cfg.JWTSecret == "" {
		missing = append(missing, "JWT_SECRET")
	}
	if cfg.EncryptionKey == "" {
		missing = append(missing, "ENCRYPTION_KEY")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}

	if len(cfg.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}
	// EncryptionKey format and length (must be a valid 64-character hex string encoding
	// exactly 32 bytes for AES-256) is validated in controller.LoadConfig after hex
	// decoding. Presence-only check is performed here to keep validation in a single place.

	// F-156: Fail startup when a known-insecure JWT secret is used outside "local"/"dev".
	knownInsecureJWT := cfg.JWTSecret == "dev-jwt-secret-min-32-characters-long"
	if knownInsecureJWT {
		env := strings.ToLower(cfg.Environment)
		if env == "local" || env == "dev" || env == "" {
			slog.Warn("using default insecure JWT_SECRET; set a unique JWT_SECRET for production")
		} else {
			return fmt.Errorf("JWT_SECRET matches a known insecure default; set a unique secret for environment %q", cfg.Environment)
		}
	}
	if cfg.NATS.AuthToken == natsDevToken {
		slog.Warn("using default NATS_AUTH_TOKEN 'nats-dev-token'; set a strong token for production")
	}

	if strings.EqualFold(cfg.Environment, "production") {
		if err := validateProductionConfig(cfg); err != nil {
			return err
		}
	}

	return nil
}

// natsDevToken is the hardcoded default NATS auth token used in development.
// Its use in production is rejected by validateProductionConfig.
const natsDevToken = "nats-dev-token"

// validateProductionConfig performs additional validation that is only enforced
// when the application is running in production mode. It rejects known-insecure
// default values for secrets and database credentials.
func validateProductionConfig(cfg *ControllerConfig) error {
	knownBadDefaults := map[string]bool{
		"devpassword":                           true,
		"dev-jwt-secret-min-32-characters-long": true,
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef": true,
	}

	if knownBadDefaults[cfg.JWTSecret.Value()] {
		return fmt.Errorf("JWT_SECRET must not use a known insecure default in production")
	}
	if knownBadDefaults[cfg.EncryptionKey.Value()] {
		return fmt.Errorf("ENCRYPTION_KEY must not use a known insecure default in production")
	}
	if strings.Contains(cfg.DatabaseURL, "devpassword") {
		return fmt.Errorf("DATABASE_URL contains a known insecure default password; use a strong password in production")
	}
	if cfg.NATS.AuthToken == natsDevToken {
		return fmt.Errorf("NATS_AUTH_TOKEN must not use the default development token in production; set a strong NATS_AUTH_TOKEN")
	}
	if len(cfg.NATS.AuthToken) < 32 {
		return fmt.Errorf("NATS_AUTH_TOKEN must be at least 32 characters in production; current length: %d", len(cfg.NATS.AuthToken))
	}

	return nil
}

// validateBillingConfig validates billing, payment, and OAuth configuration.
func validateBillingConfig(cfg *ControllerConfig) error {
	providers := []struct {
		name    string
		enabled bool
		primary bool
	}{
		{"whmcs", cfg.Billing.Providers.WHMCS.Enabled, cfg.Billing.Providers.WHMCS.Primary},
		{"native", cfg.Billing.Providers.Native.Enabled, cfg.Billing.Providers.Native.Primary},
		{"blesta", cfg.Billing.Providers.Blesta.Enabled, cfg.Billing.Providers.Blesta.Primary},
	}

	primaryCount := 0
	anyEnabled := false
	for _, p := range providers {
		if p.primary && !p.enabled {
			return fmt.Errorf("billing provider %q is marked primary but not enabled", p.name)
		}
		if p.primary {
			primaryCount++
		}
		if p.enabled {
			anyEnabled = true
		}
	}
	if anyEnabled && primaryCount != 1 {
		return fmt.Errorf("exactly one enabled billing provider must be primary, found %d", primaryCount)
	}
	if cfg.AllowSelfRegistration && anyEnabled && primaryCount == 0 {
		return fmt.Errorf("self-registration requires a primary billing provider")
	}

	nativeEnabled := cfg.Billing.Providers.Native.Enabled
	hasGateway := cfg.Stripe.SecretKey != "" || cfg.PayPal.ClientID != "" ||
		(cfg.Crypto.Provider != "" && cfg.Crypto.Provider != "disabled")
	if nativeEnabled && !hasGateway {
		slog.Warn("native billing enabled without any payment gateway configured")
	}

	if err := validateStripeConfig(&cfg.Stripe); err != nil {
		return err
	}
	if err := validatePayPalConfig(&cfg.PayPal); err != nil {
		return err
	}
	if err := validateCryptoConfig(&cfg.Crypto); err != nil {
		return err
	}
	if err := validateOAuthConfig(&cfg.OAuth); err != nil {
		return err
	}

	if cfg.Billing.GracePeriodHours < 0 {
		return fmt.Errorf("billing grace_period_hours must be non-negative")
	}
	if cfg.Billing.WarningIntervalHours < 0 {
		return fmt.Errorf("billing warning_interval_hours must be non-negative")
	}
	if cfg.Billing.AutoDeleteDays < 0 {
		return fmt.Errorf("billing auto_delete_days must be non-negative")
	}
	return nil
}

func validateStripeConfig(s *StripeConfig) error {
	hasKey := s.SecretKey != ""
	hasWebhook := s.WebhookSecret != ""
	if hasKey != hasWebhook {
		return fmt.Errorf("stripe: secret_key and webhook_secret must both be set or both empty")
	}
	return nil
}

func validatePayPalConfig(p *PayPalConfig) error {
	hasID := p.ClientID != ""
	hasSecret := p.ClientSecret != ""
	if hasID != hasSecret {
		return fmt.Errorf("paypal: client_id and client_secret must both be set or both empty")
	}
	if p.Mode != "sandbox" && p.Mode != "production" {
		return fmt.Errorf("paypal: mode must be \"sandbox\" or \"production\", got %q", p.Mode)
	}
	return nil
}

func validateCryptoConfig(c *CryptoConfig) error {
	switch c.Provider {
	case "disabled", "":
		return nil
	case "btcpay":
		if c.BTCPayServerURL == "" || c.BTCPayAPIKey == "" || c.BTCPayStoreID == "" {
			return fmt.Errorf("crypto: btcpay requires server_url, api_key, and store_id")
		}
	case "nowpayments":
		if c.NOWPaymentsAPIKey == "" || c.NOWPaymentsIPNSecret == "" {
			return fmt.Errorf("crypto: nowpayments requires api_key and ipn_secret")
		}
	default:
		return fmt.Errorf("crypto: provider must be \"btcpay\", \"nowpayments\", or \"disabled\", got %q", c.Provider)
	}
	return nil
}

func validateOAuthConfig(o *OAuthConfig) error {
	if o.Google.Enabled && (o.Google.ClientID == "" || o.Google.ClientSecret == "") {
		return fmt.Errorf("oauth: google enabled but client_id or client_secret missing")
	}
	if o.GitHub.Enabled && (o.GitHub.ClientID == "" || o.GitHub.ClientSecret == "") {
		return fmt.Errorf("oauth: github enabled but client_id or client_secret missing")
	}
	return nil
}

// validateNodeAgentConfig validates that all required fields are set.
func validateNodeAgentConfig(cfg *NodeAgentConfig) error {
	var missing []string

	if cfg.ControllerGRPCAddr == "" {
		missing = append(missing, "CONTROLLER_GRPC_ADDR")
	}
	if cfg.NodeID == "" {
		missing = append(missing, "NODE_ID")
	}
	if cfg.TLSCertFile == "" {
		missing = append(missing, "TLS_CERT_FILE")
	}
	if cfg.TLSKeyFile == "" {
		missing = append(missing, "TLS_KEY_FILE")
	}
	if cfg.TLSCAFile == "" {
		missing = append(missing, "TLS_CA_FILE")
	}

	// Validate storage backend specific requirements
	switch cfg.StorageBackend {
	case "qcow":
		if cfg.StoragePath == "" {
			missing = append(missing, "STORAGE_PATH (required when storage_backend is qcow)")
		}
	case "ceph":
		// Validate Ceph settings
		if cfg.CephPool == "" {
			missing = append(missing, "CEPH_POOL (required when storage_backend is ceph)")
		}
		if cfg.CephUser == "" {
			missing = append(missing, "CEPH_USER (required when storage_backend is ceph)")
		}
		if cfg.CephConf == "" {
			missing = append(missing, "CEPH_CONF (required when storage_backend is ceph)")
		}
	case "lvm":
		if cfg.LVMVolumeGroup == "" {
			missing = append(missing, "LVM_VOLUME_GROUP (required when storage_backend is lvm)")
		}
		if cfg.LVMThinPool == "" {
			missing = append(missing, "LVM_THIN_POOL (required when storage_backend is lvm)")
		}
	case "":
		// Empty is valid (uses default)
	default:
		return fmt.Errorf("invalid storage_backend %q: must be 'ceph', 'qcow', or 'lvm'", cfg.StorageBackend)
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}

	return nil
}

// isWeakPassword checks if a password is in the weak password list.
func isWeakPassword(password string) bool {
	return weakPasswords[strings.ToLower(password)]
}

// validatePasswords checks password environment variables for weak values.
func validatePasswords() error {
	passwordEnvVars := []string{
		"DB_PASSWORD",
		"ADMIN_PASSWORD",
		"CUSTOMER_PASSWORD",
	}

	var weakFound []string

	for _, envVar := range passwordEnvVars {
		password := os.Getenv(envVar)
		if password == "" {
			continue
		}
		if isWeakPassword(password) {
			weakFound = append(weakFound, envVar)
		}
	}

	if len(weakFound) > 0 {
		return fmt.Errorf("weak password detected in: %s", strings.Join(weakFound, ", "))
	}

	return nil
}

// validateDefaultPasswords logs warnings for default/placeholder passwords.
// In production mode, refuses to start with weak passwords.
func validateDefaultPasswords() error {
	defaultPasswordEnvVars := []string{
		"DB_PASSWORD",
		"ADMIN_PASSWORD",
		"CUSTOMER_PASSWORD",
	}

	isProduction := strings.EqualFold(os.Getenv("APP_ENV"), "production")
	var weakFound []string

	for _, envVar := range defaultPasswordEnvVars {
		password := os.Getenv(envVar)
		if password == "" {
			continue
		}
		if isWeakPassword(password) || len(password) < 12 {
			weakFound = append(weakFound, envVar)
		}
	}

	if len(weakFound) > 0 {
		if isProduction {
			return fmt.Errorf("FATAL: weak or short (<12 chars) passwords detected in production for: %s", strings.Join(weakFound, ", "))
		}
		// Weak passwords are logged by the logging middleware; skip console output here
	}

	return nil
}

func splitAndTrimCSV(input string) []string {
	parts := strings.Split(input, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}
