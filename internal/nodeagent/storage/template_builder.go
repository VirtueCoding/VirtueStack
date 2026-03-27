package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// templateBuildTimeout is the maximum time for the entire ISO build process.
const templateBuildTimeout = 45 * time.Minute

// TemplateBuilder builds VM templates from ISO images using virt-install
// with unattended installation configurations (preseed/kickstart/autoinstall).
type TemplateBuilder struct {
	logger *slog.Logger
}

// NewTemplateBuilder creates a new TemplateBuilder.
func NewTemplateBuilder(logger *slog.Logger) *TemplateBuilder {
	return &TemplateBuilder{
		logger: logger.With("component", "template-builder"),
	}
}

// BuildConfig holds the parameters for building a template from ISO.
type BuildConfig struct {
	TemplateName        string // Human-readable name for the template
	ISOPath             string // Path to the ISO file on disk
	OSFamily            string // OS family: debian, ubuntu, almalinux, rocky, centos
	OSVersion           string // OS version: 12, 24.04, 9
	DiskSizeGB          int    // Disk size in GB for the template
	MemoryMB            int    // RAM in MB for the installation VM
	VCPUs               int    // vCPUs for the installation VM
	RootPassword        string // Root password (empty = default)
	CustomInstallConfig string // Custom preseed/kickstart/autoinstall content
}

// BuildResult holds the output of a successful template build.
type BuildResult struct {
	DiskPath  string // Path to the built qcow2 disk image
	SizeBytes int64  // Virtual size of the disk in bytes
}

// Build performs an unattended OS installation from an ISO into a qcow2 disk.
// The build process runs virt-install with a generated preseed/kickstart/autoinstall
// config, waits up to 45 minutes for installation to complete, then runs virt-sysprep
// to generalize the image. The resulting disk can be imported into any storage backend.
func (b *TemplateBuilder) Build(ctx context.Context, cfg BuildConfig) (*BuildResult, error) {
	if _, err := os.Stat(cfg.ISOPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("ISO file not found: %s", cfg.ISOPath)
	}

	buildDir, err := os.MkdirTemp("", "vs-template-build-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp build directory: %w", err)
	}

	diskPath := filepath.Join(buildDir, "disk.qcow2")
	installCfgPath := filepath.Join(buildDir, "install.cfg")

	b.logger.Info("starting template build from ISO",
		"template_name", cfg.TemplateName,
		"iso_path", cfg.ISOPath,
		"os_family", cfg.OSFamily,
		"disk_size_gb", cfg.DiskSizeGB,
		"build_dir", buildDir)

	installCfg := cfg.CustomInstallConfig
	if installCfg == "" {
		installCfg, err = b.generateInstallConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("generating install config: %w", err)
		}
	}

	if err := os.WriteFile(installCfgPath, []byte(installCfg), 0600); err != nil {
		return nil, fmt.Errorf("writing install config: %w", err)
	}

	buildCtx, cancel := context.WithTimeout(ctx, templateBuildTimeout)
	defer cancel()

	args := b.buildVirtInstallArgs(cfg, diskPath, installCfgPath)

	b.logger.Info("running virt-install",
		"args", strings.Join(args, " "))

	cmd := exec.CommandContext(buildCtx, "virt-install", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("virt-install failed: %w\nstderr: %s", err, stderr.String())
	}

	// Restrict permissions on the built disk image to prevent unauthorized access.
	if err := os.Chmod(diskPath, 0600); err != nil {
		b.logger.Warn("failed to chmod built disk", "error", err)
	}

	b.logger.Info("virt-install completed, running sysprep")

	if err := b.sysprep(buildCtx, diskPath); err != nil {
		b.logger.Warn("sysprep failed (non-fatal)", "error", err)
	}

	info, err := os.Stat(diskPath)
	if err != nil {
		return nil, fmt.Errorf("stat built disk: %w", err)
	}

	virtSize, err := getQemuImgVirtualSize(buildCtx, diskPath)
	if err != nil {
		virtSize = info.Size()
	}

	b.logger.Info("template build completed",
		"template_name", cfg.TemplateName,
		"disk_path", diskPath,
		"size_bytes", virtSize)

	return &BuildResult{
		DiskPath:  diskPath,
		SizeBytes: virtSize,
	}, nil
}

