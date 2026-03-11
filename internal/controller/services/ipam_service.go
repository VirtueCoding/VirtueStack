// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// IPv6CooldownPeriod is the time an IPv4 address remains in cooldown after release.
const IPv6CooldownPeriod = 5 * time.Minute

// IPAMService provides IP Address Management business logic for VirtueStack.
// It handles IPv4 allocation/release, IPv6 subnet assignment, and reverse DNS management.
type IPAMService struct {
	ipRepo   *repository.IPRepository
	nodeRepo *repository.NodeRepository
	logger   *slog.Logger
}

// NewIPAMService creates a new IPAMService with the given dependencies.
func NewIPAMService(
	ipRepo *repository.IPRepository,
	nodeRepo *repository.NodeRepository,
	logger *slog.Logger,
) *IPAMService {
	return &IPAMService{
		ipRepo:   ipRepo,
		nodeRepo: nodeRepo,
		logger:   logger.With("component", "ipam-service"),
	}
}

// AllocateIPv4 allocates an available IPv4 address from an IP set in the specified location.
// The IP is assigned to the VM and marked as primary if it's the first IP for the VM.
func (s *IPAMService) AllocateIPv4(ctx context.Context, vmID, customerID, locationID string) (*models.IPAddress, error) {
	// Find IP set for the location
	ipSets, _, err := s.ipRepo.ListIPSets(ctx, repository.IPSetListFilter{
		LocationID: &locationID,
		IPVersion:  ptrInt16(4),
	})
	if err != nil {
		return nil, fmt.Errorf("finding IP set for location %s: %w", locationID, err)
	}
	if len(ipSets) == 0 {
		return nil, fmt.Errorf("no IPv4 IP set found for location %s", locationID)
	}

	// Use the first available IP set
	ipSet := ipSets[0]

	// Allocate an IP from the set
	ip, err := s.ipRepo.AllocateIPv4(ctx, ipSet.ID, vmID, customerID)
	if err != nil {
		return nil, fmt.Errorf("allocating IPv4 from set %s: %w", ipSet.ID, err)
	}

	s.logger.Info("IPv4 allocated",
		"ip_id", ip.ID,
		"address", ip.Address,
		"vm_id", vmID,
		"customer_id", customerID,
		"ip_set_id", ipSet.ID)

	return ip, nil
}

// ReleaseIPv4 releases an IPv4 address and puts it into cooldown for reuse.
// After the cooldown period, the IP becomes available for allocation again.
func (s *IPAMService) ReleaseIPv4(ctx context.Context, ipID string) error {
	// Verify the IP exists and is assigned
	ip, err := s.ipRepo.GetIPAddressByID(ctx, ipID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("IP address %s not found", ipID)
		}
		return fmt.Errorf("getting IP address %s: %w", ipID, err)
	}

	if ip.Status != models.IPStatusAssigned {
		return fmt.Errorf("IP address %s is not currently assigned (status: %s)", ipID, ip.Status)
	}

	// Release the IP (sets status to cooldown)
	if err := s.ipRepo.ReleaseIPv4(ctx, ipID); err != nil {
		return fmt.Errorf("releasing IPv4 %s: %w", ipID, err)
	}

	s.logger.Info("IPv4 released",
		"ip_id", ipID,
		"address", ip.Address,
		"cooldown_until", time.Now().Add(IPv6CooldownPeriod))

	return nil
}

