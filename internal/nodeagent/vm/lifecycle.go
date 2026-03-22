package vm

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/libvirtutil"
	"libvirt.org/go/libvirt"
)

type cpuUsageCacheEntry struct {
	UsagePercent float64
	LastCPUTime  uint64
	LastSampleAt time.Time
	Initialized  bool
	Sampling     bool
}

var (
	cpuUsageCacheMu   sync.RWMutex
	cpuUsageCache     = make(map[string]cpuUsageCacheEntry)
	samplerCancelMu   sync.Mutex
	samplerCancelFuncs = make(map[string]context.CancelFunc)
)

// Constants for VM lifecycle operations.
const (
	// DefaultShutdownTimeout is the default timeout for graceful VM shutdown.
	DefaultShutdownTimeout = 120
	// ShutdownPollInterval is the interval between shutdown status polls.
	ShutdownPollInterval = 2 * time.Second
	// DefaultLibvirtURI is the default libvirt connection URI.
	DefaultLibvirtURI = "qemu:///system"
	// DefaultDataDir is the default directory for VM state persistence.
	DefaultDataDir = "/var/lib/virtuestack"
	// CPUSampleInterval is the interval between CPU usage samples.
	// Override via NodeAgentConfig if finer or coarser granularity is needed.
	CPUSampleInterval = 5 * time.Second
)

