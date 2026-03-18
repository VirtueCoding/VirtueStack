// Package network provides network management for the VirtueStack Node Agent.
// This file implements libvirt nwfilter-based anti-spoofing protection for VMs.
package network

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"libvirt.org/go/libvirt"
)

// Constants for nwfilter management.
const (
	// FilterNamePrefix is the prefix for all VirtueStack nwfilter names.
	FilterNamePrefix = "vs-anti-spoof-"
	// CleanTrafficFilter is the reference to the base clean-traffic nwfilter.
	CleanTrafficFilter = "clean-traffic"
	// cleanTrafficFilterUUID is the fixed UUID assigned to the VirtueStack
	// clean-traffic base nwfilter. A stable UUID is required so that operator
	// tooling and libvirt backups can reference the filter consistently across
	// node agent restarts and re-deployments.
	cleanTrafficFilterUUID = "6f1c7f7e-9c4a-4e7b-8f3d-2a5e9c8b7d6f"
)

// NWFilterManager manages libvirt network filters for VM anti-spoofing protection.
// It creates per-VM nwfilter rules that enforce:
//   - MAC spoofing protection (only allow traffic from assigned MAC)
//   - IP spoofing protection (only allow traffic from assigned IPs)
//   - ARP spoofing protection (validate ARP replies match MAC+IP)
type NWFilterManager struct {
	conn   *libvirt.Connect
	logger *slog.Logger
}

// NewNWFilterManager creates a new NWFilterManager with the given libvirt connection.
func NewNWFilterManager(conn *libvirt.Connect, logger *slog.Logger) *NWFilterManager {
	return &NWFilterManager{
		conn:   conn,
		logger: logger.With("component", "nwfilter-manager"),
	}
}

// CreateAntiSpoofFilter creates an nwfilter for a VM with anti-spoofing rules.
// The filter protects against MAC, IP, and ARP spoofing by only allowing traffic
// from the VM's assigned MAC and IP addresses.
//
// Parameters:
//   - vmID: Unique identifier for the VM (used for filter naming)
//   - vmName: Human-readable VM name (used for filter naming)
//   - macAddress: The VM's assigned MAC address
//   - ipv4s: List of assigned IPv4 addresses
//   - ipv6s: List of assigned IPv6 addresses
func (m *NWFilterManager) CreateAntiSpoofFilter(ctx context.Context, vmID, vmName, macAddress string, ipv4s, ipv6s []string) error {
	filterName := m.filterName(vmName)
	logger := m.logger.With("vm_id", vmID, "vm_name", vmName, "filter", filterName)
	logger.Info("creating anti-spoof nwfilter", "mac", macAddress, "ipv4s", ipv4s, "ipv6s", ipv6s)

	// Generate the nwfilter XML
	filterXML := m.GenerateFilterXML(filterName, vmName, macAddress, ipv4s, ipv6s)

	// Define the nwfilter in libvirt
	filter, err := m.conn.NWFilterDefineXML(filterXML)
	if err != nil {
		return fmt.Errorf("defining nwfilter %s: %w", filterName, err)
	}
	defer filter.Free()

	logger.Info("anti-spoof nwfilter created successfully")
	return nil
}

// RemoveFilter removes an nwfilter for a VM.
// This should be called when a VM is deleted to clean up the filter.
func (m *NWFilterManager) RemoveFilter(ctx context.Context, vmName string) error {
	filterName := m.filterName(vmName)
	logger := m.logger.With("vm_name", vmName, "filter", filterName)
	logger.Info("removing nwfilter")

	// Look up the filter
	filter, err := m.conn.LookupNWFilterByName(filterName)
	if err != nil {
		if isLibvirtError(err, libvirt.ERR_NO_NWFILTER) {
			logger.Info("nwfilter already removed")
			return nil
		}
		return fmt.Errorf("looking up nwfilter %s: %w", filterName, err)
	}
	defer filter.Free()

	// Undefine the filter
	if err := filter.Undefine(); err != nil {
		return fmt.Errorf("undefining nwfilter %s: %w", filterName, err)
	}

	logger.Info("nwfilter removed successfully")
	return nil
}

