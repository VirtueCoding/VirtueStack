// Package network provides network management for the VirtueStack Node Agent.
// This file implements DHCP server management using dnsmasq for VMs.
package network

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// validBridgeIfaceRegex validates Linux bridge/tap interface names.
// Interface names must be 1-15 characters, starting with a letter, and
// containing only alphanumeric characters, underscores, or hyphens.
// This prevents newline injection into dnsmasq configuration files.
var validBridgeIfaceRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,14}$`)

// Constants for DHCP management.
const (
	// DefaultBridgeInterface is the default bridge interface for VMs.
	DefaultBridgeInterface = "vs-br0"
	// DefaultDNS is the default DNS server for DHCP clients.
	DefaultDNS = "8.8.8.8"
	// DHCPConfigSuffix is the suffix for DHCP config files.
	DHCPConfigSuffix = ".conf"
	// DHCPLeaseSuffix is the suffix for DHCP lease files.
	DHCPLeaseSuffix = ".lease"
	// DHCPPIDSuffix is the suffix for DHCP PID files.
	DHCPPIDSuffix = ".pid"
	// DHCPLogSuffix is the suffix for DHCP log files.
	DHCPLogSuffix = ".log"
	// DHCPStatusFileSuffix is the suffix for DHCP status files.
	DHCPStatusFileSuffix = ".status"
)

// DHCPManager manages dnsmasq-based DHCP servers for VMs.
// Each VM gets its own dnsmasq instance with a static IP lease.
// This provides per-VM DHCP isolation and prevents broadcast domain issues.
type DHCPManager struct {
	configDir    string // /var/lib/virtuestack/dhcp
	leaseDir     string // /var/lib/virtuestack/dhcp/leases
	pidDir       string // /var/lib/virtuestack/dhcp/pid
	logDir       string // /var/lib/virtuestack/dhcp/logs
	statusDir    string // /var/lib/virtuestack/dhcp/status
	logger       *slog.Logger
	mu           sync.RWMutex
	runningProcs map[string]*dnsmasqProcess // vmID -> process info
	wg           sync.WaitGroup
}

// dnsmasqProcess tracks a running dnsmasq instance.
type dnsmasqProcess struct {
	cmd       *exec.Cmd
	startTime time.Time
	pid       int
}

// DHCPLease represents a DHCP lease for a VM.
type DHCPLease struct {
	VMID       string    `json:"vm_id"`
	VMName     string    `json:"vm_name"`
	MACAddress string    `json:"mac_address"`
	IPAddress  string    `json:"ip_address"`
	Gateway    string    `json:"gateway"`
	DNS        string    `json:"dns"`
	StartedAt  time.Time `json:"started_at"`
	PID        int       `json:"pid"`
	Status     string    `json:"status"` // "running", "stopped"
}

// DHCPConfig contains configuration for starting a DHCP server for a VM.
type DHCPConfig struct {
	VMID            string
	VMName          string
	MACAddress      string
	IPAddress       string
	Gateway         string
	DNS             string
	BridgeInterface string
}

// NewDHCPManager creates a new DHCPManager with the given directories.
func NewDHCPManager(configDir, leaseDir, pidDir string, logger *slog.Logger) *DHCPManager {
	return &DHCPManager{
		configDir:    configDir,
		leaseDir:     leaseDir,
		pidDir:       pidDir,
		logDir:       filepath.Join(configDir, "logs"),
		statusDir:    filepath.Join(configDir, "status"),
		logger:       logger.With("component", "dhcp-manager"),
		runningProcs: make(map[string]*dnsmasqProcess),
	}
}

// Stop waits for all background goroutines to complete.
func (m *DHCPManager) Stop() {
	m.wg.Wait()
}

// Initialize creates the necessary directories for DHCP management.
func (m *DHCPManager) Initialize(ctx context.Context) error {
	logger := m.logger.With("operation", "initialize")
	logger.Info("initializing DHCP manager")

	dirs := []string{
		m.configDir,
		m.leaseDir,
		m.pidDir,
		m.logDir,
		m.statusDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	logger.Info("DHCP manager initialized successfully")
	return nil
}

// StartDHCPForVM starts a dnsmasq instance for a specific VM.
// It generates the configuration, writes the lease file, and starts the dnsmasq process.
// Returns an error if DHCP is already running for this VM.
func (m *DHCPManager) StartDHCPForVM(ctx context.Context, vmID, vmName, macAddress, ipAddress, gateway string) error {
	return m.StartDHCPForVMWithConfig(ctx, DHCPConfig{
		VMID:            vmID,
		VMName:          vmName,
		MACAddress:      macAddress,
		IPAddress:       ipAddress,
		Gateway:         gateway,
		DNS:             DefaultDNS,
		BridgeInterface: DefaultBridgeInterface,
	})
}

// StartDHCPForVMWithConfig starts a dnsmasq instance with full configuration.
func (m *DHCPManager) StartDHCPForVMWithConfig(ctx context.Context, cfg DHCPConfig) error {
	logger := m.logger.With("vm_id", cfg.VMID, "vm_name", cfg.VMName, "operation", "start_dhcp")
	logger.Info("starting DHCP for VM", "mac", cfg.MACAddress, "ip", cfg.IPAddress, "gateway", cfg.Gateway)

	// Check if dnsmasq is available
	if err := m.checkDNSMasqAvailable(ctx); err != nil {
		return fmt.Errorf("dnsmasq not available: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already running
	if _, err := m.checkExistingDHCPProcess(cfg.VMID, logger); err != nil {
		return err
	}

	// Set defaults
	if cfg.DNS == "" {
		cfg.DNS = DefaultDNS
	}
	if cfg.BridgeInterface == "" {
		cfg.BridgeInterface = DefaultBridgeInterface
	}

	// Write config files
	files, err := m.writeDHCPConfigFiles(cfg)
	if err != nil {
		return err
	}

	// Start dnsmasq process
	cmd, logFile, pid, err := m.startDNSMasqProcess(ctx, files, logger)
	if err != nil {
		return err
	}

	// Track process and save status
	m.trackDHCPProcess(cfg, cmd, pid, logger)

	// Start process monitor
	m.startProcessMonitor(ctx, cfg, cmd, logFile)

	return nil
}

// StopDHCPForVM stops the dnsmasq instance for a specific VM.
// It gracefully terminates the process and cleans up config and lease files.
func (m *DHCPManager) StopDHCPForVM(ctx context.Context, vmID string) error {
	logger := m.logger.With("vm_id", vmID, "operation", "stop_dhcp")
	logger.Info("stopping DHCP for VM")

	m.mu.Lock()
	defer m.mu.Unlock()

	// Get the PID
	pid := m.getPID(vmID)
	if pid == 0 {
		logger.Info("DHCP not running for VM")
		return nil // Not running, nothing to stop
	}

	// Send SIGTERM for graceful shutdown
	if err := m.sendSignal(pid, syscall.SIGTERM); err != nil {
		logger.Warn("failed to send SIGTERM, trying SIGKILL", "error", err)
		// Force kill if SIGTERM fails
		if err := m.sendSignal(pid, syscall.SIGKILL); err != nil {
			logger.Warn("failed to send SIGKILL", "error", err)
		}
	}

	// Wait for process to exit (with timeout)
	m.waitForProcessExit(pid, 10*time.Second)

	// Clean up files
	m.cleanupVMFiles(vmID)

	// Remove from tracking
	delete(m.runningProcs, vmID)

	logger.Info("DHCP stopped successfully")
	return nil
}

// RestartDHCPForVM restarts the dnsmasq instance for a specific VM.
// It stops the existing instance and starts a new one with the same configuration.
func (m *DHCPManager) RestartDHCPForVM(ctx context.Context, vmID string) error {
	logger := m.logger.With("vm_id", vmID, "operation", "restart_dhcp")
	logger.Info("restarting DHCP for VM")

	// Get current lease info
	lease, err := m.GetVMLease(ctx, vmID)
	if err != nil {
		return fmt.Errorf("getting current DHCP lease: %w", err)
	}

	if lease == nil {
		return fmt.Errorf("no DHCP lease found for VM %s: %w", vmID, errors.ErrNotFound)
	}

	// Stop existing
	if err := m.StopDHCPForVM(ctx, vmID); err != nil {
		logger.Warn("error stopping DHCP during restart", "error", err)
	}

	// Poll until the previous dnsmasq process has fully exited (or the caller's
	// context expires). This is more reliable than a fixed sleep because process
	// teardown speed varies with system load.
	const restartPollInterval = 50 * time.Millisecond
	const restartMaxWait = 3 * time.Second
	deadline := time.Now().Add(restartMaxWait)
	for time.Now().Before(deadline) {
		if pid := m.getPID(vmID); pid == 0 || !m.isProcessRunning(pid) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(restartPollInterval):
		}
	}

	// Start new with same config
	return m.StartDHCPForVM(ctx, vmID, lease.VMName, lease.MACAddress, lease.IPAddress, lease.Gateway)
}

// GetVMLease retrieves the current DHCP lease status for a VM.
func (m *DHCPManager) GetVMLease(ctx context.Context, vmID string) (*DHCPLease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.getVMLeaseUnlocked(vmID)
}

// getVMLeaseUnlocked retrieves the current DHCP lease status for a VM
// without acquiring any lock. The caller must hold at least a read lock.
func (m *DHCPManager) getVMLeaseUnlocked(vmID string) (*DHCPLease, error) {
	statusPath := m.statusFilePath(vmID)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No lease found
		}
		return nil, fmt.Errorf("reading lease status: %w", err)
	}

	var lease DHCPLease
	if err := json.Unmarshal(data, &lease); err != nil {
		return nil, fmt.Errorf("parsing lease status: %w", err)
	}

	// Update status based on process state
	pid := m.getPID(vmID)
	if pid > 0 && m.isProcessRunning(pid) {
		lease.Status = "running"
	} else {
		lease.Status = "stopped"
	}

	return &lease, nil
}

// ListActiveDHCP lists all active dnsmasq instances.
func (m *DHCPManager) ListActiveDHCP(ctx context.Context) ([]DHCPLease, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var leases []DHCPLease

	// Read all status files
	entries, err := os.ReadDir(m.statusDir)
	if err != nil {
		if os.IsNotExist(err) {
			return leases, nil
		}
		return nil, fmt.Errorf("reading status directory: %w", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), DHCPStatusFileSuffix) {
			continue
		}

		vmID := strings.TrimSuffix(entry.Name(), DHCPStatusFileSuffix)
		lease, err := m.getVMLeaseUnlocked(vmID)
		if err != nil {
			m.logger.Warn("failed to get lease", "vm_id", vmID, "error", err)
			continue
		}

		if lease != nil && lease.Status == "running" {
			leases = append(leases, *lease)
		}
	}

	return leases, nil
}

// GenerateDNSMasqConfig generates the dnsmasq configuration content for a VM.
// The configuration sets up a static DHCP lease for the VM's MAC address.
// Returns an error if any network address or interface name is invalid.
func (m *DHCPManager) GenerateDNSMasqConfig(vmName, mac, ip, gateway string) (string, error) {
	return m.GenerateDNSMasqConfigWithDNS(vmName, mac, ip, gateway, DefaultDNS)
}

// GenerateDNSMasqConfigWithDNS generates dnsmasq config with custom DNS server.
// Returns an error if any network address or interface name is invalid.
func (m *DHCPManager) GenerateDNSMasqConfigWithDNS(vmName, mac, ip, gateway, dns string) (string, error) {
	return m.GenerateDNSMasqConfigFull(DHCPConfig{
		VMName:          vmName,
		MACAddress:      mac,
		IPAddress:       ip,
		Gateway:         gateway,
		DNS:             dns,
		BridgeInterface: DefaultBridgeInterface,
	})
}

// GenerateDNSMasqConfigFull generates the full dnsmasq configuration for a VM.
// It validates all network addresses with net.ParseIP and the bridge interface
// name against a strict regex to prevent newline injection into the config file.
func (m *DHCPManager) GenerateDNSMasqConfigFull(cfg DHCPConfig) (string, error) {
	// Validate bridge interface name to prevent newline/directive injection.
	if !validBridgeIfaceRegex.MatchString(cfg.BridgeInterface) {
		return "", fmt.Errorf("invalid bridge interface name %q: must match %s",
			cfg.BridgeInterface, validBridgeIfaceRegex.String())
	}

	// Validate IP addresses.
	if net.ParseIP(cfg.IPAddress) == nil {
		return "", fmt.Errorf("invalid IP address %q for VM %s", cfg.IPAddress, cfg.VMID)
	}
	if net.ParseIP(cfg.Gateway) == nil {
		return "", fmt.Errorf("invalid gateway address %q for VM %s", cfg.Gateway, cfg.VMID)
	}
	if net.ParseIP(cfg.DNS) == nil {
		return "", fmt.Errorf("invalid DNS address %q for VM %s", cfg.DNS, cfg.VMID)
	}

	var sb strings.Builder

	// Header comment
	sb.WriteString(fmt.Sprintf("# dnsmasq.conf for VM: %s\n", cfg.VMName))
	sb.WriteString(fmt.Sprintf("# Generated by VirtueStack DHCP Manager\n"))
	sb.WriteString(fmt.Sprintf("# VM ID: %s\n\n", cfg.VMID))

	// Interface binding
	sb.WriteString(fmt.Sprintf("interface=%s\n", cfg.BridgeInterface))
	sb.WriteString("bind-interfaces\n")
	sb.WriteString("except-interface=lo\n\n")

	// DHCP configuration - single IP range for static lease
	sb.WriteString(fmt.Sprintf("# DHCP range (single IP for static lease)\n"))
	sb.WriteString(fmt.Sprintf("dhcp-range=%s,%s,255.255.255.0,12h\n", cfg.IPAddress, cfg.IPAddress))
	sb.WriteString(fmt.Sprintf("dhcp-host=%s,%s,%s,infinite\n\n", cfg.MACAddress, cfg.IPAddress, sanitizeHostname(cfg.VMName)))

	// Gateway and DNS options
	sb.WriteString(fmt.Sprintf("# Gateway and DNS\n"))
	sb.WriteString(fmt.Sprintf("dhcp-option=3,%s\n", cfg.Gateway))
	sb.WriteString(fmt.Sprintf("dhcp-option=6,%s\n\n", cfg.DNS))

	// Lease file configuration
	leaseFile := m.leaseFilePath(cfg.VMID)
	sb.WriteString(fmt.Sprintf("# Lease file\n"))
	sb.WriteString("leasefile-ro\n")
	sb.WriteString(fmt.Sprintf("dhcp-leasefile=%s\n\n", leaseFile))

	// Logging
	logFile := m.logFilePath(cfg.VMID)
	sb.WriteString(fmt.Sprintf("# Logging\n"))
	sb.WriteString(fmt.Sprintf("log-facility=%s\n", logFile))
	sb.WriteString("log-dhcp\n\n")

	// Run as non-root for security
	sb.WriteString(fmt.Sprintf("# Run as non-root\n"))
	sb.WriteString("user=virtuestack\n")
	sb.WriteString("group=virtuestack\n\n")

	// PID file
	pidFile := m.pidFilePath(cfg.VMID)
	sb.WriteString(fmt.Sprintf("# PID file\n"))
	sb.WriteString(fmt.Sprintf("pid-file=%s\n", pidFile))

	return sb.String(), nil
}

// CheckDNSMasqInstalled checks if dnsmasq is installed and available.
func (m *DHCPManager) CheckDNSMasqInstalled(ctx context.Context) error {
	return m.checkDNSMasqAvailable(ctx)
}

// CleanupStaleProcesses cleans up any stale dnsmasq processes.
// This should be called during node agent startup.
func (m *DHCPManager) CleanupStaleProcesses(ctx context.Context) error {
	logger := m.logger.With("operation", "cleanup_stale")
	logger.Info("cleaning up stale DHCP processes")

	// Read all PID files
	entries, err := os.ReadDir(m.pidDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading PID directory: %w", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), DHCPPIDSuffix) {
			continue
		}

		vmID := strings.TrimSuffix(entry.Name(), DHCPPIDSuffix)
		pidPath := filepath.Join(m.pidDir, entry.Name())

		pidData, err := os.ReadFile(pidPath)
		if err != nil {
			continue
		}

		pid, _ := strconv.Atoi(string(pidData))
		if pid > 0 && m.isProcessRunning(pid) {
			logger.Info("killing stale dnsmasq process", "vm_id", vmID, "pid", pid)
			if err := m.sendSignal(pid, syscall.SIGTERM); err != nil {
			logger.Warn("failed to send SIGTERM", "error", err)
		}
			m.waitForProcessExit(pid, 5*time.Second)
		}

		// Clean up files
		m.cleanupVMFiles(vmID)
	}

	return nil
}

// Helper methods

// configFilePath returns the path to the config file for a VM.
func (m *DHCPManager) configFilePath(vmID string) string {
	return filepath.Join(m.configDir, vmID+DHCPConfigSuffix)
}

// leaseFilePath returns the path to the lease file for a VM.
func (m *DHCPManager) leaseFilePath(vmID string) string {
	return filepath.Join(m.leaseDir, vmID+DHCPLeaseSuffix)
}

// pidFilePath returns the path to the PID file for a VM.
func (m *DHCPManager) pidFilePath(vmID string) string {
	return filepath.Join(m.pidDir, vmID+DHCPPIDSuffix)
}

// logFilePath returns the path to the log file for a VM.
func (m *DHCPManager) logFilePath(vmID string) string {
	return filepath.Join(m.logDir, vmID+DHCPLogSuffix)
}

// statusFilePath returns the path to the status file for a VM.
func (m *DHCPManager) statusFilePath(vmID string) string {
	return filepath.Join(m.statusDir, vmID+DHCPStatusFileSuffix)
}

// checkDNSMasqAvailable checks if dnsmasq is installed.
func (m *DHCPManager) checkDNSMasqAvailable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "dnsmasq", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dnsmasq not found or not executable: %w", err)
	}
	return nil
}

// testDNSMasqConfig tests a dnsmasq configuration file for validity.
func (m *DHCPManager) testDNSMasqConfig(ctx context.Context, configPath string) error {
	cmd := exec.CommandContext(ctx, "dnsmasq", "-C", configPath, "--test")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("config test failed: %w, output: %s", err, string(output))
	}
	return nil
}

// isProcessRunning checks if a process with the given PID is running.
func (m *DHCPManager) isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to check if running.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// getPID reads the PID for a VM from the PID file.
func (m *DHCPManager) getPID(vmID string) int {
	pidFile := m.pidFilePath(vmID)
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(string(pidData))
	return pid
}

// sendSignal sends a signal to a process.
func (m *DHCPManager) sendSignal(pid int, sig syscall.Signal) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(sig)
}

// waitForProcessExit waits for a process to exit with a timeout.
func (m *DHCPManager) waitForProcessExit(pid int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !m.isProcessRunning(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Force kill if still running
	if err := m.sendSignal(pid, syscall.SIGKILL); err != nil {
		m.logger.Warn("failed to send SIGKILL", "error", err)
	}
}

// cleanupVMFiles removes all DHCP-related files for a VM.
func (m *DHCPManager) cleanupVMFiles(vmID string) {
	files := []string{
		m.configFilePath(vmID),
		m.leaseFilePath(vmID),
		m.pidFilePath(vmID),
		m.logFilePath(vmID),
		m.statusFilePath(vmID),
	}

	for _, f := range files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			m.logger.Warn("failed to remove file", "file", f, "error", err)
		}
	}
}

// saveLeaseStatus saves the lease status to a file.
func (m *DHCPManager) saveLeaseStatus(vmID string, lease *DHCPLease) error {
	statusPath := m.statusFilePath(vmID)
	data, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling lease status: %w", err)
	}
	return os.WriteFile(statusPath, data, 0644)
}

// monitorProcess monitors a dnsmasq process and cleans up when it exits.
// cancel is the CancelFunc for the monitor context and is called when the goroutine
// completes to release context resources.
//
// It runs cmd.Wait() in an inner goroutine and selects between the wait result
// and ctx.Done().  This ensures that when the caller cancels the context (e.g.
// via DHCPManager.Stop()) the dnsmasq child process is killed promptly and
// wg.Done() is called so wg.Wait() in Stop() can return.
func (m *DHCPManager) monitorProcess(ctx context.Context, cancel context.CancelFunc, vmID string, cmd *exec.Cmd, logFile *os.File) {
	defer m.wg.Done()
	defer cancel()
	defer func() {
		if err := logFile.Close(); err != nil {
			m.logger.Debug("failed to close dnsmasq log file", "error", err)
		}
	}()

	// waitCh receives the result of cmd.Wait() from the inner goroutine.
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-waitCh:
		// Process exited on its own.
		if err != nil {
			m.logger.Warn("dnsmasq process exited with error", "vm_id", vmID, "error", err)
		} else {
			m.logger.Info("dnsmasq process exited normally", "vm_id", vmID)
		}
	case <-ctx.Done():
		// Context was cancelled (e.g. DHCPManager.Stop() was called).
		// Kill the dnsmasq process so cmd.Wait() in the inner goroutine returns.
		m.logger.Info("context cancelled, killing dnsmasq process", "vm_id", vmID)
		if cmd.Process != nil {
			if killErr := cmd.Process.Kill(); killErr != nil {
				m.logger.Warn("failed to kill dnsmasq process", "vm_id", vmID, "error", killErr)
			}
		}
		// Drain the wait channel so the inner goroutine is not leaked.
		<-waitCh
		err = ctx.Err()
	}

	// Update status
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update status file to stopped using the unlocked variant to avoid re-entrant lock.
	lease, err := m.getVMLeaseUnlocked(vmID)
	if err == nil && lease != nil {
		lease.Status = "stopped"
		if saveErr := m.saveLeaseStatus(vmID, lease); saveErr != nil {
			m.logger.Warn("failed to update lease status on exit", "vm_id", vmID, "error", saveErr)
		}
	}

	// Remove from running processes
	delete(m.runningProcs, vmID)
}

// maxHostnameLen is the maximum hostname length per RFC 1123.
const maxHostnameLen = 253

// sanitizeHostname sanitizes a VM name for use as a DHCP hostname.
// It applies character filtering and enforces the RFC 1123 limit of 253 characters.
func sanitizeHostname(name string) string {
	// Replace spaces and special characters
	result := strings.ReplaceAll(name, " ", "-")
	result = strings.ReplaceAll(result, ".", "-")
	result = strings.ReplaceAll(result, "_", "-")
	// Remove any remaining non-alphanumeric characters except dash
	var sb strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			sb.WriteRune(r)
		}
	}
	sanitized := sb.String()
	// Truncate to RFC 1123 maximum hostname length
	if len(sanitized) > maxHostnameLen {
		sanitized = sanitized[:maxHostnameLen]
	}
	return sanitized
}