// Manager handles VM lifecycle operations via libvirt.
type Manager struct {
	conn    *libvirt.Connect
	logger  *slog.Logger
	dataDir string
	cpuWg   sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewManager creates a new VM Manager with the given libvirt connection.
// It uses DefaultDataDir for persistence if dataDir is empty.
func NewManager(conn *libvirt.Connect, logger *slog.Logger, dataDir string) *Manager {
	if dataDir == "" {
		dataDir = DefaultDataDir
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		conn:    conn,
		logger:  logger.With("component", "vm-manager"),
		dataDir: dataDir,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Stop stops all background goroutines and releases resources.
func (m *Manager) Stop() {
	m.cancel()
	m.cpuWg.Wait()
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
	defer func() {
		if err := domain.Free(); err != nil {
			m.logger.Debug("failed to free domain", "error", err)
		}
	}()

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

	// Record start time for uptime tracking
	if err := m.recordVMStartTime(cfg.VMID); err != nil {
		logger.Warn("failed to record VM start time", "error", err)
		// Don't fail the operation if recording fails
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
// Records the start time for uptime tracking.
func (m *Manager) StartVM(ctx context.Context, vmID string) error {
	logger := m.logger.With("vm_id", vmID)
	logger.Info("starting VM")

	domain, err := m.lookupDomain(vmID)
	if err != nil {
		return fmt.Errorf("lookup domain for start: %w", err)
	}
	defer func() {
		if err := domain.Free(); err != nil {
			m.logger.Debug("failed to free domain", "error", err)
		}
	}()

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

	// Record start time for uptime tracking
	if err := m.recordVMStartTime(vmID); err != nil {
		logger.Warn("failed to record VM start time", "error", err)
		// Don't fail the operation if recording fails
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
		return fmt.Errorf("lookup domain for stop: %w", err)
	}
	defer func() {
		if err := domain.Free(); err != nil {
			m.logger.Debug("failed to free domain", "error", err)
		}
	}()

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
				// Clear the start time since VM is now stopped
				m.clearVMStartTime(vmID)
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
		return fmt.Errorf("lookup domain for force stop: %w", err)
	}
	defer func() {
		if err := domain.Free(); err != nil {
			m.logger.Debug("failed to free domain", "error", err)
		}
	}()

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

	// Clear the start time since VM is now stopped
	m.clearVMStartTime(vmID)

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
		return fmt.Errorf("lookup domain for delete: %w", err)
	}
	defer func() {
		if err := domain.Free(); err != nil {
			m.logger.Debug("failed to free domain", "error", err)
		}
	}()

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

	// Clear the uptime data since VM is deleted
	m.clearVMStartTime(vmID)

	// Stop CPU sampler goroutine to prevent resource leak
	domainName := DomainNameFromID(vmID)
	stopCPUSampler(domainName)

	// Clear CPU usage cache entry to prevent memory leak
	cpuUsageCacheMu.Lock()
	delete(cpuUsageCache, domainName)
	cpuUsageCacheMu.Unlock()

	logger.Info("VM deleted successfully")
	return nil
}

// GetStatus returns the current status of a VM.
func (m *Manager) GetStatus(ctx context.Context, vmID string) (*VMStatus, error) {
	domain, err := m.lookupDomain(vmID)
	if err != nil {
		return nil, fmt.Errorf("lookup domain for status: %w", err)
	}
	defer func() {
		if err := domain.Free(); err != nil {
			m.logger.Debug("failed to free domain", "error", err)
		}
	}()

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
		return nil, fmt.Errorf("lookup domain for metrics: %w", err)
	}
	defer func() {
		if err := domain.Free(); err != nil {
			m.logger.Debug("failed to free domain", "error", err)
		}
	}()

	// Check if running
	state, _, err := domain.GetState()
	if err != nil {
		return nil, fmt.Errorf("getting domain state: %w", err)
	}

	if state != libvirt.DOMAIN_RUNNING {
		return &VMMetrics{VMID: vmID}, nil
	}

	// Collect all metrics
	logger := m.logger.With("vm_id", vmID)
	data := m.collectMetrics(ctx, domain, vmID, logger)
	return data.toVMMetrics(vmID), nil
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
		if err := dom.Free(); err != nil {
			logger.Debug("failed to free domain", "error", err)
		}
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
		if libvirtutil.IsLibvirtError(err, libvirt.ERR_NO_DOMAIN) {
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

// uptimeData represents the persisted uptime data for a VM.
type uptimeData struct {
	StartTimeUnix int64  `json:"start_time_unix"`
	VMID          string `json:"vm_id"`
}

// getUptimeDir returns the directory path for storing uptime data.
func (m *Manager) getUptimeDir() string {
	return filepath.Join(m.dataDir, "uptime")
}

// getUptimeFilePath returns the file path for a VM's uptime data.
func (m *Manager) getUptimeFilePath(vmID string) string {
	return filepath.Join(m.getUptimeDir(), fmt.Sprintf("%s.json", vmID))
}

// ensureUptimeDir creates the uptime directory if it doesn't exist.
func (m *Manager) ensureUptimeDir() error {
	dir := m.getUptimeDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating uptime directory: %w", err)
	}
	return nil
}

// recordVMStartTime persists the VM start time to disk.
// This allows uptime tracking to survive agent restarts.
func (m *Manager) recordVMStartTime(vmID string) error {
	if err := m.ensureUptimeDir(); err != nil {
		return err
	}

	data := uptimeData{
		StartTimeUnix: time.Now().Unix(),
		VMID:          vmID,
	}

	filePath := m.getUptimeFilePath(vmID)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("creating uptime file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			m.logger.Debug("failed to close uptime file", "error", err)
		}
	}()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("encoding uptime data: %w", err)
	}

	return nil
}

// getVMStartTime retrieves the persisted start time for a VM.
// Returns 0 if no start time is recorded (VM never started or data lost).
func (m *Manager) getVMStartTime(vmID string) int64 {
	filePath := m.getUptimeFilePath(vmID)

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		m.logger.Warn("failed to open uptime file", "vm_id", vmID, "error", err)
		return 0
	}
	defer func() {
		if err := file.Close(); err != nil {
			m.logger.Debug("failed to close uptime file", "error", err)
		}
	}()

	var data uptimeData
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		m.logger.Warn("failed to decode uptime data", "vm_id", vmID, "error", err)
		return 0
	}

	return data.StartTimeUnix
}

// clearVMStartTime removes the persisted start time for a VM.
// Called when a VM is stopped or deleted.
func (m *Manager) clearVMStartTime(vmID string) {
	filePath := m.getUptimeFilePath(vmID)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		m.logger.Warn("failed to remove uptime file", "vm_id", vmID, "error", err)
	}
}