// AllocateIPv6 allocates a /64 IPv6 subnet from a node's /48 prefix.
// Each VM gets its own /64 subnet with a gateway address.
// The subnet index is tracked per-prefix to ensure unique allocation.
func (s *IPAMService) AllocateIPv6(ctx context.Context, vmID, customerID, nodeID string) (*models.VMIPv6Subnet, error) {
	// Get the node's IPv6 prefix
	prefix, err := s.ipRepo.GetIPv6PrefixByNode(ctx, nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("no IPv6 prefix assigned to node %s", nodeID)
		}
		return nil, fmt.Errorf("getting IPv6 prefix for node %s: %w", nodeID, err)
	}

	// Get all existing subnets for this prefix to find the next available index
	existingSubnets, err := s.ipRepo.GetVMIPv6SubnetsByPrefix(ctx, prefix.ID)
	if err != nil {
		return nil, fmt.Errorf("checking existing IPv6 subnets for prefix: %w", err)
	}

	// Calculate the next subnet index (find max and increment)
	nextIndex := 0
	for _, sub := range existingSubnets {
		if sub.SubnetIndex >= nextIndex {
			nextIndex = sub.SubnetIndex + 1
		}
	}

	// Check we haven't exhausted the /48 prefix (65536 possible /64 subnets)
	if nextIndex >= 65536 {
		return nil, fmt.Errorf("IPv6 prefix %s is exhausted (no available /64 subnets)", prefix.Prefix)
	}

	// Generate the /64 subnet from the /48 prefix
	subnet, gateway, err := generateIPv6Subnet(prefix.Prefix, nextIndex)
	if err != nil {
		return nil, fmt.Errorf("generating IPv6 subnet: %w", err)
	}

	// Create the subnet record
	vmSubnet := &models.VMIPv6Subnet{
		ID:           uuid.New().String(),
		VMID:         vmID,
		IPv6PrefixID: prefix.ID,
		Subnet:       subnet,
		SubnetIndex:  nextIndex,
		Gateway:      gateway,
	}

	if err := s.ipRepo.CreateVMIPv6Subnet(ctx, vmSubnet); err != nil {
		return nil, fmt.Errorf("creating IPv6 subnet: %w", err)
	}

	s.logger.Info("IPv6 subnet allocated",
		"subnet_id", vmSubnet.ID,
		"subnet", subnet,
		"gateway", gateway,
		"vm_id", vmID,
		"customer_id", customerID,
		"node_id", nodeID,
		"subnet_index", nextIndex)

	return vmSubnet, nil
}

// ReleaseIPv6 releases an IPv6 subnet assignment.
// The subnet becomes available for reuse (subnet indexes may have gaps).
func (s *IPAMService) ReleaseIPv6(ctx context.Context, subnetID string) error {
	// Get the subnet to verify it exists and log details
	subnet, err := s.ipRepo.GetVMIPv6SubnetByID(ctx, subnetID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("IPv6 subnet %s not found", subnetID)
		}
		return fmt.Errorf("getting IPv6 subnet %s: %w", subnetID, err)
	}

	// Delete the subnet
	if err := s.ipRepo.DeleteVMIPv6SubnetByID(ctx, subnetID); err != nil {
		return fmt.Errorf("releasing IPv6 subnet %s: %w", subnetID, err)
	}

	s.logger.Info("IPv6 subnet released",
		"subnet_id", subnetID,
		"subnet", subnet.Subnet,
		"vm_id", subnet.VMID)

	return nil
}

// SetRDNS sets the reverse DNS hostname for an IP address.
func (s *IPAMService) SetRDNS(ctx context.Context, ipID, hostname string) error {
	// Verify the IP exists
	_, err := s.ipRepo.GetIPAddressByID(ctx, ipID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("IP address %s not found", ipID)
		}
		return fmt.Errorf("getting IP address %s: %w", ipID, err)
	}

	// Set the RDNS hostname
	if err := s.ipRepo.SetRDNS(ctx, ipID, hostname); err != nil {
		return fmt.Errorf("setting RDNS for IP %s: %w", ipID, err)
	}

	s.logger.Info("RDNS set",
		"ip_id", ipID,
		"hostname", hostname)

	return nil
}

// GetRDNS returns the reverse DNS hostname for an IP address.
// Returns an empty string if no RDNS is set.
func (s *IPAMService) GetRDNS(ctx context.Context, ipID string) (string, error) {
	hostname, err := s.ipRepo.GetRDNS(ctx, ipID)
	if err != nil {
		return "", fmt.Errorf("getting RDNS for IP %s: %w", ipID, err)
	}

	return hostname, nil
}

