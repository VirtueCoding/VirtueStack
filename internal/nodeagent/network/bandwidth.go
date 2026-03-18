// Package network provides network management for the VirtueStack Node Agent.
// This file implements bandwidth tracking and throttling for VMs.
package network

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"

	"github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/libvirtutil"
	"libvirt.org/go/libvirt"
)

// validIfaceRegex validates interface names to prevent command injection.
// Interface names should only contain alphanumeric characters and underscores.
var validIfaceRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// Constants for bandwidth management.
const (
	// DefaultThrottleRateKbps is the default throttle rate (5 Mbps = 5000 Kbps).
	DefaultThrottleRateKbps = 5000
	// DefaultBurstKB is the default burst allowance in kilobytes.
	DefaultBurstKB = 32
	// DefaultLatencyMs is the default latency for queued packets.
	DefaultLatencyMs = 400
)

// ThrottleConfig contains configuration for bandwidth throttling via tc.
type ThrottleConfig struct {
	// RateKbps is the throttle rate in kilobits per second.
	RateKbps int
	// BurstKB is the burst allowance in kilobytes.
	BurstKB int
	// LatencyMs is the maximum latency for queued packets.
	LatencyMs int
}

// DefaultThrottleConfig returns the default throttle configuration.
func DefaultThrottleConfig() ThrottleConfig {
	return ThrottleConfig{
		RateKbps:  DefaultThrottleRateKbps,
		BurstKB:   DefaultBurstKB,
		LatencyMs: DefaultLatencyMs,
	}
}

// NetworkStats contains network statistics for a VM interface.
type NetworkStats struct {
	// InterfaceName is the name of the network interface (e.g., vnet0).
	InterfaceName string
	// BytesIn is the total bytes received.
	BytesIn uint64
	// BytesOut is the total bytes transmitted.
	BytesOut uint64
	// PacketsIn is the total packets received.
	PacketsIn uint64
	// PacketsOut is the total packets transmitted.
	PacketsOut uint64
}

// NodeBandwidthManager manages bandwidth tracking and throttling for VMs on a node.
// It uses libvirt to read network statistics and tc (traffic control) to apply
// bandwidth throttling.
type NodeBandwidthManager struct {
	conn   *libvirt.Connect
	logger *slog.Logger
}

// NewNodeBandwidthManager creates a new NodeBandwidthManager with the given libvirt connection.
func NewNodeBandwidthManager(conn *libvirt.Connect, logger *slog.Logger) *NodeBandwidthManager {
	return &NodeBandwidthManager{
		conn:   conn,
		logger: logger.With("component", "bandwidth-manager"),
	}
}

// GetVMNetworkStats retrieves network statistics for a VM's interfaces.
// It returns aggregated stats across all network interfaces.
func (m *NodeBandwidthManager) GetVMNetworkStats(ctx context.Context, vmName string) (bytesIn, bytesOut uint64, err error) {
	logger := m.logger.With("vm_name", vmName, "operation", "get_network_stats")

	domain, err := m.lookupDomain(vmName)
	if err != nil {
		return 0, 0, err
	}
	defer domain.Free()

	// Check if running
	state, _, err := domain.GetState()
	if err != nil {
		return 0, 0, fmt.Errorf("getting domain state: %w", err)
	}

	if state != libvirt.DOMAIN_RUNNING {
		logger.Debug("VM not running, returning zero stats")
		return 0, 0, nil
	}

	// Get interface names from domain XML
	ifaces, err := m.getInterfaceNames(domain)
	if err != nil {
		return 0, 0, fmt.Errorf("getting interface names: %w", err)
	}

	// Aggregate stats from all interfaces
	for _, ifaceName := range ifaces {
		stats, err := domain.InterfaceStats(ifaceName)
		if err != nil {
			logger.Warn("failed to get interface stats", "interface", ifaceName, "error", err)
			continue
		}
		bytesIn += uint64(stats.RxBytes)
		bytesOut += uint64(stats.TxBytes)
	}

	logger.Debug("retrieved network stats", "bytes_in", bytesIn, "bytes_out", bytesOut, "interface_count", len(ifaces))
	return bytesIn, bytesOut, nil
}

