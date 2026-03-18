// Package network provides helper functions for DHCP manager operations.
// These functions decompose the StartDHCPForVMWithConfig function to comply with
// CODING_STANDARD.md QG-01 (functions <= 40 lines).
package network

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// dhcpProcessCheck contains the result of checking for existing DHCP processes.
type dhcpProcessCheck struct {
	running bool
	pid     int
}

// checkExistingDHCPProcess checks if a DHCP process is already running for the VM.
// Returns whether a process is running and its PID.
func (m *DHCPManager) checkExistingDHCPProcess(vmID string, logger *slog.Logger) (*dhcpProcessCheck, error) {
	// Check in-memory tracking first
	if proc, exists := m.runningProcs[vmID]; exists {
		if m.isProcessRunning(proc.pid) {
			logger.Warn("DHCP already running for VM")
			return &dhcpProcessCheck{running: true, pid: proc.pid},
				fmt.Errorf("DHCP already running for VM %s: %w", vmID, errors.ErrAlreadyExists)
		}
		// Process is dead, clean up
		delete(m.runningProcs, vmID)
	}

	// Check PID file as well
	pidFile := m.pidFilePath(vmID)
	if pidData, err := os.ReadFile(pidFile); err == nil {
		pid, _ := strconv.Atoi(string(pidData))
		if pid > 0 && m.isProcessRunning(pid) {
			logger.Warn("DHCP already running for VM (from PID file)")
			return &dhcpProcessCheck{running: true, pid: pid},
				fmt.Errorf("DHCP already running for VM %s: %w", vmID, errors.ErrAlreadyExists)
		}
	}
	return &dhcpProcessCheck{running: false}, nil
}

// dhcpFiles contains the paths to DHCP-related files.
type dhcpFiles struct {
	configPath string
	leasePath  string
	logPath    string
	pidFile    string
}

// writeDHCPConfigFiles generates and writes the DHCP configuration files.
func (m *DHCPManager) writeDHCPConfigFiles(cfg DHCPConfig) (*dhcpFiles, error) {
	// Generate config file content
	configContent, err := m.GenerateDNSMasqConfigFull(cfg)
	if err != nil {
		return nil, fmt.Errorf("generating DHCP config for VM %s: %w", cfg.VMID, err)
	}

	files := &dhcpFiles{
		configPath: m.configFilePath(cfg.VMID),
		leasePath:  m.leaseFilePath(cfg.VMID),
		logPath:    m.logFilePath(cfg.VMID),
		pidFile:    m.pidFilePath(cfg.VMID),
	}

	// Write config file
	if err := os.WriteFile(files.configPath, []byte(configContent), 0644); err != nil {
		return nil, fmt.Errorf("writing DHCP config file: %w", err)
	}

	// Ensure log directory exists
	logDir := m.logDir
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("creating log directory: %w", err)
	}

	// Create empty lease file
	if err := os.WriteFile(files.leasePath, []byte{}, 0644); err != nil {
		return nil, fmt.Errorf("creating lease file: %w", err)
	}

	return files, nil
}

// startDNSMasqProcess starts the dnsmasq process and returns the command and PID.
func (m *DHCPManager) startDNSMasqProcess(
	ctx context.Context,
	files *dhcpFiles,
	logger *slog.Logger,
) (*exec.Cmd, *os.File, int, error) {
	// Test config before starting
	if err := m.testDNSMasqConfig(files.configPath); err != nil {
		os.Remove(files.configPath)
		os.Remove(files.leasePath)
		return nil, nil, 0, fmt.Errorf("invalid dnsmasq config: %w", err)
	}

	// Start dnsmasq
	cmd := exec.CommandContext(ctx, "dnsmasq",
		"-C", files.configPath,
		"--keep-in-foreground",
		"--no-daemon",
	)

	// Set up log file for dnsmasq output
	logFile, err := os.OpenFile(files.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("opening log file: %w", err)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Start the process
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, nil, 0, fmt.Errorf("starting dnsmasq: %w", err)
	}

	// Get the PID
	pid := cmd.Process.Pid

	// Write PID file
	if err := os.WriteFile(files.pidFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		// Kill the process if we can't write PID file
		cmd.Process.Kill()
		logFile.Close()
		return nil, nil, 0, fmt.Errorf("writing PID file: %w", err)
	}

	return cmd, logFile, pid, nil
}

// trackDHCPProcess saves the lease status and tracks the running process.
func (m *DHCPManager) trackDHCPProcess(
	cfg DHCPConfig,
	cmd *exec.Cmd,
	pid int,
	logger *slog.Logger,
) {
	// Save status
	lease := &DHCPLease{
		VMID:       cfg.VMID,
		VMName:     cfg.VMName,
		MACAddress: cfg.MACAddress,
		IPAddress:  cfg.IPAddress,
		Gateway:    cfg.Gateway,
		DNS:        cfg.DNS,
		StartedAt:  time.Now(),
		PID:        pid,
		Status:     "running",
	}
	if err := m.saveLeaseStatus(cfg.VMID, lease); err != nil {
		logger.Warn("failed to save lease status", "error", err)
	}

	// Track the process
	m.runningProcs[cfg.VMID] = &dnsmasqProcess{
		cmd:       cmd,
		startTime: time.Now(),
		pid:       pid,
	}

	logger.Info("DHCP started successfully", "pid", pid)
}

// startProcessMonitor starts the goroutine that monitors the dnsmasq process.
func (m *DHCPManager) startProcessMonitor(
	ctx context.Context,
	cfg DHCPConfig,
	cmd *exec.Cmd,
	logFile *os.File,
) {
	// Start goroutine to wait for process exit and clean up.
	// A fresh background context with timeout is created so that cancellation of the
	// caller's ctx does not immediately cancel the monitor. cancel() is called inside
	// the goroutine after it finishes so the context is properly released.
	m.wg.Add(1)
	monitorCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	go m.monitorProcess(monitorCtx, cancel, cfg.VMID, cmd, logFile)
}