// Cleanup removes the build directory and temporary files.
// Call this after the built disk has been imported into the target storage backend.
func (b *TemplateBuilder) Cleanup(buildDir string) {
	if buildDir != "" && strings.HasPrefix(buildDir, os.TempDir()) {
		if err := os.RemoveAll(buildDir); err != nil {
			b.logger.Warn("failed to clean build dir", "dir", buildDir, "error", err)
		}
	}
}

func (b *TemplateBuilder) buildVirtInstallArgs(cfg BuildConfig, diskPath, installCfgPath string) []string {
	domainName := fmt.Sprintf("vs-build-%s", SanitizeTemplateName(cfg.TemplateName))

	args := []string{
		"--name", domainName,
		"--ram", fmt.Sprintf("%d", cfg.MemoryMB),
		"--vcpus", fmt.Sprintf("%d", cfg.VCPUs),
		"--disk", fmt.Sprintf("path=%s,size=%d,format=qcow2,bus=virtio", diskPath, cfg.DiskSizeGB),
		"--cdrom", cfg.ISOPath,
		"--os-variant", b.osVariant(cfg.OSFamily, cfg.OSVersion),
		"--network", "none",
		"--graphics", "none",
		"--console", "pty,target_type=serial",
		"--noautoconsole",
		"--wait", "-1",
		"--noreboot",
	}

	switch cfg.OSFamily {
	case "debian":
		args = append(args, "--initrd-inject", installCfgPath)
		args = append(args, "--extra-args",
			"auto=true priority=critical preseed/file=/install.cfg console=ttyS0,115200n8")
	case "ubuntu":
		args = append(args, "--initrd-inject", installCfgPath)
		args = append(args, "--extra-args",
			"autoinstall console=ttyS0,115200n8")
	case "almalinux", "rocky", "centos":
		args = append(args, "--initrd-inject", installCfgPath)
		args = append(args, "--extra-args",
			fmt.Sprintf("inst.ks=file:/%s console=ttyS0,115200n8", filepath.Base(installCfgPath)))
	}

	return args
}

func (b *TemplateBuilder) osVariant(osFamily, osVersion string) string {
	switch osFamily {
	case "debian":
		return "debian" + osVersion
	case "ubuntu":
		return "ubuntu" + strings.ReplaceAll(osVersion, ".", "")
	case "almalinux":
		return "almalinux" + osVersion
	case "rocky":
		return "rocky" + osVersion
	case "centos":
		return "centos" + osVersion
	default:
		return "linux2022"
	}
}

func (b *TemplateBuilder) sysprep(ctx context.Context, diskPath string) error {
	cmd := exec.CommandContext(ctx, "virt-sysprep",
		"--add", diskPath,
		"--operations",
		"defaults,-ssh-userdir,-lvm-uuids",
	)
	return cmd.Run()
}

func (b *TemplateBuilder) generateInstallConfig(cfg BuildConfig) (string, error) {
	password := cfg.RootPassword
	if password == "" {
		password = "virtuestack"
	}

	switch cfg.OSFamily {
	case "debian":
		return renderInstallTemplate(debianPreseedTmpl, cfg, password)
	case "ubuntu":
		return renderInstallTemplate(ubuntuAutoinstallTmpl, cfg, password)
	case "almalinux", "rocky", "centos":
		return renderInstallTemplate(rhelKickstartTmpl, cfg, password)
	default:
		return "", fmt.Errorf("unsupported OS family: %s", cfg.OSFamily)
	}
}

func renderInstallTemplate(tmplStr string, cfg BuildConfig, password string) (string, error) {
	tmpl, err := template.New("install").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parsing install template: %w", err)
	}

	data := map[string]interface{}{
		"RootPassword": password,
		"Hostname":     SanitizeTemplateName(cfg.TemplateName),
		"OSFamily":     cfg.OSFamily,
		"OSVersion":    cfg.OSVersion,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering install template: %w", err)
	}
	return buf.String(), nil
}

