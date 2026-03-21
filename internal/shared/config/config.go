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

	// NATS configuration
	NATS NATSConfig `yaml:"nats"`

	// Optional configurations
	SMTP        SMTPConfig        `yaml:"smtp"`
	Telegram    TelegramConfig    `yaml:"telegram"`
	Backup      BackupConfig      `yaml:"backup"`
	PowerDNS    PowerDNSConfig    `yaml:"powerdns"`
	FileStorage FileStorageConfig `yaml:"file_storage"`
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
	StorageBackend string `yaml:"storage_backend" env:"STORAGE_BACKEND"` // "ceph" or "qcow"
	StoragePath    string `yaml:"storage_path" env:"STORAGE_PATH"`       // Base path for file storage (e.g., /var/lib/virtuestack)

	// Ceph storage configuration (used when StorageBackend == "ceph")
	CephPool string `yaml:"ceph_pool" env:"CEPH_POOL"`
	CephUser string `yaml:"ceph_user" env:"CEPH_USER"`
	CephConf string `yaml:"ceph_conf" env:"CEPH_CONF"`

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

	// Shutdown timeout for graceful termination (e.g., "30s", "1m")
	ShutdownTimeout string `yaml:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT"`
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
		StorageBackend: defaultStorageBackend,
		StoragePath:    defaultStoragePath,
		CephPool:       defaultCephPool,
		CephUser:       defaultCephUser,
		CephConf:       defaultCephConf,
		CloudInitPath:  defaultCloudInitPath,
		ISOStoragePath: defaultISOStoragePath,
		LogLevel:       defaultLogLevel,
		MetricsAddr:    ":9091",
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
	if v := os.Getenv("SHUTDOWN_TIMEOUT"); v != "" {
		cfg.ShutdownTimeout = v
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

	if cfg.JWTSecret == "dev-jwt-secret-min-32-characters-long" {
		slog.Warn("using default insecure JWT_SECRET; set a unique JWT_SECRET for production")
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
	if cfg.StorageBackend == "qcow" {
		if cfg.StoragePath == "" {
			missing = append(missing, "STORAGE_PATH (required when storage_backend is qcow)")
		}
	} else if cfg.StorageBackend == "ceph" {
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
	} else if cfg.StorageBackend != "" && cfg.StorageBackend != "ceph" && cfg.StorageBackend != "qcow" {
		return fmt.Errorf("invalid storage_backend %q: must be 'ceph' or 'qcow'", cfg.StorageBackend)
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