// GetAllInterfaceStats retrieves detailed network statistics for all VM interfaces.
func (m *NodeBandwidthManager) GetAllInterfaceStats(ctx context.Context, vmName string) ([]NetworkStats, error) {
	domain, err := m.lookupDomain(vmName)
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
		return nil, nil
	}

	// Get interface names from domain XML
	ifaces, err := m.getInterfaceNames(domain)
	if err != nil {
		return nil, fmt.Errorf("getting interface names: %w", err)
	}

	var stats []NetworkStats
	for _, ifaceName := range ifaces {
		ifStats, err := domain.InterfaceStats(ifaceName)
		if err != nil {
			m.logger.Warn("failed to get interface stats", "interface", ifaceName, "error", err)
			continue
		}

		stats = append(stats, NetworkStats{
			InterfaceName: ifaceName,
			BytesIn:       uint64(ifStats.RxBytes),
			BytesOut:      uint64(ifStats.TxBytes),
			PacketsIn:     uint64(ifStats.RxPackets),
			PacketsOut:    uint64(ifStats.TxPackets),
		})
	}

	return stats, nil
}

// ApplyThrottle applies bandwidth throttling to a VM's network interface.
// Uses tc (traffic control) with tbf (Token Bucket Filter) to limit bandwidth.
// The minimum throttle rate is 5 Mbps to ensure the VM remains usable.
func (m *NodeBandwidthManager) ApplyThrottle(ctx context.Context, vmName string, rateKbps int) error {
	logger := m.logger.With("vm_name", vmName, "operation", "apply_throttle", "rate_kbps", rateKbps)

	// Ensure minimum throttle rate
	if rateKbps < DefaultThrottleRateKbps {
		rateKbps = DefaultThrottleRateKbps
		logger.Debug("rate below minimum, using minimum", "min_rate_kbps", DefaultThrottleRateKbps)
	}

	// Get the interface name
	ifaceName, err := m.GetInterfaceName(ctx, vmName)
	if err != nil {
		return fmt.Errorf("getting interface name: %w", err)
	}

	// Validate interface name to prevent command injection
	if !validIfaceRegex.MatchString(ifaceName) {
		return fmt.Errorf("invalid interface name for throttling: %s", ifaceName)
	}

	logger = logger.With("interface", ifaceName)

	// Check if already throttled
	throttled, err := m.isThrottled(ifaceName)
	if err != nil {
		logger.Warn("failed to check throttle status", "error", err)
	}

	if throttled {
		// Remove existing throttle first
		if err := m.removeThrottleFromInterface(ifaceName); err != nil {
			logger.Warn("failed to remove existing throttle", "error", err)
		}
	}

	// Apply new throttle using tc
	cfg := DefaultThrottleConfig()
	cfg.RateKbps = rateKbps

	// tc qdisc add dev vnet0 root tbf rate 5mbit burst 32kbit latency 400ms
	// Note: tc uses "kbit" for kilobits, "mbit" for megabits
	rateStr := fmt.Sprintf("%dkbit", rateKbps)
	burstStr := fmt.Sprintf("%dkbit", cfg.BurstKB)
	latencyStr := fmt.Sprintf("%dms", cfg.LatencyMs)

	cmd := exec.CommandContext(ctx, "tc", "qdisc", "add", "dev", ifaceName,
		"root", "tbf", "rate", rateStr, "burst", burstStr, "latency", latencyStr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("applying tc throttle: %w, output: %s", err, string(output))
	}

	logger.Info("throttle applied successfully")
	return nil
}