func getQemuImgVirtualSize(ctx context.Context, path string) (int64, error) {
	cmd := exec.CommandContext(ctx, "qemu-img", "info", "--output=json", path)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	var result struct {
		VirtualSize int64 `json:"virtual-size"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, err
	}
	return result.VirtualSize, nil
}

// SanitizeTemplateName cleans a template name for use in storage references.
// It lowercases the name, replaces spaces/underscores with hyphens, removes
// other non-alphanumeric characters, and truncates to 50 characters.
func SanitizeTemplateName(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteRune(c)
		} else if c == ' ' || c == '_' {
			b.WriteRune('-')
		}
	}
	result := b.String()
	if len(result) > 50 {
		result = result[:50]
	}
	return result
}

// ============================================================================
// Built-in Installation Configurations
// ============================================================================

// debianPreseedTmpl is the preseed template for Debian unattended installation.
const debianPreseedTmpl = `# Debian Preseed Configuration - Generated by VirtueStack
# Locale and keyboard
d-i debian-installer/locale string en_US.UTF-8
d-i keyboard-configuration/xkb-keymap select us
d-i console-setup/ask_detect boolean false

# Network (disabled during install, configured by cloud-init later)
d-i netcfg/enable boolean false
d-i netcfg/choose_interface select auto

# Mirror
d-i mirror/country string manual
d-i mirror/http/hostname string deb.debian.org
d-i mirror/http/directory string /debian
d-i mirror/http/proxy string

# Clock
d-i clock-setup/utc boolean true
d-i time/zone string UTC
d-i clock-setup/ntp boolean true

# Partitioning - single ext4 partition for template
d-i partman-auto/method string regular
d-i partman-auto/choose_recipe select atomic
d-i partman-partitioning/confirm_write_new_label boolean true
d-i partman/choose_partition select finish
d-i partman/confirm boolean true
d-i partman/confirm_nooverwrite boolean true

# Root password
d-i passwd/root-login boolean true
d-i passwd/root-password password {{.RootPassword}}
d-i passwd/root-password-again password {{.RootPassword}}
d-i passwd/make-user boolean false

# Packages
tasksel tasksel/first multiselect standard, ssh-server
d-i pkgsel/include string cloud-init qemu-guest-agent openssh-server curl
d-i pkgsel/upgrade select full-upgrade

# GRUB
d-i grub-installer/only_debian boolean true
d-i grub-installer/with_other_os boolean false
d-i grub-installer/bootdev string default

# Serial console for VirtueStack
d-i debian-installer/add-kernel-opts string console=tty0 console=ttyS0,115200n8

# Finish
d-i finish-install/reboot_in_progress note
d-i debian-installer/exit/poweroff boolean true
`

// ubuntuAutoinstallTmpl is the autoinstall template for Ubuntu.
const ubuntuAutoinstallTmpl = `#cloud-config
autoinstall:
  version: 1
  locale: en_US.UTF-8
  keyboard:
    layout: us
  identity:
    hostname: {{.Hostname}}
    password: "{{.RootPassword}}"
    username: root
  ssh:
    install-server: true
    allow-pw: true
  storage:
    layout:
      name: direct
  packages:
    - cloud-init
    - qemu-guest-agent
    - openssh-server
    - curl
  late-commands:
    - echo 'PermitRootLogin yes' >> /target/etc/ssh/sshd_config
    - curtin in-target --target=/target -- systemctl enable qemu-guest-agent
    - curtin in-target --target=/target -- systemctl enable cloud-init
    - 'sed -i "s/GRUB_CMDLINE_LINUX_DEFAULT=.*/GRUB_CMDLINE_LINUX_DEFAULT=\"console=tty0 console=ttyS0,115200n8\"/" /target/etc/default/grub'
    - curtin in-target --target=/target -- update-grub
  shutdown: poweroff
`

// rhelKickstartTmpl is the kickstart template for AlmaLinux/Rocky/CentOS.
const rhelKickstartTmpl = `# Kickstart Configuration - Generated by VirtueStack
# System language and keyboard
lang en_US.UTF-8
keyboard us
timezone UTC --utc

# Root password
rootpw --plaintext {{.RootPassword}}

# Network (disabled, configured by cloud-init)
network --bootproto=dhcp --activate

# System bootloader
bootloader --append="console=tty0 console=ttyS0,115200n8" --location=mbr

# Partition clearing
clearpart --all --initlabel
autopart --type=plain --nohome

# Packages
%packages
@^minimal-environment
cloud-init
qemu-guest-agent
openssh-server
curl
%end

# Services
services --enabled=sshd,qemu-guest-agent,cloud-init

# SELinux
selinux --enforcing

# Firewall
firewall --enabled --ssh

# Post-install
%post
# Enable serial console
systemctl enable serial-getty@ttyS0.service
# Enable cloud-init
systemctl enable cloud-init
# Enable guest agent
systemctl enable qemu-guest-agent
%end

# Power off after install
poweroff
`
