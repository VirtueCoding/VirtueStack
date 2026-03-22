package storage

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
	var sb strings.Builder
	sb.WriteString("#cloud-config\n")
	sb.WriteString(fmt.Sprintf("hostname: %s\n", cfg.Hostname))
	sb.WriteString("manage_etc_hosts: true\n")
	sb.WriteString("chpasswd:\n")
	sb.WriteString("  expire: false\n")
	sb.WriteString("  users:\n")
	sb.WriteString("    - name: root\n")
	sb.WriteString(fmt.Sprintf("      password: %s\n", cfg.RootPasswordHash))
	sb.WriteString("      type: HASH\n")

	if len(cfg.SSHPublicKeys) > 0 {
		sb.WriteString("ssh_authorized_keys:\n")
		for _, key := range cfg.SSHPublicKeys {
			sb.WriteString(fmt.Sprintf("  - %s\n", key))
		}
	}

	sb.WriteString("disable_root: false\n")
	sb.WriteString("ssh_pwauth: true\n")
	sb.WriteString("package_update: false\n")
	sb.WriteString("timezone: UTC\n")

	return os.WriteFile(filepath.Join(tmpDir, "user-data"), []byte(sb.String()), 0644)
}

// writeNetworkConfig writes the Netplan v2 network-config file to tmpDir.
func (g *CloudInitGenerator) writeNetworkConfig(tmpDir string, cfg *CloudInitConfig) error {
	var sb strings.Builder
	sb.WriteString("network:\n")
	sb.WriteString("  version: 2\n")
	sb.WriteString("  renderer: networkd\n")
	sb.WriteString("  ethernets:\n")
	sb.WriteString("    ens3:\n")
	sb.WriteString("      addresses:\n")
	if cfg.IPv4Address != "" {
		sb.WriteString(fmt.Sprintf("        - %s\n", cfg.IPv4Address))
	}
	if cfg.IPv6Address != "" {
		sb.WriteString(fmt.Sprintf("        - \"%s\"\n", cfg.IPv6Address))
	}
	sb.WriteString("      routes:\n")
	if cfg.IPv4Gateway != "" {
		sb.WriteString("        - to: 0.0.0.0/0\n")
		sb.WriteString(fmt.Sprintf("          via: %s\n", cfg.IPv4Gateway))
	}
	if cfg.IPv6Gateway != "" {
		sb.WriteString("        - to: \"::/0\"\n")
		sb.WriteString(fmt.Sprintf("          via: \"%s\"\n", cfg.IPv6Gateway))
	}
	if len(cfg.Nameservers) > 0 {
		sb.WriteString("      nameservers:\n")
		sb.WriteString("        addresses:\n")
		for _, ns := range cfg.Nameservers {
			sb.WriteString(fmt.Sprintf("          - %s\n", ns))
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