// RemoveThrottle removes bandwidth throttling from a VM's network interface.
func (m *NodeBandwidthManager) RemoveThrottle(ctx context.Context, vmName string) error {
	logger := m.logger.With("vm_name", vmName, "operation", "remove_throttle")

	// Get the interface name
	ifaceName, err := m.GetInterfaceName(ctx, vmName)
	if err != nil {
		return fmt.Errorf("getting interface name: %w", err)
	}

	// Validate interface name to prevent command injection
	if !validIfaceRegex.MatchString(ifaceName) {
		return fmt.Errorf("invalid interface name for throttling: %s", ifaceName)
	}

	logger = logger.With("interface", ifaceName)

	// Check if throttled
	throttled, err := m.isThrottled(ifaceName)
	if err != nil {
		logger.Warn("failed to check throttle status", "error", err)
	}

	if !throttled {
		logger.Debug("interface not throttled, nothing to remove")
		return nil
	}

	// Remove throttle
	if err := m.removeThrottleFromInterface(ifaceName); err != nil {
		return err
	}

	logger.Info("throttle removed successfully")
	return nil
}

// ListThrottledVMs returns a list of VMs that are currently bandwidth-throttled.
// This checks the tc qdisc configuration for each VM's interface.
func (m *NodeBandwidthManager) ListThrottledVMs(ctx context.Context) ([]string, error) {
	// Get all active domains
	domains, err := m.conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE)
	if err != nil {
		return nil, fmt.Errorf("listing active domains: %w", err)
	}

	var throttledVMs []string
	for _, domain := range domains {
		name, err := domain.GetName()
		if err != nil {
			domain.Free()
			continue
		}

		// Check if throttled
		ifaceName, err := m.getInterfaceNameFromDomain(&domain)
		if err != nil {
			domain.Free()
			continue
		}

		throttled, err := m.isThrottled(ifaceName)
		if err != nil {
			m.logger.Warn("failed to check throttle status", "vm_name", name, "error", err)
		}

		if throttled {
			throttledVMs = append(throttledVMs, name)
		}

		domain.Free()
	}

	return throttledVMs, nil
}

// GetInterfaceName retrieves the network interface name (e.g., vnet0) for a VM.
// This is parsed from the domain XML.
func (m *NodeBandwidthManager) GetInterfaceName(ctx context.Context, vmName string) (string, error) {
	domain, err := m.lookupDomain(vmName)
	if err != nil {
		return "", err
	}
	defer domain.Free()

	return m.getInterfaceNameFromDomain(domain)
}

// getInterfaceNameFromDomain extracts the interface name from a domain.
func (m *NodeBandwidthManager) getInterfaceNameFromDomain(domain *libvirt.Domain) (string, error) {
	xmlDesc, err := domain.GetXMLDesc(0)
	if err != nil {
		return "", fmt.Errorf("getting domain XML: %w", err)
	}

	names, err := libvirtutil.GetInterfaceNames(xmlDesc)
	if err != nil {
		return "", fmt.Errorf("parsing domain XML: %w", err)
	}

	if len(names) == 0 {
		return "", fmt.Errorf("no network interface found in domain XML")
	}

	// Return first interface (primary)
	return names[0], nil
}

// getInterfaceNames extracts all interface names from a domain.
func (m *NodeBandwidthManager) getInterfaceNames(domain *libvirt.Domain) ([]string, error) {
	xmlDesc, err := domain.GetXMLDesc(0)
	if err != nil {
		return nil, fmt.Errorf("getting domain XML: %w", err)
	}

	names, err := libvirtutil.GetInterfaceNames(xmlDesc)
	if err != nil {
		return nil, fmt.Errorf("parsing domain XML: %w", err)
	}

	if len(names) == 0 {
		return nil, fmt.Errorf("no network interfaces found in domain XML")
	}

	return names, nil
}

