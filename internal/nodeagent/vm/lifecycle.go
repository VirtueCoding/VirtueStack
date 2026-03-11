package vm

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"libvirt.org/go/libvirt"
)

// Constants for VM lifecycle operations.
const (
	// DefaultShutdownTimeout is the default timeout for graceful VM shutdown.
	DefaultShutdownTimeout = 120
	// ShutdownPollInterval is the interval between shutdown status polls.
	ShutdownPollInterval = 2 * time.Second
	// DefaultLibvirtURI is the default libvirt connection URI.
	DefaultLibvirtURI = "qemu:///system"
)

// Manager handles VM lifecycle operations via libvirt.
type Manager struct {
	conn   *libvirt.Connect
	logger *slog.Logger
}

// NewManager creates a new VM Manager with the given libvirt connection.
func NewManager(conn *libvirt.Connect, logger *slog.Logger) *Manager {
	return &Manager{
		conn:   conn,
		logger: logger.With("component", "vm-manager"),
	}
}

// CreateVM creates and starts a new virtual machine.
// It defines the domain, starts it, and returns the VNC port.
func (m *Manager) CreateVM(ctx context.Context, cfg *DomainConfig) (*CreateResult, error) {
	logger := m.logger.With("vm_id", cfg.VMID)
	logger.Info("creating VM", "hostname", cfg.Hostname, "vcpu", cfg.VCPU, "memory_mb", cfg.MemoryMB)

	// Generate domain XML
	domainXML, err := GenerateDomainXML(cfg)
	if err != nil {
		return nil, fmt.Errorf("generating domain XML: %w", err)
	}

	// Define the domain
	domain, err := m.conn.DomainDefineXML(domainXML)
	if err != nil {
		return nil, fmt.Errorf("defining domain: %w", err)
	}
	defer domain.Free()

	domainName, err := domain.GetName()
	if err != nil {
		return nil, fmt.Errorf("getting domain name: %w", err)
	}

	// Start the domain
	if err := domain.Create(); err != nil {
		// Clean up: undefine the domain if start fails
		if undefineErr := domain.UndefineFlags(libvirt.DOMAIN_UNDEFINE_NVRAM); undefineErr != nil {
			logger.Error("failed to undefine domain after start failure", "error", undefineErr)
		}
		return nil, fmt.Errorf("starting domain: %w", err)
	}

	// Get VNC port from running domain's XML
	vncPort, err := m.getVNCPort(domain)
	if err != nil {
		logger.Warn("could not get VNC port", "error", err)
		vncPort = 0
	}

	logger.Info("VM created successfully", "domain_name", domainName, "vnc_port", vncPort)

	return &CreateResult{
		DomainName: domainName,
		VNCPort:    vncPort,
	}, nil
}

// StartVM starts a stopped virtual machine.
// Returns nil if the VM is already running (idempotent).
func (m *Manager) StartVM(ctx context.Context, vmID string) error {
	logger := m.logger.With("vm_id", vmID)
	logger.Info("starting VM")

	domain, err := m.lookupDomain(vmID)
	if err != nil {
		return err
	}
	defer domain.Free()

	// Check current state
	state, _, err := domain.GetState()
	if err != nil {
		return fmt.Errorf("getting domain state: %w", err)
	}

	// If already running, return success (idempotent)
	if state == libvirt.DOMAIN_RUNNING {
		logger.Info("VM already running")
		return nil
	}

	// Start the domain
	if err := domain.Create(); err != nil {
		return fmt.Errorf("starting VM %s: %w", vmID, err)
	}

	logger.Info("VM started successfully")
	return nil
}

// StopVM gracefully shuts down a VM using ACPI.
// It waits for the specified timeout before returning an error.
func (m *Manager) StopVM(ctx context.Context, vmID string, timeoutSec int) error {
	logger := m.logger.With("vm_id", vmID)
	logger.Info("stopping VM", "timeout_sec", timeoutSec)

	if timeoutSec <= 0 {
		timeoutSec = DefaultShutdownTimeout
	}

	domain, err := m.lookupDomain(vmID)
	if err != nil {
		return err
	}
	defer domain.Free()

	// Check current state
	state, _, err := domain.GetState()
	if err != nil {
		return fmt.Errorf("getting domain state: %w", err)
	}

	// If already stopped, return success
	if state == libvirt.DOMAIN_SHUTOFF {
		logger.Info("VM already stopped")
		return nil
	}

	// Send ACPI shutdown signal
	if err := domain.Shutdown(); err != nil {
		return fmt.Errorf("sending ACPI shutdown: %w", err)
	}

	// Poll for shutdown completion
	timeout := time.After(time.Duration(timeoutSec) * time.Second)
	ticker := time.NewTicker(ShutdownPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("shutdown cancelled: %w", ctx.Err())
		case <-timeout:
			return fmt.Errorf("shutdown timeout for VM %s: %w", vmID, errors.ErrTimeout)
		case <-ticker.C:
			currentState, _, err := domain.GetState()
			if err != nil {
				return fmt.Errorf("checking shutdown state: %w", err)
			}
			if currentState == libvirt.DOMAIN_SHUTOFF {
				logger.Info("VM stopped successfully")
				return nil
			}
		}
	}
}

