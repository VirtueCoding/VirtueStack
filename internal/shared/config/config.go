// Package config provides configuration loading for VirtueStack components.
// It supports loading from environment variables (primary) and YAML config files.
// Environment variables take precedence over YAML values.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Default configuration values.
const (
	defaultListenAddr     = ":8080"
	defaultLogLevel      = "info"
	defaultCephPool      = "vs-vms"
	defaultCephUser      = "virtuestack"
	defaultCephConf      = "/etc/ceph/ceph.conf"
	defaultCloudInitPath = "/var/lib/virtuestack/cloud-init"
	defaultISOStoragePath = "/var/lib/virtuestack/iso"
)

// SMTPConfig holds SMTP email configuration.
type SMTPConfig struct {
	Host     string `yaml:"host" env:"SMTP_HOST"`
	Port     int    `yaml:"port" env:"SMTP_PORT"`
	User     string `yaml:"user" env:"SMTP_USER"`
	Password string `yaml:"password" env:"SMTP_PASSWORD"`
	From     string `yaml:"from" env:"SMTP_FROM"`
}

// TelegramConfig holds Telegram notification configuration.
type TelegramConfig struct {
	BotToken string `yaml:"bot_token" env:"TELEGRAM_BOT_TOKEN"`
	ChatID   string `yaml:"chat_id" env:"TELEGRAM_CHAT_ID"`
}

// BackupConfig holds backup configuration.
type BackupConfig struct {
	Enabled      bool   `yaml:"enabled" env:"BACKUP_ENABLED"`
	Schedule     string `yaml:"schedule" env:"BACKUP_SCHEDULE"`
	Retention    int    `yaml:"retention" env:"BACKUP_RETENTION"`
	StoragePath  string `yaml:"storage_path" env:"BACKUP_STORAGE_PATH"`
	RemoteHost   string `yaml:"remote_host" env:"BACKUP_REMOTE_HOST"`
	RemoteUser   string `yaml:"remote_user" env:"BACKUP_REMOTE_USER"`
	RemotePath   string `yaml:"remote_path" env:"BACKUP_REMOTE_PATH"`
}

// PowerDNSConfig holds PowerDNS integration configuration.
type PowerDNSConfig struct {
	APIURL    string `yaml:"api_url" env:"POWERDNS_API_URL"`
	APIKey    string `yaml:"api_key" env:"POWERDNS_API_KEY"`
	ServerID  string `yaml:"server_id" env:"POWERDNS_SERVER_ID"`
	ZoneName  string `yaml:"zone_name" env:"POWERDNS_ZONE_NAME"`
}

// ControllerConfig holds all configuration for the VirtueStack Controller.
type ControllerConfig struct {
	DatabaseURL   string `yaml:"database_url" env:"DATABASE_URL"`
	NatsURL       string `yaml:"nats_url" env:"NATS_URL"`
	JWTSecret     string `yaml:"jwt_secret" env:"JWT_SECRET"`
	EncryptionKey string `yaml:"encryption_key" env:"ENCRYPTION_KEY"`
	ListenAddr    string `yaml:"listen_addr" env:"LISTEN_ADDR"`
	LogLevel      string `yaml:"log_level" env:"LOG_LEVEL"`

	// Optional configurations
	SMTP     SMTPConfig     `yaml:"smtp"`
	Telegram TelegramConfig `yaml:"telegram"`
	Backup   BackupConfig   `yaml:"backup"`
	PowerDNS PowerDNSConfig `yaml:"powerdns"`
}

// NodeAgentConfig holds all configuration for the VirtueStack Node Agent.
type NodeAgentConfig struct {
	ControllerGRPCAddr string `yaml:"controller_grpc_addr" env:"CONTROLLER_GRPC_ADDR"`
	NodeID             string `yaml:"node_id" env:"NODE_ID"`

	// Ceph storage configuration
	CephPool string `yaml:"ceph_pool" env:"CEPH_POOL"`
	CephUser string `yaml:"ceph_user" env:"CEPH_USER"`
	CephConf string `yaml:"ceph_conf" env:"CEPH_CONF"`

	// TLS configuration for gRPC
	TLSCertFile string `yaml:"tls_cert_file" env:"TLS_CERT_FILE"`
	TLSKeyFile  string `yaml:"tls_key_file" env:"TLS_KEY_FILE"`
	TLSCAFile   string `yaml:"tls_ca_file" env:"TLS_CA_FILE"`

	// Paths
	CloudInitPath  string `yaml:"cloudinit_path" env:"CLOUDINIT_PATH"`
	ISOStoragePath string `yaml:"iso_storage_path" env:"ISO_STORAGE_PATH"`

	// Logging
	LogLevel string `yaml:"log_level" env:"LOG_LEVEL"`
}