// GetPrimaryIPv4 returns the primary IPv4 address for a VM.
// Returns ErrNotFound if the VM has no primary IPv4 address.
func (s *IPAMService) GetPrimaryIPv4(ctx context.Context, vmID string) (*models.IPAddress, error) {
	// List all IPs for the VM
	ips, _, err := s.ipRepo.ListIPAddresses(ctx, repository.IPAddressListFilter{
		VMID: &vmID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing IP addresses for VM %s: %w", vmID, err)
	}

	// Find the primary IPv4
	for i := range ips {
		if ips[i].IsPrimary && ips[i].IPVersion == 4 {
			return &ips[i], nil
		}
	}

	return nil, fmt.Errorf("no primary IPv4 address found for VM %s: %w", vmID, sharederrors.ErrNotFound)
}

// ListVMAddresses returns all IP addresses assigned to a VM.
func (s *IPAMService) ListVMAddresses(ctx context.Context, vmID string) ([]models.IPAddress, error) {
	ips, _, err := s.ipRepo.ListIPAddresses(ctx, repository.IPAddressListFilter{
		VMID: &vmID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing IP addresses for VM %s: %w", vmID, err)
	}

	return ips, nil
}

// ReleaseIPsByVM releases all IP addresses assigned to a VM.
// This is used during VM deletion to free up all assigned IPs.
func (s *IPAMService) ReleaseIPsByVM(ctx context.Context, vmID string) error {
	// Get all IPs for the VM
	ips, _, err := s.ipRepo.ListIPAddresses(ctx, repository.IPAddressListFilter{
		VMID: &vmID,
	})
	if err != nil {
		return fmt.Errorf("listing IPs for VM %s: %w", vmID, err)
	}

	// Release each IP
	var releaseErrors []error
	for _, ip := range ips {
		if ip.Status == models.IPStatusAssigned {
			if err := s.ipRepo.ReleaseIPv4(ctx, ip.ID); err != nil {
				releaseErrors = append(releaseErrors,
					fmt.Errorf("releasing IP %s: %w", ip.ID, err))
			} else {
				s.logger.Info("IP released during VM cleanup",
					"ip_id", ip.ID,
					"address", ip.Address,
					"vm_id", vmID)
			}
		}
	}

	// Also release IPv6 subnets
	if err := s.ipRepo.DeleteVMIPv6SubnetsByVM(ctx, vmID); err != nil {
		s.logger.Warn("failed to release IPv6 subnets during VM cleanup",
			"vm_id", vmID,
			"error", err)
	}

	if len(releaseErrors) > 0 {
		return fmt.Errorf("errors releasing IPs for VM %s: %v", vmID, releaseErrors)
	}

	return nil
}

// GetIPsByVM returns all IP addresses assigned to a VM.
// This implements the IPAMService interface used by VMService.
func (s *IPAMService) GetIPsByVM(ctx context.Context, vmID string) ([]models.IPAddress, error) {
	return s.ListVMAddresses(ctx, vmID)
}

// GetIPv6SubnetsByVM returns all IPv6 subnets assigned to a VM.
func (s *IPAMService) GetIPv6SubnetsByVM(ctx context.Context, vmID string) ([]models.VMIPv6Subnet, error) {
	subnets, err := s.ipRepo.GetVMIPv6SubnetsByVM(ctx, vmID)
	if err != nil {
		return nil, fmt.Errorf("getting IPv6 subnets for VM %s: %w", vmID, err)
	}
	return subnets, nil
}

// Helper functions

// ptrInt16 returns a pointer to an int16 value.
func ptrInt16(v int16) *int16 {
	return &v
}

// generateIPv6Subnet generates a /64 subnet and gateway from a /48 prefix.
// The subnetIndex determines which /64 block is used from the /48.
func generateIPv6Subnet(prefix48 string, subnetIndex int) (subnet, gateway string, err error) {
	// Parse the /48 prefix
	prefix, err := netip.ParsePrefix(prefix48)
	if err != nil {
		return "", "", fmt.Errorf("parsing prefix %s: %w", prefix48, err)
	}

	// Get the prefix address
	addr := prefix.Addr()

	// Convert to 16-byte array
	bytes := addr.As16()

	// The subnet index goes in bits 48-63 of the address (the next 16 bits after the /48 prefix)
	// We need to modify bytes 6 and 7 (indices 6 and 7 in the 16-byte array)
	// Bytes 6-7 contain bits 48-63
	byte6 := (subnetIndex >> 8) & 0xFF
	byte7 := subnetIndex & 0xFF

	bytes[6] = byte(byte6)
	bytes[7] = byte(byte7)

	// Create the new address
	newAddr := netip.AddrFrom16(bytes)

	// Create /64 subnet
	subnetAddr := netip.PrefixFrom(newAddr, 64)
	if !subnetAddr.IsValid() {
		return "", "", fmt.Errorf("invalid subnet generated")
	}

	subnet = subnetAddr.String()

	// Gateway is typically the first address in the subnet (::1)
	gatewayBytes := newAddr.As16()
	gatewayBytes[15] = 1 // Set last byte to 1 for ::1
	gatewayAddr := netip.AddrFrom16(gatewayBytes)
	gateway = gatewayAddr.String()

	return subnet, gateway, nil
}