// ForceStopVM immediately terminates a VM (power off).
// Use with caution as this may cause data loss.
func (m *Manager) ForceStopVM(ctx context.Context, vmID string) error {
	logger := m.logger.With("vm_id", vmID)
	logger.Info("force stopping VM")

	domain, err := m.lookupDomain(vmID)
	if err != nil {
		return err
	}
	defer domain.Free()

	// Check current state
	state, _, err := domain.GetState()
	if err != nil {
		return fmt.Errorf("getting domain state: %w", err)
	}

	// If already stopped, return success
	if state == libvirt.DOMAIN_SHUTOFF {
		logger.Info("VM already stopped")
		return nil
	}

	// Destroy (force power off) the domain
	if err := domain.Destroy(); err != nil {
		return fmt.Errorf("force stopping VM %s: %w", vmID, err)
	}

	logger.Info("VM force stopped successfully")
	return nil
}

// DeleteVM permanently removes a VM and its definition.
// If the VM is running, it will be force-stopped first.
func (m *Manager) DeleteVM(ctx context.Context, vmID string) error {
	logger := m.logger.With("vm_id", vmID)
	logger.Info("deleting VM")

	domain, err := m.lookupDomain(vmID)
	if err != nil {
		return err
	}
	defer domain.Free()

	// Check current state
	state, _, err := domain.GetState()
	if err != nil {
		return fmt.Errorf("getting domain state: %w", err)
	}

	// If running, destroy first
	if state == libvirt.DOMAIN_RUNNING {
		logger.Info("VM is running, destroying first")
		if err := domain.Destroy(); err != nil {
			return fmt.Errorf("destroying running VM %s: %w", vmID, err)
		}
	}

	// Undefine with NVRAM cleanup
	if err := domain.UndefineFlags(libvirt.DOMAIN_UNDEFINE_NVRAM); err != nil {
		return fmt.Errorf("undefining VM %s: %w", vmID, err)
	}

	logger.Info("VM deleted successfully")
	return nil
}

// GetStatus returns the current status of a VM.
func (m *Manager) GetStatus(ctx context.Context, vmID string) (*VMStatus, error) {
	domain, err := m.lookupDomain(vmID)
	if err != nil {
		return nil, err
	}
	defer domain.Free()

	// Get domain state
	state, _, err := domain.GetState()
	if err != nil {
		return nil, fmt.Errorf("getting domain state: %w", err)
	}

	// Get domain info for VCPU and memory
	info, err := domain.GetInfo()
	if err != nil {
		return nil, fmt.Errorf("getting domain info: %w", err)
	}

	// Get uptime if running
	var uptime int64
	if state == libvirt.DOMAIN_RUNNING {
		uptime, err = m.getDomainUptime(domain)
		if err != nil {
			m.logger.Warn("could not get domain uptime", "error", err, "vm_id", vmID)
		}
	}

	return &VMStatus{
		VMID:          vmID,
		Status:        mapLibvirtState(state),
		VCPU:          int32(info.NrVirtCpu),
		MemoryMB:      int32(info.Memory / 1024), // Convert from KB to MB
		UptimeSeconds: uptime,
	}, nil
}