// getDomainUptime returns the uptime of a running domain in seconds.
// It calculates uptime from the persisted start time, surviving agent restarts.
func (m *Manager) getDomainUptime(domain *libvirt.Domain) (int64, error) {
	// Get domain name to lookup the VM ID
	name, err := domain.GetName()
	if err != nil {
		return 0, fmt.Errorf("getting domain name: %w", err)
	}

	// Extract VM ID from domain name (format: vs-{vmID})
	vmID := strings.TrimPrefix(name, "vs-")
	if vmID == name {
		return 0, fmt.Errorf("could not extract VM ID from domain name: %s", name)
	}

	// Get the persisted start time
	startTimeUnix := m.getVMStartTime(vmID)
	if startTimeUnix == 0 {
		// No start time recorded, return 0
		return 0, nil
	}

	// Calculate uptime in seconds
	startTime := time.Unix(startTimeUnix, 0)
	uptime := time.Since(startTime).Seconds()

	return int64(uptime), nil
}

// getCPUUsage calculates the CPU usage percentage for a domain.
func (m *Manager) getCPUUsage(ctx context.Context, domain *libvirt.Domain) (float64, error) {
	domainName, err := domain.GetName()
	if err != nil {
		return 0, err
	}

	m.ensureCPUSampler(ctx, domainName)

	cpuUsageCacheMu.RLock()
	entry, ok := cpuUsageCache[domainName]
	cpuUsageCacheMu.RUnlock()
	if ok {
		return entry.UsagePercent, nil
	}

	info, err := domain.GetInfo()
	if err != nil {
		return 0, err
	}
	if info.NrVirtCpu == 0 {
		return 0, nil
	}

	cpuUsageCacheMu.Lock()
	cpuUsageCache[domainName] = cpuUsageCacheEntry{
		UsagePercent: 0,
		LastCPUTime:  info.CpuTime,
		LastSampleAt: time.Now(),
		Initialized:  true,
	}
	cpuUsageCacheMu.Unlock()

	return 0, nil
}

func (m *Manager) ensureCPUSampler(ctx context.Context, domainName string) {
	cpuUsageCacheMu.Lock()
	entry := cpuUsageCache[domainName]
	if entry.Sampling {
		cpuUsageCacheMu.Unlock()
		return
	}
	entry.Sampling = true
	cpuUsageCache[domainName] = entry
	cpuUsageCacheMu.Unlock()

	// Create a per-domain context that can be cancelled when the VM is deleted
	// Derive sampler context from passed context. We use context.WithoutCancel to detach
	// from request cancellation since the sampler should run independently of individual requests.
	// The sampler is cancelled when the VM is deleted via stopCPUSampler.
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	samplerCancelMu.Lock()
	samplerCancelFuncs[domainName] = cancel
	samplerCancelMu.Unlock()

	m.cpuWg.Add(1)
	go m.runCPUSampler(ctx, domainName)
}

func (m *Manager) runCPUSampler(ctx context.Context, domainName string) {
	defer m.cpuWg.Done()

	m.sampleCPUUsage(domainName)
	ticker := time.NewTicker(CPUSampleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sampleCPUUsage(domainName)
		}
	}
}

// stopCPUSampler stops the CPU sampler goroutine for a domain.
func stopCPUSampler(domainName string) {
	samplerCancelMu.Lock()
	if cancel, ok := samplerCancelFuncs[domainName]; ok {
		cancel()
		delete(samplerCancelFuncs, domainName)
	}
	samplerCancelMu.Unlock()
}

