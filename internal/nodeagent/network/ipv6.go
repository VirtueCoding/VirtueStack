// Package network provides network management for the VirtueStack Node Agent.
// This file implements IPv6 subnet allocation for VMs in an L2 bridged architecture.
//
// ARCHITECTURE NOTE (L2 Bridged):
// In the new L2 bridged architecture, the node's bridge (br0) is a simple L2 bridge
// connected to an upstream VLAN. IPv6 routing and Router Advertisements are handled
// by the upstream router, NOT by individual nodes. This file only handles:
//   - /64 subnet allocation per VM (for cloud-init static configuration)
//   - EUI-64 address generation
//   - Subnet tracking and release
//
// The node agent does NOT:
//   - Run radvd (Router Advertisements from upstream)
//   - Configure /48 prefix on bridge (managed centrally)
//   - Act as IPv6 router (upstream handles this)
package network

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// Constants for IPv6 management.
const (
	// IPv6StatusSuffix is the suffix for IPv6 status files.
	IPv6StatusSuffix = ".status"
	// IPv6VMSubnetLen is the subnet prefix length per VM.
	IPv6VMSubnetLen = 64
	// IPv6MaxSubnets is the maximum number of /64 subnets in a /48.
	IPv6MaxSubnets = 65536
)

// IPv6Subnet represents an allocated IPv6 subnet for a VM.
type IPv6Subnet struct {
	// VMID is the unique identifier for the VM.
	VMID string `json:"vm_id"`
	// Subnet is the allocated subnet CIDR (e.g., 2001:db8:0:1::/64).
	Subnet string `json:"subnet"`
	// Gateway is the gateway address for the subnet (e.g., 2001:db8:0:1::1).
	Gateway string `json:"gateway"`
	// PrefixLen is the prefix length (always 64 for VM subnets).
	PrefixLen int `json:"prefix_len"`
	// Index is the subnet index within the node's /48 (0-65535).
	Index int `json:"index"`
}

// IPv6Info contains current IPv6 configuration for a VM.
type IPv6Info struct {
	// VMID is the unique identifier for the VM.
	VMID string `json:"vm_id"`
	// Subnet is the allocated subnet CIDR.
	Subnet string `json:"subnet"`
	// Gateway is the gateway address.
	Gateway string `json:"gateway"`
	// Addresses are the configured addresses (via cloud-init).
	Addresses []string `json:"addresses"`
}

// IPv6Manager manages IPv6 subnet allocation for VMs.
// In the L2 bridged architecture, it provides:
//   - Per-VM /64 subnet allocation from the node's assigned /48
//   - EUI-64 address generation for cloud-init configuration
//   - Subnet tracking and release
//
// The node's /48 prefix is assigned by the controller and used for allocation,
// but is NOT configured on the local bridge (upstream router handles routing).
type IPv6Manager struct {
	configDir  string // /var/lib/virtuestack/ipv6
	logger     *slog.Logger
	mu         sync.RWMutex
	nodePrefix string          // Node's allocated /48 prefix (set by controller)
	subnetAlloc map[int]string // index -> vmID mapping for allocation
}

// NewIPv6Manager creates a new IPv6Manager with the given configuration.
func NewIPv6Manager(configDir string, logger *slog.Logger) *IPv6Manager {
	return &IPv6Manager{
		configDir:   configDir,
		logger:      logger.With("component", "ipv6-manager"),
		subnetAlloc: make(map[int]string),
	}
}

// Initialize creates the necessary directories for IPv6 management.
func (m *IPv6Manager) Initialize(ctx context.Context) error {
	logger := m.logger.With("operation", "initialize")
	logger.Info("initializing IPv6 manager")

	dirs := []string{
		m.configDir,
		filepath.Join(m.configDir, "status"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	// Load existing allocations from status files
	if err := m.loadExistingAllocations(ctx); err != nil {
		logger.Warn("failed to load existing allocations", "error", err)
	}

	logger.Info("IPv6 manager initialized successfully")
	return nil
}

// loadExistingAllocations loads existing subnet allocations from status files.
// Must be called with m.mu held if concurrent access is possible; here it is
// called only during Initialize which runs before the manager is shared, but
// we acquire the lock defensively to satisfy the data-race requirement (F-182).
func (m *IPv6Manager) loadExistingAllocations(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	statusDir := filepath.Join(m.configDir, "status")

	entries, err := os.ReadDir(statusDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading status directory: %w", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), IPv6StatusSuffix) {
			continue
		}

		vmID := strings.TrimSuffix(entry.Name(), IPv6StatusSuffix)
		statusPath := filepath.Join(statusDir, entry.Name())

		data, err := os.ReadFile(statusPath)
		if err != nil {
			continue
		}

		var info IPv6Info
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}

		// Parse subnet index from the subnet
		subnet, err := netip.ParsePrefix(info.Subnet)
		if err != nil {
			continue
		}

		index := m.extractSubnetIndex(subnet)
		if index >= 0 {
			m.subnetAlloc[index] = vmID
		}
	}

	return nil
}

