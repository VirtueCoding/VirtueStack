package storage

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/crypto/ssh"
)

// passwordHashRe matches common Unix password hash formats:
// SHA-512 ($6$...), SHA-256 ($5$...), bcrypt ($2b$...), Argon2id ($argon2id$...).
var passwordHashRe = regexp.MustCompile(`^\$[0-9a-z]+\$.+`)

// validatePasswordHash validates that the given string looks like a Unix password hash.
func validatePasswordHash(hash string) error {
	if hash == "" {
		return nil
	}
	if !passwordHashRe.MatchString(hash) {
		return fmt.Errorf("root_password_hash does not appear to be a valid Unix password hash (must start with $id$)")
	}
	return nil
}

// cloudInitOutputDir is the default output directory for cloud-init ISO files.
const cloudInitOutputDir = "/var/lib/virtuestack/cloud-init"

// CloudInitConfig holds all parameters for generating a cloud-init NoCloud ISO.
type CloudInitConfig struct {
	// VMID is the unique identifier of the virtual machine.
	VMID string
	// Hostname is the desired hostname for the VM.
	Hostname string
	// RootPasswordHash is the pre-hashed root password (Argon2id or SHA-512).
	RootPasswordHash string
	// SSHPublicKeys is the list of SSH public keys to authorize for root login.
	SSHPublicKeys []string
	// IPv4Address is the static IPv4 address with prefix length (e.g., "192.0.2.50/24").
	IPv4Address string
	// IPv4Gateway is the IPv4 default gateway address.
	IPv4Gateway string
	// IPv6Address is the static IPv6 address with prefix length (e.g., "2001:db8::/64").
	IPv6Address string
	// IPv6Gateway is the IPv6 default gateway address.
	IPv6Gateway string
	// Nameservers is the list of DNS resolver addresses.
	Nameservers []string
}

// CloudInitGenerator generates cloud-init NoCloud ISOs for VM provisioning.
type CloudInitGenerator struct {
	outputPath string // e.g., "/var/lib/virtuestack/cloud-init"
	logger     *slog.Logger
}

// NewCloudInitGenerator creates a new CloudInitGenerator.
// outputPath is the directory where generated ISO files will be stored.
func NewCloudInitGenerator(outputPath string, logger *slog.Logger) *CloudInitGenerator {
	if outputPath == "" {
		outputPath = cloudInitOutputDir
	}
	return &CloudInitGenerator{
		outputPath: outputPath,
		logger:     logger.With("component", "cloudinit-generator"),
	}
}

// Generate creates a cloud-init NoCloud ISO for the specified VM.
// The ISO contains meta-data, user-data, and network-config files.
// Returns the path to the generated ISO file.
func (g *CloudInitGenerator) Generate(ctx context.Context, cfg *CloudInitConfig) (string, error) {
	logger := g.logger.With("vm_id", cfg.VMID, "hostname", cfg.Hostname)
	logger.Info("generating cloud-init ISO")

	if err := validateCloudInitHostname(cfg.Hostname); err != nil {
		return "", fmt.Errorf("invalid hostname for VM %s: %w", cfg.VMID, err)
	}

	if err := os.MkdirAll(g.outputPath, 0755); err != nil {
		return "", fmt.Errorf("creating cloud-init output dir %s: %w", g.outputPath, err)
	}

	tmpDir, err := os.MkdirTemp("", "cloud-init-"+cfg.VMID+"-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir for cloud-init %s: %w", cfg.VMID, err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			logger.Warn("failed to remove temp dir for cloud-init", "path", tmpDir, "error", err)
		}
	}()

	if err := g.writeMetaData(tmpDir, cfg); err != nil {
		return "", fmt.Errorf("writing meta-data for VM %s: %w", cfg.VMID, err)
	}
	if err := g.writeUserData(tmpDir, cfg); err != nil {
		return "", fmt.Errorf("writing user-data for VM %s: %w", cfg.VMID, err)
	}
	if err := g.writeNetworkConfig(tmpDir, cfg); err != nil {
		return "", fmt.Errorf("writing network-config for VM %s: %w", cfg.VMID, err)
	}

	isoPath := filepath.Join(g.outputPath, fmt.Sprintf("vs-%s-seed.iso", cfg.VMID))
	if err := g.buildISO(ctx, tmpDir, isoPath); err != nil {
		if rmErr := os.Remove(isoPath); rmErr != nil && !os.IsNotExist(rmErr) {
			logger.Warn("failed to remove incomplete ISO file", "path", isoPath, "error", rmErr)
		}
		return "", fmt.Errorf("building cloud-init ISO for VM %s: %w", cfg.VMID, err)
	}

	logger.Info("cloud-init ISO generated", "path", isoPath)
	return isoPath, nil
}