func (m *Manager) sampleCPUUsage(domainName string) {
	domain, err := m.conn.LookupDomainByName(domainName)
	if err != nil {
		return
	}
	defer func() {
		if err := domain.Free(); err != nil {
			m.logger.Debug("failed to free domain", "error", err)
		}
	}()

	info, err := domain.GetInfo()
	if err != nil {
		return
	}

	now := time.Now()
	if info.NrVirtCpu == 0 {
		return
	}

	cpuUsageCacheMu.Lock()
	entry := cpuUsageCache[domainName]
	if entry.Initialized && !entry.LastSampleAt.IsZero() && info.CpuTime >= entry.LastCPUTime {
		cpuDelta := info.CpuTime - entry.LastCPUTime
		timeDelta := now.Sub(entry.LastSampleAt).Nanoseconds()
		if timeDelta > 0 {
			cpuPercent := float64(cpuDelta) / float64(timeDelta) * 100 / float64(info.NrVirtCpu)
			if cpuPercent < 0 {
				cpuPercent = 0
			}
			if cpuPercent > 100 {
				cpuPercent = 100
			}
			entry.UsagePercent = cpuPercent
		}
	}
	entry.LastCPUTime = info.CpuTime
	entry.LastSampleAt = now
	entry.Initialized = true
	entry.Sampling = true
	cpuUsageCache[domainName] = entry
	cpuUsageCacheMu.Unlock()
}

// getMemoryUsage returns the memory usage and total for a domain.
func (m *Manager) getMemoryUsage(domain *libvirt.Domain) (int64, int64, error) {
	info, err := domain.GetInfo()
	if err != nil {
		return 0, 0, err
	}

	totalBytes := int64(info.Memory) * 1024 // Convert from KB to bytes

	domainName, err := domain.GetName()
	if err == nil {
		if usage, readErr := readDomainMemoryUsage(domainName); readErr == nil {
			return usage, totalBytes, nil
		}
	}

	// Fall back to libvirt memory stats
	usage, ok := m.getMemoryUsageFromStats(domain)
	if ok {
		return usage, totalBytes, nil
	}

	return 0, totalBytes, nil
}

// getMemoryUsageFromStats extracts memory usage from libvirt domain memory stats.
// Returns the usage in bytes and true if successful, or 0 and false if not available.
func (m *Manager) getMemoryUsageFromStats(domain *libvirt.Domain) (int64, bool) {
	stats, err := domain.MemoryStats(uint32(libvirt.DOMAIN_MEMORY_STAT_NR), 0)
	if err != nil {
		return 0, false
	}

	available, unused, rss := parseMemoryStats(stats)

	// Try available/unused calculation first
	if available > 0 && available >= unused {
		usedBytes := int64((available - unused) * 1024)
		if usedBytes >= 0 {
			return usedBytes, true
		}
	}

	// Fall back to RSS
	if rss > 0 {
		return int64(rss * 1024), true
	}

	return 0, false
}

// parseMemoryStats extracts available, unused, and RSS values from memory stats.
func parseMemoryStats(stats []libvirt.DomainMemoryStat) (available, unused, rss uint64) {
	for _, stat := range stats {
		//nolint:exhaustive // We only care about these specific memory stats; others are intentionally ignored
		switch libvirt.DomainMemoryStatTags(stat.Tag) {
		case libvirt.DOMAIN_MEMORY_STAT_AVAILABLE:
			available = stat.Val
		case libvirt.DOMAIN_MEMORY_STAT_UNUSED:
			unused = stat.Val
		case libvirt.DOMAIN_MEMORY_STAT_RSS:
			rss = stat.Val
		default:
			// Ignore other memory stat tags (e.g., SWAP_IN, SWAP_OUT, MAJOR_FAULT, etc.)
		}
	}
	return
}

func readDomainMemoryUsage(domainName string) (int64, error) {
	escaped := strings.ReplaceAll(domainName, "-", "\\x2d")
	v2Paths := []string{
		filepath.Join("/sys/fs/cgroup/machine.slice", "machine-qemu\\x2d"+escaped+".scope", "memory.current"),
		filepath.Join("/sys/fs/cgroup/system.slice/libvirt-qemu.service/machine.slice", "machine-qemu\\x2d"+escaped+".scope", "memory.current"),
	}

	for _, p := range v2Paths {
		if val, err := readInt64File(p); err == nil {
			return val, nil
		}
	}

	v1Paths := []string{
		filepath.Join("/sys/fs/cgroup/memory/machine.slice", "machine-qemu\\x2d"+escaped+".scope", "memory.usage_in_bytes"),
		filepath.Join("/sys/fs/cgroup/memory/libvirt/qemu", domainName, "memory.usage_in_bytes"),
	}

	for _, p := range v1Paths {
		if val, err := readInt64File(p); err == nil {
			return val, nil
		}
	}

	return 0, fmt.Errorf("memory usage not found")
}