// LoadControllerConfig loads the controller configuration from environment variables
// and optionally from a YAML file if VS_CONFIG_FILE is set.
// Environment variables take precedence over YAML values.
// Required fields: DatabaseURL, NatsURL, JWTSecret, EncryptionKey.
func LoadControllerConfig() (*ControllerConfig, error) {
	cfg := &ControllerConfig{
		ListenAddr: defaultListenAddr,
		LogLevel:   defaultLogLevel,
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

	return cfg, nil
}

// LoadNodeAgentConfig loads the node agent configuration from environment variables
// and optionally from a YAML file if VS_CONFIG_FILE is set.
// Environment variables take precedence over YAML values.
// Required fields: ControllerGRPCAddr, NodeID, TLSCertFile, TLSKeyFile, TLSCAFile.
func LoadNodeAgentConfig() (*NodeAgentConfig, error) {
	cfg := &NodeAgentConfig{
		CephPool:       defaultCephPool,
		CephUser:       defaultCephUser,
		CephConf:       defaultCephConf,
		CloudInitPath:  defaultCloudInitPath,
		ISOStoragePath: defaultISOStoragePath,
		LogLevel:       defaultLogLevel,
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

// loadYAMLFile reads and unmarshals a YAML configuration file.
func loadYAMLFile(filename string, cfg any) error {
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
func applyEnvOverrides(cfg *ControllerConfig) {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("NATS_URL"); v != "" {
		cfg.NatsURL = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWTSecret = v
	}
	if v := os.Getenv("ENCRYPTION_KEY"); v != "" {
		cfg.EncryptionKey = v
	}
	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	// SMTP config
	if v := os.Getenv("SMTP_HOST"); v != "" {
		cfg.SMTP.Host = v
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

	// Telegram config
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		cfg.Telegram.BotToken = v
	}
	if v := os.Getenv("TELEGRAM_CHAT_ID"); v != "" {
		cfg.Telegram.ChatID = v
	}

	// PowerDNS config
	if v := os.Getenv("POWERDNS_API_URL"); v != "" {
		cfg.PowerDNS.APIURL = v
	}
	if v := os.Getenv("POWERDNS_API_KEY"); v != "" {
		cfg.PowerDNS.APIKey = v
	}
	if v := os.Getenv("POWERDNS_SERVER_ID"); v != "" {
		cfg.PowerDNS.ServerID = v
	}
	if v := os.Getenv("POWERDNS_ZONE_NAME"); v != "" {
		cfg.PowerDNS.ZoneName = v
	}
}

// applyEnvOverridesNodeAgent applies environment variable values to NodeAgentConfig.
// Environment variables take precedence over YAML values.
func applyEnvOverridesNodeAgent(cfg *NodeAgentConfig) {
	if v := os.Getenv("CONTROLLER_GRPC_ADDR"); v != "" {
		cfg.ControllerGRPCAddr = v
	}
	if v := os.Getenv("NODE_ID"); v != "" {
		cfg.NodeID = v
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
	if v := os.Getenv("TLS_CERT_FILE"); v != "" {
		cfg.TLSCertFile = v
	}
	if v := os.Getenv("TLS_KEY_FILE"); v != "" {
		cfg.TLSKeyFile = v
	}
	if v := os.Getenv("TLS_CA_FILE"); v != "" {
		cfg.TLSCAFile = v
	}
	if v := os.Getenv("CLOUDINIT_PATH"); v != "" {
		cfg.CloudInitPath = v
	}
	if v := os.Getenv("ISO_STORAGE_PATH"); v != "" {
		cfg.ISOStoragePath = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
}

// validateControllerConfig validates that all required fields are set.
func validateControllerConfig(cfg *ControllerConfig) error {
	var missing []string

	if cfg.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if cfg.NatsURL == "" {
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

	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}

	return nil
}