func validateCloudInitHostname(hostname string) error {
	if strings.TrimSpace(hostname) == "" {
		return fmt.Errorf("hostname must not be empty")
	}
	if strings.ContainsAny(hostname, "\r\n") {
		return fmt.Errorf("hostname must not contain line breaks")
	}
	if strings.Contains(hostname, ":") {
		return fmt.Errorf("hostname must not contain colon characters")
	}

	return nil
}

// Delete removes the cloud-init ISO file for the given VM ID.
func (g *CloudInitGenerator) Delete(vmID string) error {
	isoPath := filepath.Join(g.outputPath, fmt.Sprintf("vs-%s-seed.iso", vmID))
	if err := os.Remove(isoPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting cloud-init ISO for VM %s: %w", vmID, err)
	}
	g.logger.Info("cloud-init ISO deleted", "vm_id", vmID, "path", isoPath)
	return nil
}

// writeMetaData writes the cloud-init meta-data file to tmpDir.
func (g *CloudInitGenerator) writeMetaData(tmpDir string, cfg *CloudInitConfig) error {
	content := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", cfg.VMID, cfg.Hostname)
	return os.WriteFile(filepath.Join(tmpDir, "meta-data"), []byte(content), 0644)
}

// writeUserData writes the cloud-init user-data (cloud-config) file to tmpDir.
func (g *CloudInitGenerator) writeUserData(tmpDir string, cfg *CloudInitConfig) error {
	// Validate the root password hash format before embedding it in YAML
	if err := validatePasswordHash(cfg.RootPasswordHash); err != nil {
		return fmt.Errorf("invalid root_password_hash for VM %s: %w", cfg.VMID, err)
	}

	var sb strings.Builder
	sb.WriteString("#cloud-config\n")
	sb.WriteString(fmt.Sprintf("hostname: %s\n", cfg.Hostname))
	sb.WriteString("manage_etc_hosts: true\n")
	sb.WriteString("chpasswd:\n")
	sb.WriteString("  expire: false\n")
	sb.WriteString("  users:\n")
	sb.WriteString("    - name: root\n")
	// Quote the hash value to prevent YAML injection via special characters
	sb.WriteString(fmt.Sprintf("      password: %q\n", cfg.RootPasswordHash))
	sb.WriteString("      type: HASH\n")

	if len(cfg.SSHPublicKeys) > 0 {
		sb.WriteString("ssh_authorized_keys:\n")
		for _, key := range cfg.SSHPublicKeys {
			canonicalKey, err := normalizeAuthorizedKey(key)
			if err != nil {
				return fmt.Errorf("invalid SSH public key: %w", err)
			}
			sb.WriteString(fmt.Sprintf("  - %q\n", canonicalKey))
		}
	}

	sb.WriteString("disable_root: false\n")
	sb.WriteString("ssh_pwauth: true\n")
	sb.WriteString("package_update: false\n")
	sb.WriteString("timezone: UTC\n")

	return os.WriteFile(filepath.Join(tmpDir, "user-data"), []byte(sb.String()), 0644)
}