// isThrottled checks if an interface has a tc tbf qdisc configured.
func (m *NodeBandwidthManager) isThrottled(ifaceName string) (bool, error) {
	// Validate interface name to prevent command injection
	if !validIfaceRegex.MatchString(ifaceName) {
		return false, fmt.Errorf("invalid interface name for throttling: %s", ifaceName)
	}

	// tc qdisc show dev vnet0
	// context.TODO() is used because isThrottled is an internal helper that does not
	// yet accept a context; callers should pass a context in a future refactor.
	cmd := exec.CommandContext(context.TODO(), "tc", "qdisc", "show", "dev", ifaceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("checking tc qdisc: %w, output: %s", err, string(output))
	}

	// Check if tbf is in the output
	return strings.Contains(string(output), "tbf"), nil
}

// removeThrottleFromInterface removes the tc qdisc from an interface.
func (m *NodeBandwidthManager) removeThrottleFromInterface(ifaceName string) error {
	// Validate interface name to prevent command injection
	if !validIfaceRegex.MatchString(ifaceName) {
		return fmt.Errorf("invalid interface name for throttling: %s", ifaceName)
	}

	// tc qdisc del dev vnet0 root
	// context.TODO() is used because removeThrottleFromInterface is an internal helper
	// that does not yet accept a context; callers should pass a context in a future refactor.
	cmd := exec.CommandContext(context.TODO(), "tc", "qdisc", "del", "dev", ifaceName, "root")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore error if qdisc doesn't exist
		if !strings.Contains(string(output), "No such file or directory") &&
			!strings.Contains(string(output), "Cannot delete qdisc") {
			return fmt.Errorf("removing tc qdisc: %w, output: %s", err, string(output))
		}
	}
	return nil
}

// lookupDomain looks up a domain by VM name (domain name).
func (m *NodeBandwidthManager) lookupDomain(vmName string) (*libvirt.Domain, error) {
	domain, err := m.conn.LookupDomainByName(vmName)
	if err != nil {
		if libvirtutil.IsLibvirtError(err, libvirt.ERR_NO_DOMAIN) {
			return nil, fmt.Errorf("VM %s: %w", vmName, errors.ErrNotFound)
		}
		return nil, fmt.Errorf("looking up domain %s: %w", vmName, err)
	}
	return domain, nil
}

// GetThrottleStatus returns the current throttle configuration for an interface.
// Returns nil if not throttled.
func (m *NodeBandwidthManager) GetThrottleStatus(ctx context.Context, vmName string) (*ThrottleConfig, error) {
	// Get the interface name
	ifaceName, err := m.GetInterfaceName(ctx, vmName)
	if err != nil {
		return nil, fmt.Errorf("getting interface name: %w", err)
	}

	// Validate interface name to prevent command injection
	if !validIfaceRegex.MatchString(ifaceName) {
		return nil, fmt.Errorf("invalid interface name for throttling: %s", ifaceName)
	}

	// Check if throttled
	throttled, err := m.isThrottled(ifaceName)
	if err != nil {
		return nil, err
	}

	if !throttled {
		return nil, nil
	}

	// Parse tc output to get current rate
	cmd := exec.CommandContext(ctx, "tc", "qdisc", "show", "dev", ifaceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("getting tc qdisc info: %w", err)
	}

	// Parse output like: "qdisc tbf 8001: root rate 5Mbit burst 32Kb lat 400.0ms"
	// This is a simplified parse - production code would need more robust parsing
	outputStr := string(output)
	cfg := DefaultThrottleConfig()

	// Extract rate
	if strings.Contains(outputStr, "rate") {
		parts := strings.Fields(outputStr)
		for i, part := range parts {
			if part == "rate" && i+1 < len(parts) {
				rateStr := parts[i+1]
				// Parse rate string like "5Mbit" or "5000Kbit"
				if strings.HasSuffix(rateStr, "Mbit") {
					var mbits int
					fmt.Sscanf(rateStr, "%dMbit", &mbits)
					cfg.RateKbps = mbits * 1000
				} else if strings.HasSuffix(rateStr, "Kbit") {
					fmt.Sscanf(rateStr, "%dKbit", &cfg.RateKbps)
				}
				break
			}
		}
	}

	return &cfg, nil
}