func readInt64File(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	valueStr := strings.TrimSpace(string(data))
	if valueStr == "" {
		return 0, fmt.Errorf("empty value")
	}

	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		return 0, err
	}

	if value < 0 {
		return 0, fmt.Errorf("negative value")
	}

	return value, nil
}

// getDiskStats returns the disk read and write bytes for a domain.
func (m *Manager) getDiskStats(domain *libvirt.Domain) (int64, int64, error) {
	stats, err := domain.BlockStats("vda")
	if err != nil {
		return 0, 0, err
	}

	return stats.RdBytes, stats.WrBytes, nil
}

func (m *Manager) getDiskStatsFull(domain *libvirt.Domain) (int64, int64, int64, int64, error) {
	stats, err := domain.BlockStats("vda")
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return stats.RdBytes, stats.WrBytes, stats.RdReq, stats.WrReq, nil
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
		stats, err := domain.InterfaceStats(iface.Name)
		if err != nil {
			continue
		}
		totalRX += stats.RxBytes
		totalTX += stats.TxBytes
	}

	return totalRX, totalTX, nil
}

// getNetworkStatsFromXML extracts network stats by parsing domain XML.
func (m *Manager) getNetworkStatsFromXML(domain *libvirt.Domain) (int64, int64, error) {
	xmlDesc, err := domain.GetXMLDesc(0)
	if err != nil {
		return 0, 0, err
	}

	names, err := libvirtutil.GetInterfaceNames(xmlDesc)
	if err != nil {
		return 0, 0, err
	}

	var totalRX, totalTX int64
	for _, ifaceName := range names {
		stats, err := domain.InterfaceStats(ifaceName)
		if err != nil {
			continue
		}
		totalRX += stats.RxBytes
		totalTX += stats.TxBytes
	}

	return totalRX, totalTX, nil
}

func (m *Manager) getNetworkStatsFull(domain *libvirt.Domain) (rxBytes, txBytes, rxPkts, txPkts, rxErrs, txErrs, rxDrop, txDrop int64, err error) {
	ifaces, err := domain.ListAllInterfaceAddresses(libvirt.DOMAIN_INTERFACE_ADDRESSES_SRC_AGENT)
	if err != nil {
		return m.getNetworkStatsFullFromXML(domain)
	}

	for _, iface := range ifaces {
		stats, err := domain.InterfaceStats(iface.Name)
		if err != nil {
			continue
		}
		rxBytes += stats.RxBytes
		txBytes += stats.TxBytes
		rxPkts += stats.RxPackets
		txPkts += stats.TxPackets
		rxErrs += stats.RxErrs
		txErrs += stats.TxErrs
		rxDrop += stats.RxDrop
		txDrop += stats.TxDrop
	}

	return rxBytes, txBytes, rxPkts, txPkts, rxErrs, txErrs, rxDrop, txDrop, nil
}

func (m *Manager) getNetworkStatsFullFromXML(domain *libvirt.Domain) (rxBytes, txBytes, rxPkts, txPkts, rxErrs, txErrs, rxDrop, txDrop int64, err error) {
	xmlDesc, err := domain.GetXMLDesc(0)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}

	names, err := libvirtutil.GetInterfaceNames(xmlDesc)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, err
	}

	for _, ifaceName := range names {
		stats, err := domain.InterfaceStats(ifaceName)
		if err != nil {
			continue
		}
		rxBytes += stats.RxBytes
		txBytes += stats.TxBytes
		rxPkts += stats.RxPackets
		txPkts += stats.TxPackets
		rxErrs += stats.RxErrs
		txErrs += stats.TxErrs
		rxDrop += stats.RxDrop
		txDrop += stats.TxDrop
	}

	return
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