// GetMetrics returns real-time resource utilization metrics for a VM.
func (m *Manager) GetMetrics(ctx context.Context, vmID string) (*VMMetrics, error) {
	domain, err := m.lookupDomain(vmID)
	if err != nil {
		return nil, err
	}
	defer domain.Free()

	// Check if running
	state, _, err := domain.GetState()
	if err != nil {
		return nil, fmt.Errorf("getting domain state: %w", err)
	}

	if state != libvirt.DOMAIN_RUNNING {
		return &VMMetrics{VMID: vmID}, nil
	}

	// Get CPU stats
	cpuPercent, err := m.getCPUUsage(domain)
	if err != nil {
		m.logger.Warn("could not get CPU usage", "error", err, "vm_id", vmID)
	}

	// Get memory stats
	memUsage, memTotal, err := m.getMemoryUsage(domain)
	if err != nil {
		m.logger.Warn("could not get memory usage", "error", err, "vm_id", vmID)
	}

	// Get disk stats
	diskRead, diskWrite, err := m.getDiskStats(domain)
	if err != nil {
		m.logger.Warn("could not get disk stats", "error", err, "vm_id", vmID)
	}

	// Get network stats
	netRX, netTX, err := m.getNetworkStats(domain)
	if err != nil {
		m.logger.Warn("could not get network stats", "error", err, "vm_id", vmID)
	}

	return &VMMetrics{
		VMID:             vmID,
		CPUUsagePercent:  cpuPercent,
		MemoryUsageBytes: memUsage,
		MemoryTotalBytes: memTotal,
		DiskReadBytes:    diskRead,
		DiskWriteBytes:   diskWrite,
		NetworkRXBytes:   netRX,
		NetworkTXBytes:   netTX,
	}, nil
}

// GetNodeResources returns aggregate resource information for the node.
func (m *Manager) GetNodeResources(ctx context.Context) (*NodeResources, error) {
	logger := m.logger.With("operation", "get_node_resources")

	// Get node info
	nodeInfo, err := m.conn.GetNodeInfo()
	if err != nil {
		return nil, fmt.Errorf("getting node info: %w", err)
	}

	// Get active domains
	domains, err := m.conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE)
	if err != nil {
		return nil, fmt.Errorf("listing active domains: %w", err)
	}

	// Calculate used resources
	var usedVCPU int32
	var usedMemoryMB int64
	for _, dom := range domains {
		info, err := dom.GetInfo()
		if err != nil {
			logger.Warn("could not get domain info", "error", err)
			continue
		}
		usedVCPU += int32(info.NrVirtCpu)
		usedMemoryMB += int64(info.Memory / 1024) // Convert from KB to MB
		dom.Free()
	}

	// Read load average from /proc/loadavg
	loadAvg := m.readLoadAverage()

	// Read uptime from /proc/uptime
	uptime := m.readUptime()

	return &NodeResources{
		TotalVCPU:     int32(nodeInfo.Cores * nodeInfo.Sockets * nodeInfo.Threads),
		UsedVCPU:      usedVCPU,
		TotalMemoryMB: int64(nodeInfo.Memory / 1024), // Convert from KB to MB
		UsedMemoryMB:  usedMemoryMB,
		VMCount:       int32(len(domains)),
		LoadAverage:   loadAvg,
		UptimeSeconds: uptime,
	}, nil
}

// lookupDomain looks up a domain by VM ID.
// The domain name is formatted as "vs-{vmID}".
func (m *Manager) lookupDomain(vmID string) (*libvirt.Domain, error) {
	domainName := DomainNameFromID(vmID)
	domain, err := m.conn.LookupDomainByName(domainName)
	if err != nil {
		if isLibvirtError(err, libvirt.ERR_NO_DOMAIN) {
			return nil, fmt.Errorf("VM %s: %w", vmID, errors.ErrNotFound)
		}
		return nil, fmt.Errorf("looking up domain %s: %w", domainName, err)
	}
	return domain, nil
}

// getVNCPort extracts the VNC port from a running domain's XML.
func (m *Manager) getVNCPort(domain *libvirt.Domain) (int32, error) {
	xmlDesc, err := domain.GetXMLDesc(0)
	if err != nil {
		return 0, fmt.Errorf("getting domain XML: %w", err)
	}

	var domainDef struct {
		Devices struct {
			Graphics []struct {
				Type string `xml:"type,attr"`
				Port string `xml:"port,attr"`
			} `xml:"graphics"`
		} `xml:"devices"`
	}

	if err := xml.Unmarshal([]byte(xmlDesc), &domainDef); err != nil {
		return 0, fmt.Errorf("parsing domain XML: %w", err)
	}

	for _, gfx := range domainDef.Devices.Graphics {
		if gfx.Type == "vnc" {
			port, err := strconv.ParseInt(gfx.Port, 10, 32)
			if err != nil {
				return 0, fmt.Errorf("parsing VNC port: %w", err)
			}
			return int32(port), nil
		}
	}

	return 0, fmt.Errorf("VNC graphics not found in domain XML")
}

// getDomainUptime returns the uptime of a running domain in seconds.
func (m *Manager) getDomainUptime(domain *libvirt.Domain) (int64, error) {
	// Use the domain's metadata or calculate from creation time
	// libvirt doesn't have a direct uptime API, so we approximate
	// by checking the domain's start time from metadata
	return 0, nil // Placeholder - in production, track VM start times
}