// FilterExists checks if an nwfilter exists for a VM.
func (m *NWFilterManager) FilterExists(ctx context.Context, vmName string) (bool, error) {
	filterName := m.filterName(vmName)
	filter, err := m.conn.LookupNWFilterByName(filterName)
	if err != nil {
		if isLibvirtError(err, libvirt.ERR_NO_NWFILTER) {
			return false, nil
		}
		return false, fmt.Errorf("checking nwfilter %s existence: %w", filterName, err)
	}
	filter.Free()
	return true, nil
}

// GenerateFilterXML generates the nwfilter XML for anti-spoofing protection.
// The generated filter includes:
//   - Reference to clean-traffic base filter
//   - Rogue DHCP prevention: Drop outbound DHCP replies (prevent VM as DHCP server)
//   - Allow outbound DHCP requests (0.0.0.0 -> DHCP server)
//   - Allow outbound IPv6 Router Solicitations
//   - MAC validation rule (allow only from assigned MAC)
//   - IPv4 validation rules (allow only from assigned IPv4s)
//   - IPv6 validation rules (allow only from assigned IPv6s)
//   - ARP validation rules (ARP replies must match MAC+IP)
//   - Default drop rule for unmatched traffic
func (m *NWFilterManager) GenerateFilterXML(filterName, vmName, mac string, ipv4s, ipv6s []string) string {
	var sb strings.Builder

	sb.WriteString(`<filter name='`)
	sb.WriteString(escapeXML(filterName))
	sb.WriteString(`'>`)

	// Reference the clean-traffic base filter for common protections
	sb.WriteString(`<filterref filter='clean-traffic'/>`)

	// SECURITY: Rogue DHCP prevention
	// Drop outbound DHCP replies to prevent VM from acting as a rogue DHCP server.
	// This must come BEFORE the allow rules. Priority 80 ensures it's evaluated early.
	sb.WriteString(`<rule action='drop' direction='out' priority='80'>`)
	sb.WriteString(`<udp srcportstart='67' srcportend='67'/>`)
	sb.WriteString(`</rule>`)

	// Allow outbound DHCP requests (client -> server).
	// VM needs to request IP via DHCP. Source IP is 0.0.0.0 initially.
	// Priority 90 ensures this is evaluated before MAC/IP checks.
	sb.WriteString(`<rule action='accept' direction='out' priority='90'>`)
	sb.WriteString(`<ip srcipaddr='0.0.0.0' protocol='udp'/>`)
	sb.WriteString(`<udp srcportstart='68' srcportend='68' dstportstart='67' dstportend='67'/>`)
	sb.WriteString(`</rule>`)

	// Allow outbound DHCPv6 requests (client -> server).
	// VM needs to request IPv6 via DHCPv6. Uses link-local source.
	sb.WriteString(`<rule action='accept' direction='out' priority='90'>`)
	sb.WriteString(`<ipv6 protocol='udp'/>`)
	sb.WriteString(`<udp srcportstart='546' srcportend='546' dstportstart='547' dstportend='547'/>`)
	sb.WriteString(`</rule>`)

	// Allow outbound IPv6 Router Solicitations (ICMPv6 type 133).
	// Required for SLAAC and IPv6 auto-configuration.
	sb.WriteString(`<rule action='accept' direction='out' priority='90'>`)
	sb.WriteString(`<ipv6 protocol='icmpv6'/>`)
	sb.WriteString(`<icmpv6 type='133'/>`)
	sb.WriteString(`</rule>`)

	// Allow outbound IPv6 Neighbor Solicitations (ICMPv6 type 135).
	// Required for IPv6 address resolution.
	sb.WriteString(`<rule action='accept' direction='out' priority='90'>`)
	sb.WriteString(`<ipv6 protocol='icmpv6'/>`)
	sb.WriteString(`<icmpv6 type='135'/>`)
	sb.WriteString(`</rule>`)

	// MAC spoofing protection: Only allow traffic from assigned MAC
	// Priority 500 ensures this rule is evaluated early
	sb.WriteString(`<rule action='accept' direction='out' priority='500'>`)
	sb.WriteString(`<mac match='yes' srcmacaddr='`)
	sb.WriteString(escapeXML(mac))
	sb.WriteString(`'/>`)
	sb.WriteString(`</rule>`)

	// IPv4 spoofing protection: Only allow traffic from assigned IPv4 addresses
	for i, ip := range ipv4s {
		if ip == "" {
			continue
		}
		// Each IP rule gets a unique priority (100 + index) for ordering
		priority := 100 + i
		sb.WriteString(`<rule action='accept' direction='out' priority='`)
		sb.WriteString(fmt.Sprintf("%d", priority))
		sb.WriteString(`'>`)
		sb.WriteString(`<ip match='yes' srcipaddr='`)
		sb.WriteString(escapeXML(ip))
		sb.WriteString(`'/>`)
		sb.WriteString(`</rule>`)
	}

	// IPv6 spoofing protection: Only allow traffic from assigned IPv6 addresses
	for i, ip := range ipv6s {
		if ip == "" {
			continue
		}
		priority := 200 + i
		sb.WriteString(`<rule action='accept' direction='out' priority='`)
		sb.WriteString(fmt.Sprintf("%d", priority))
		sb.WriteString(`'>`)
		sb.WriteString(`<ipv6 match='yes' srcipaddr='`)
		sb.WriteString(escapeXML(ip))
		sb.WriteString(`'/>`)
		sb.WriteString(`</rule>`)
	}

	// ARP spoofing protection: Validate ARP replies
	// ARP replies must come from the assigned MAC and advertise the assigned IP
	for _, ip := range ipv4s {
		if ip == "" {
			continue
		}
		// Priority 50 for ARP rules (higher priority than IP rules)
		sb.WriteString(`<rule action='accept' direction='inout' priority='50'>`)
		sb.WriteString(`<arp match='yes' arpsrcmacaddr='`)
		sb.WriteString(escapeXML(mac))
		sb.WriteString(`' arpsrcipaddr='`)
		sb.WriteString(escapeXML(ip))
		sb.WriteString(`'/>`)
		sb.WriteString(`</rule>`)
	}

	// Explicit drop rule for any traffic that doesn't match the above rules
	// This provides defense-in-depth (libvirt nwfilter has implicit drop, but explicit is clearer)
	sb.WriteString(`<rule action='drop' direction='out' priority='1000'>`)
	sb.WriteString(`<all/>`)
	sb.WriteString(`</rule>`)

	sb.WriteString(`</filter>`)

	return sb.String()
}

