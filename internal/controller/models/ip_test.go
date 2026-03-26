package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIPStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"available", IPStatusAvailable, "available"},
		{"assigned", IPStatusAssigned, "assigned"},
		{"reserved", IPStatusReserved, "reserved"},
		{"cooldown", IPStatusCooldown, "cooldown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.constant)
		})
	}
}

func TestIPStatusConstants_Unique(t *testing.T) {
	statuses := []string{
		IPStatusAvailable,
		IPStatusAssigned,
		IPStatusReserved,
		IPStatusCooldown,
	}

	seen := make(map[string]bool)
	for _, s := range statuses {
		assert.False(t, seen[s], "IP status %q should be unique", s)
		seen[s] = true
	}
	assert.Len(t, seen, 4, "should have exactly 4 IP statuses")
}

func TestIPSetCreateRequest_Fields(t *testing.T) {
	locID := "loc-123"
	vlanID := 100

	req := IPSetCreateRequest{
		Name:       "Production IPs",
		LocationID: &locID,
		Network:    "10.0.0.0/24",
		Gateway:    "10.0.0.1",
		VlanID:     &vlanID,
		IPVersion:  4,
		NodeIDs:    []string{"node-1", "node-2"},
	}

	assert.Equal(t, "Production IPs", req.Name)
	assert.Equal(t, &locID, req.LocationID)
	assert.Equal(t, "10.0.0.0/24", req.Network)
	assert.Equal(t, "10.0.0.1", req.Gateway)
	assert.Equal(t, &vlanID, req.VlanID)
	assert.Equal(t, 4, req.IPVersion)
	assert.Len(t, req.NodeIDs, 2)
}

func TestIPImportRequest_Fields(t *testing.T) {
	req := IPImportRequest{
		IPSetID:   "ipset-123",
		Addresses: []string{"10.0.0.2", "10.0.0.3", "10.0.0.4"},
	}

	assert.Equal(t, "ipset-123", req.IPSetID)
	assert.Len(t, req.Addresses, 3)
}

func TestIPv6Prefix_Fields(t *testing.T) {
	prefix := IPv6Prefix{
		ID:     "prefix-123",
		NodeID: "node-456",
		Prefix: "2001:db8::/48",
	}

	assert.Equal(t, "prefix-123", prefix.ID)
	assert.Equal(t, "node-456", prefix.NodeID)
	assert.Equal(t, "2001:db8::/48", prefix.Prefix)
}

func TestVMIPv6Subnet_Fields(t *testing.T) {
	subnet := VMIPv6Subnet{
		ID:           "subnet-123",
		VMID:         "vm-456",
		IPv6PrefixID: "prefix-789",
		Subnet:       "2001:db8:0:1::/64",
		SubnetIndex:  1,
		Gateway:      "2001:db8:0:1::1",
	}

	assert.Equal(t, "subnet-123", subnet.ID)
	assert.Equal(t, "vm-456", subnet.VMID)
	assert.Equal(t, "2001:db8:0:1::/64", subnet.Subnet)
	assert.Equal(t, 1, subnet.SubnetIndex)
	assert.Equal(t, "2001:db8:0:1::1", subnet.Gateway)
}