// extractSubnetIndex extracts the subnet index from a /64 subnet within a /48.
func (m *IPv6Manager) extractSubnetIndex(subnet netip.Prefix) int {
	if subnet.Bits() != IPv6VMSubnetLen {
		return -1
	}

	addr := subnet.Addr()
	bytes := addr.As16()

	// For a prefix like 2001:db8:0:1234::/64, the index is in bytes 6-7
	// (the 3rd and 4th hextets, which represent the subnet ID within the /48)
	index := int(bytes[6])<<8 | int(bytes[7])
	return index
}

// SetNodePrefix sets the node's /48 prefix for subnet allocation.
// This is called by the controller to assign the node's prefix.
// In the L2 bridged architecture, this prefix is used ONLY for allocation
// calculations - it is NOT configured on the local bridge interface.
func (m *IPv6Manager) SetNodePrefix(prefix string) error {
	// Validate the prefix
	_, err := netip.ParsePrefix(prefix)
	if err != nil {
		return fmt.Errorf("invalid prefix %s: %w", prefix, err)
	}

	// Store the node prefix for allocation
	m.mu.Lock()
	m.nodePrefix = prefix
	m.mu.Unlock()

	m.logger.Info("node prefix set for allocation", "prefix", prefix)
	return nil
}

// AllocateVMSubnet allocates a /64 subnet for a VM from the node's /48 prefix.
// The subnetIndex parameter allows specifying a particular index (0-65535),
// or -1 to auto-allocate the next available subnet.
//
// Returns the allocated subnet info which can be used for cloud-init
// static IPv6 configuration in the VM.
func (m *IPv6Manager) AllocateVMSubnet(ctx context.Context, vmID string, subnetIndex int) (*IPv6Subnet, error) {
	logger := m.logger.With("vm_id", vmID, "operation", "allocate_subnet")
	logger.Info("allocating IPv6 subnet", "requested_index", subnetIndex)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if VM already has an allocation
	for idx, existingVMID := range m.subnetAlloc {
		if existingVMID == vmID {
			// Return existing allocation
			return m.buildSubnetFromIndex(vmID, idx), nil
		}
	}

	// Check if node prefix is configured
	if m.nodePrefix == "" {
		return nil, fmt.Errorf("node prefix not configured: %w", errors.ErrNotFound)
	}

	// Determine the index to use
	var index int
	if subnetIndex >= 0 {
		// Specific index requested
		if subnetIndex >= IPv6MaxSubnets {
			return nil, fmt.Errorf("subnet index %d out of range (max %d)", subnetIndex, IPv6MaxSubnets-1)
		}
		if existing, taken := m.subnetAlloc[subnetIndex]; taken {
			return nil, fmt.Errorf("subnet index %d already allocated to VM %s", subnetIndex, existing)
		}
		index = subnetIndex
	} else {
		// Auto-allocate next available
		index = -1
		for i := 0; i < IPv6MaxSubnets; i++ {
			if _, taken := m.subnetAlloc[i]; !taken {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, fmt.Errorf("no available subnets in node's /48")
		}
	}

	// Allocate the subnet
	m.subnetAlloc[index] = vmID

	subnet := m.buildSubnetFromIndex(vmID, index)
	logger.Info("subnet allocated", "subnet", subnet.Subnet, "gateway", subnet.Gateway, "index", index)

	// Save status
	info := &IPv6Info{
		VMID:      vmID,
		Subnet:    subnet.Subnet,
		Gateway:   subnet.Gateway,
		Addresses: []string{},
	}
	if err := m.saveStatus(vmID, info); err != nil {
		logger.Warn("failed to save IPv6 status", "error", err)
	}

	return subnet, nil
}

// buildSubnetFromIndex creates an IPv6Subnet from an index.
func (m *IPv6Manager) buildSubnetFromIndex(vmID string, index int) *IPv6Subnet {
	// Parse the node prefix
	pfx, _ := netip.ParsePrefix(m.nodePrefix)
	addr := pfx.Addr()

	// Build the /64 subnet by setting the subnet ID (index) in bytes 6-7
	bytes := addr.As16()
	bytes[6] = byte(index >> 8)
	bytes[7] = byte(index)

	// Create the subnet prefix
	subnetAddr := netip.AddrFrom16(bytes)
	subnet := netip.PrefixFrom(subnetAddr, IPv6VMSubnetLen)

	// Create gateway (::1)
	gatewayBytes := bytes
	gatewayBytes[15] = 1
	gatewayAddr := netip.AddrFrom16(gatewayBytes)

	return &IPv6Subnet{
		VMID:      vmID,
		Subnet:    subnet.String(),
		Gateway:   gatewayAddr.String(),
		PrefixLen: IPv6VMSubnetLen,
		Index:     index,
	}
}

// ReleaseVMSubnet releases the allocated subnet for a VM.
func (m *IPv6Manager) ReleaseVMSubnet(ctx context.Context, vmID string) error {
	logger := m.logger.With("vm_id", vmID, "operation", "release_subnet")

	m.mu.Lock()
	defer m.mu.Unlock()

	for idx, existingVMID := range m.subnetAlloc {
		if existingVMID == vmID {
			delete(m.subnetAlloc, idx)
			
			// Clean up status file
			statusPath := m.statusFilePath(vmID)
			if err := os.Remove(statusPath); err != nil && !os.IsNotExist(err) {
				logger.Warn("failed to remove status file", "error", err)
			}
			
			logger.Info("subnet released", "index", idx)
			return nil
		}
	}

	logger.Info("no subnet allocation found for VM")
	return nil
}

// GetVMSubnet retrieves the allocated subnet for a VM.
func (m *IPv6Manager) GetVMSubnet(ctx context.Context, vmID string) (*IPv6Subnet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for idx, existingVMID := range m.subnetAlloc {
		if existingVMID == vmID {
			return m.buildSubnetFromIndex(vmID, idx), nil
		}
	}

	return nil, fmt.Errorf("no subnet allocated for VM %s: %w", vmID, errors.ErrNotFound)
}

// GetVMIPv6Info retrieves the current IPv6 configuration for a VM.
func (m *IPv6Manager) GetVMIPv6Info(ctx context.Context, vmID string) (*IPv6Info, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getVMIPv6InfoUnlocked(vmID)
}

// getVMIPv6InfoUnlocked retrieves IPv6 info without locking (for internal use).
func (m *IPv6Manager) getVMIPv6InfoUnlocked(vmID string) (*IPv6Info, error) {
	statusPath := m.statusFilePath(vmID)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading IPv6 status: %w", err)
	}

	var info IPv6Info
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parsing IPv6 status: %w", err)
	}

	return &info, nil
}