func normalizeAuthorizedKey(key string) (string, error) {
	_, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(key))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(rest)) != "" {
		return "", fmt.Errorf("authorized key contains trailing content")
	}

	return strings.TrimSpace(key), nil
}

// validateCIDRHost validates that s is a valid CIDR address (IP/prefix) by checking
// the host part with net.ParseIP after stripping the prefix length.
func validateCIDRHost(s string) error {
	if s == "" {
		return nil
	}
	// s may be "192.0.2.1/24" or "2001:db8::1/64"
	ip, _, err := net.ParseCIDR(s)
	if err != nil || ip == nil {
		return fmt.Errorf("invalid CIDR address %q", s)
	}
	return nil
}

// writeNetworkConfig writes the Netplan v2 network-config file to tmpDir.
func (g *CloudInitGenerator) writeNetworkConfig(tmpDir string, cfg *CloudInitConfig) error {
	// Validate all IP addresses before embedding them in YAML
	if err := validateCIDRHost(cfg.IPv4Address); err != nil {
		return fmt.Errorf("invalid IPv4 address for VM %s: %w", cfg.VMID, err)
	}
	if err := validateCIDRHost(cfg.IPv6Address); err != nil {
		return fmt.Errorf("invalid IPv6 address for VM %s: %w", cfg.VMID, err)
	}
	if cfg.IPv4Gateway != "" && net.ParseIP(cfg.IPv4Gateway) == nil {
		return fmt.Errorf("invalid IPv4 gateway %q for VM %s", cfg.IPv4Gateway, cfg.VMID)
	}
	if cfg.IPv6Gateway != "" && net.ParseIP(cfg.IPv6Gateway) == nil {
		return fmt.Errorf("invalid IPv6 gateway %q for VM %s", cfg.IPv6Gateway, cfg.VMID)
	}
	for _, ns := range cfg.Nameservers {
		if net.ParseIP(ns) == nil {
			return fmt.Errorf("invalid nameserver %q for VM %s", ns, cfg.VMID)
		}
	}

	var sb strings.Builder
	sb.WriteString("network:\n")
	sb.WriteString("  version: 2\n")
	sb.WriteString("  renderer: networkd\n")
	sb.WriteString("  ethernets:\n")
	sb.WriteString("    ens3:\n")
	sb.WriteString("      addresses:\n")
	if cfg.IPv4Address != "" {
		sb.WriteString(fmt.Sprintf("        - %q\n", cfg.IPv4Address))
	}
	if cfg.IPv6Address != "" {
		sb.WriteString(fmt.Sprintf("        - %q\n", cfg.IPv6Address))
	}
	sb.WriteString("      routes:\n")
	if cfg.IPv4Gateway != "" {
		sb.WriteString("        - to: 0.0.0.0/0\n")
		sb.WriteString(fmt.Sprintf("          via: %q\n", cfg.IPv4Gateway))
	}
	if cfg.IPv6Gateway != "" {
		sb.WriteString("        - to: \"::/0\"\n")
		sb.WriteString(fmt.Sprintf("          via: %q\n", cfg.IPv6Gateway))
	}
	if len(cfg.Nameservers) > 0 {
		sb.WriteString("      nameservers:\n")
		sb.WriteString("        addresses:\n")
		for _, ns := range cfg.Nameservers {
			sb.WriteString(fmt.Sprintf("          - %q\n", ns))
		}
	}

	return os.WriteFile(filepath.Join(tmpDir, "network-config"), []byte(sb.String()), 0644)
}

// buildISO invokes genisoimage to create the NoCloud ISO from the temp directory.
func (g *CloudInitGenerator) buildISO(ctx context.Context, tmpDir, isoPath string) error {
	cmd := exec.CommandContext(ctx,
		"genisoimage",
		"-output", isoPath,
		"-volid", "cidata",
		"-joliet",
		"-rock",
		tmpDir,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("genisoimage failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	return nil
}
