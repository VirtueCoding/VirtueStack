package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"online", NodeStatusOnline, "online"},
		{"degraded", NodeStatusDegraded, "degraded"},
		{"offline", NodeStatusOffline, "offline"},
		{"draining", NodeStatusDraining, "draining"},
		{"failed", NodeStatusFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.constant)
		})
	}
}

func TestNodeStatusConstants_Unique(t *testing.T) {
	statuses := []string{
		NodeStatusOnline,
		NodeStatusDegraded,
		NodeStatusOffline,
		NodeStatusDraining,
		NodeStatusFailed,
	}

	seen := make(map[string]bool)
	for _, s := range statuses {
		assert.False(t, seen[s], "node status %q should be unique", s)
		seen[s] = true
	}
	assert.Len(t, seen, 5, "should have exactly 5 node statuses")
}

func TestNodeCreateRequest_Fields(t *testing.T) {
	locID := "loc-123"
	req := NodeCreateRequest{
		Hostname:       "node-01",
		GRPCAddress:    "10.0.0.1:50051",
		ManagementIP:   "10.0.0.1",
		LocationID:     &locID,
		TotalVCPU:      32,
		TotalMemoryMB:  65536,
		StorageBackend: "ceph",
		CephPool:       "vs-vms",
	}

	assert.Equal(t, "node-01", req.Hostname)
	assert.Equal(t, "10.0.0.1:50051", req.GRPCAddress)
	assert.Equal(t, "10.0.0.1", req.ManagementIP)
	assert.Equal(t, &locID, req.LocationID)
	assert.Equal(t, 32, req.TotalVCPU)
	assert.Equal(t, 65536, req.TotalMemoryMB)
	assert.Equal(t, "ceph", req.StorageBackend)
}

func TestNodeListFilter_Fields(t *testing.T) {
	status := "online"
	locID := "loc-123"

	filter := NodeListFilter{
		Status:     &status,
		LocationID: &locID,
	}

	assert.Equal(t, &status, filter.Status)
	assert.Equal(t, &locID, filter.LocationID)
}

func TestNodeStatus_AvailableResources(t *testing.T) {
	status := NodeStatus{
		NodeID:            "node-123",
		Hostname:          "node-01",
		Status:            NodeStatusOnline,
		TotalVCPU:         32,
		AllocatedVCPU:     20,
		AvailableVCPU:     12,
		TotalMemoryMB:     65536,
		AllocatedMemoryMB: 40960,
		AvailableMemoryMB: 24576,
		VMCount:           10,
		IsHealthy:         true,
	}

	assert.Equal(t, 12, status.AvailableVCPU)
	assert.Equal(t, 24576, status.AvailableMemoryMB)
	assert.True(t, status.IsHealthy)
	assert.Equal(t, 10, status.VMCount)
}

func TestNode_SensitiveFieldsNotSerialized(t *testing.T) {
	encrypted := "encrypted"
	node := Node{
		IPMIAddress:           &encrypted,
		IPMIUsernameEncrypted: &encrypted,
		IPMIPasswordEncrypted: &encrypted,
	}
	assert.NotNil(t, node.IPMIAddress)
	assert.NotNil(t, node.IPMIUsernameEncrypted)
	assert.NotNil(t, node.IPMIPasswordEncrypted)
}