// getCPUUsage calculates the CPU usage percentage for a domain.
func (m *Manager) getCPUUsage(domain *libvirt.Domain) (float64, error) {
	// Get CPU time at two points and calculate usage
	cpuTime1, err := domain.GetInfo()
	if err != nil {
		return 0, err
	}

	time.Sleep(100 * time.Millisecond)

	cpuTime2, err := domain.GetInfo()
	if err != nil {
		return 0, err
	}

	// Calculate CPU usage percentage
	// cpuTime is in nanoseconds
	cpuDelta := cpuTime2.CpuTime - cpuTime1.CpuTime
	timeDelta := 100 * 1e6 // 100ms in nanoseconds

	// CPU usage = (cpu_delta / time_delta) * 100 / num_cpus
	cpuPercent := float64(cpuDelta) / float64(timeDelta) * 100 / float64(cpuTime1.NrVirtCpu)
	if cpuPercent > 100 {
		cpuPercent = 100
	}

	return cpuPercent, nil
}

// getMemoryUsage returns the memory usage and total for a domain.
func (m *Manager) getMemoryUsage(domain *libvirt.Domain) (int64, int64, error) {
	info, err := domain.GetInfo()
	if err != nil {
		return 0, 0, err
	}

	totalBytes := int64(info.Memory) * 1024 // Convert from KB to bytes
	// libvirt doesn't provide actual memory usage directly
	// In production, use memory balloon or guest agent

	return 0, totalBytes, nil
}

// getDiskStats returns the disk read and write bytes for a domain.
func (m *Manager) getDiskStats(domain *libvirt.Domain) (int64, int64, error) {
	stats, err := domain.BlockStats("vda")
	if err != nil {
		return 0, 0, err
	}

	return stats.RdBytes, stats.WrBytes, nil
}

// getNetworkStats returns the network RX and TX bytes for a domain.
func (m *Manager) getNetworkStats(domain *libvirt.Domain) (int64, int64, error) {
	// Get interface stats - we need to find the interface name first
	// For simplicity, we'll get stats from the first interface
	ifaces, err := domain.ListAllInterfaceAddresses(libvirt.DOMAIN_INTERFACE_ADDRESSES_SRC_AGENT)
	if err != nil {
		// Fall back to parsing domain XML for interface name
		return m.getNetworkStatsFromXML(domain)
	}

	var totalRX, totalTX int64
	for _, iface := range ifaces {
		totalRX += iface.RxBytes
		totalTX += iface.TxBytes
	}

	return totalRX, totalTX, nil
}

// getNetworkStatsFromXML extracts network stats by parsing domain XML.
func (m *Manager) getNetworkStatsFromXML(domain *libvirt.Domain) (int64, int64, error) {
	xmlDesc, err := domain.GetXMLDesc(0)
	if err != nil {
		return 0, 0, err
	}

	var domainDef struct {
		Devices struct {
			Interfaces []struct {
				Target struct {
					Dev string `xml:"dev,attr"`
				} `xml:"target"`
			} `xml:"interface"`
		} `xml:"devices"`
	}

	if err := xml.Unmarshal([]byte(xmlDesc), &domainDef); err != nil {
		return 0, 0, err
	}

	var totalRX, totalTX int64
	for _, iface := range domainDef.Devices.Interfaces {
		if iface.Target.Dev != "" {
			stats, err := domain.InterfaceStats(iface.Target.Dev)
			if err != nil {
				continue
			}
			totalRX += stats.RxBytes
			totalTX += stats.TxBytes
		}
	}

	return totalRX, totalTX, nil
}

// readLoadAverage reads the load average from /proc/loadavg.
func (m *Manager) readLoadAverage() [3]float64 {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		m.logger.Warn("could not read /proc/loadavg", "error", err)
		return [3]float64{}
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return [3]float64{}
	}

	var load [3]float64
	for i := 0; i < 3 && i < len(fields); i++ {
		val, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			continue
		}
		load[i] = val
	}

	return load
}

// readUptime reads the system uptime from /proc/uptime.
func (m *Manager) readUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		m.logger.Warn("could not read /proc/uptime", "error", err)
		return 0
	}

	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0
	}

	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}

	return int64(uptime)
}

// isLibvirtError checks if an error is a specific libvirt error code.
func isLibvirtError(err error, code libvirt.ErrorNumber) bool {
	if lerr, ok := err.(libvirt.Error); ok {
		return lerr.Code == code
	}
	return false
}