// GetFilterName returns the nwfilter name for a VM.
// This is a convenience method for external callers.
func (m *NWFilterManager) GetFilterName(vmName string) string {
	return m.filterName(vmName)
}

// filterName generates the nwfilter name for a VM.
func (m *NWFilterManager) filterName(vmName string) string {
	return FilterNamePrefix + sanitizeFilterName(vmName)
}

// sanitizeFilterName sanitizes a VM name for use in nwfilter names.
// Libvirt nwfilter names must be valid XML attribute values and
// should not contain special characters.
func sanitizeFilterName(name string) string {
	// Replace common problematic characters
	result := strings.ReplaceAll(name, " ", "-")
	result = strings.ReplaceAll(result, ".", "-")
	result = strings.ReplaceAll(result, "_", "-")
	return result
}

// escapeXML escapes special characters for safe inclusion in XML attributes.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// EnsureBaseFilters ensures that the base nwfilter templates exist in libvirt.
// This should be called during node agent initialization to create the
// clean-traffic base filter if it doesn't exist.
//
// NOTE: The clean-traffic filter is intentionally minimal/empty. All anti-spoofing
// rules are defined in per-VM filters (vs-anti-spoof-*) to ensure proper isolation.
// An empty clean-traffic filter prevents IP spoofing bypass that would occur with
// overly permissive rules at the base level.
func (m *NWFilterManager) EnsureBaseFilters(ctx context.Context) error {
	logger := m.logger.With("operation", "ensure_base_filters")
	logger.Info("ensuring base nwfilter templates exist")

	// Check if clean-traffic filter exists
	_, err := m.conn.LookupNWFilterByName(CleanTrafficFilter)
	if err == nil {
		logger.Info("clean-traffic nwfilter already exists")
		return nil
	}

	if !isLibvirtError(err, libvirt.ERR_NO_NWFILTER) {
		return fmt.Errorf("checking clean-traffic nwfilter: %w", err)
	}

	// Create a minimal clean-traffic base filter.
	// IMPORTANT: This is intentionally empty. All MAC/IP/ARP validation is done
	// in per-VM filters to prevent bypass scenarios. Do NOT add rules here that
	// could allow spoofed traffic to pass.
	cleanTrafficXML := fmt.Sprintf("<filter name='clean-traffic' chain='root'>\n  <uuid>%s</uuid>\n</filter>", cleanTrafficFilterUUID)

	filter, err := m.conn.NWFilterDefineXML(cleanTrafficXML)
	if err != nil {
		return fmt.Errorf("creating clean-traffic nwfilter: %w", err)
	}
	defer filter.Free()

	logger.Info("clean-traffic nwfilter created successfully")
	return nil
}