// GenerateEUI64Address generates an EUI-64 IPv6 address from a MAC address.
// The address is formed by combining the subnet prefix with the EUI-64 derived interface ID.
// This is useful for generating predictable IPv6 addresses for cloud-init configuration.
func (m *IPv6Manager) GenerateEUI64Address(subnetCIDR, macAddress string) (string, error) {
	// Parse the subnet
	pfx, err := netip.ParsePrefix(subnetCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid subnet: %w", err)
	}

	// Parse the MAC address
	mac, err := net.ParseMAC(macAddress)
	if err != nil {
		return "", fmt.Errorf("invalid MAC address: %w", err)
	}

	macBytes := mac

	// Insert 0xFFFE in the middle
	eui64 := make([]byte, 8)
	copy(eui64[0:3], macBytes[0:3])
	eui64[3] = 0xFF
	eui64[4] = 0xFE
	copy(eui64[5:8], macBytes[3:6])

	// Flip the 7th bit of the first byte (Universal/Local bit)
	eui64[0] ^= 0x02

	// Combine with prefix
	prefixBytes := pfx.Addr().As16()
	var addrBytes [16]byte
	copy(addrBytes[0:8], prefixBytes[0:8])
	copy(addrBytes[8:16], eui64)

	addr := netip.AddrFrom16(addrBytes)
	return addr.String(), nil
}

// GetNodePrefix returns the configured node prefix.
func (m *IPv6Manager) GetNodePrefix() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.nodePrefix
}

// ListAllocations lists all current subnet allocations.
func (m *IPv6Manager) ListAllocations(ctx context.Context) ([]IPv6Subnet, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var allocations []IPv6Subnet
	for idx, vmID := range m.subnetAlloc {
		allocations = append(allocations, *m.buildSubnetFromIndex(vmID, idx))
	}
	return allocations, nil
}

// Helper methods

// statusFilePath returns the path to the status file for a VM.
func (m *IPv6Manager) statusFilePath(vmID string) string {
	return filepath.Join(m.configDir, "status", vmID+IPv6StatusSuffix)
}

// saveStatus saves the IPv6 status to a file.
func (m *IPv6Manager) saveStatus(vmID string, info *IPv6Info) error {
	statusPath := m.statusFilePath(vmID)
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling IPv6 status: %w", err)
	}
	return os.WriteFile(statusPath, data, 0600)
}