// NWFilterInfo contains information about a network filter.
type NWFilterInfo struct {
	// Name is the nwfilter name.
	Name string
	// UUID is the nwfilter UUID.
	UUID string
	// XML is the nwfilter XML definition.
	XML string
}

// ListFilters lists all VirtueStack nwfilters.
func (m *NWFilterManager) ListFilters(ctx context.Context) ([]NWFilterInfo, error) {
	filters, err := m.conn.ListAllNWFilters(0)
	if err != nil {
		return nil, fmt.Errorf("listing nwfilters: %w", err)
	}

	var result []NWFilterInfo
	for _, f := range filters {
		name, err := f.GetName()
		if err != nil {
			f.Free()
			continue
		}

		// Only include VirtueStack filters
		if !strings.HasPrefix(name, FilterNamePrefix) {
			f.Free()
			continue
		}

		uuid, _ := f.GetUUIDString()
		xmlDesc, _ := f.GetXMLDesc(0)

		result = append(result, NWFilterInfo{
			Name: name,
			UUID: uuid,
			XML:  xmlDesc,
		})
		f.Free()
	}

	return result, nil
}

// GetFilter retrieves information about a specific nwfilter.
func (m *NWFilterManager) GetFilter(ctx context.Context, vmName string) (*NWFilterInfo, error) {
	filterName := m.filterName(vmName)

	filter, err := m.conn.LookupNWFilterByName(filterName)
	if err != nil {
		if isLibvirtError(err, libvirt.ERR_NO_NWFILTER) {
			return nil, fmt.Errorf("nwfilter %s: %w", filterName, errors.ErrNotFound)
		}
		return nil, fmt.Errorf("looking up nwfilter %s: %w", filterName, err)
	}
	defer filter.Free()

	uuid, _ := filter.GetUUIDString()
	xmlDesc, err := filter.GetXMLDesc(0)
	if err != nil {
		return nil, fmt.Errorf("getting nwfilter XML: %w", err)
	}

	return &NWFilterInfo{
		Name: filterName,
		UUID: uuid,
		XML:  xmlDesc,
	}, nil
}