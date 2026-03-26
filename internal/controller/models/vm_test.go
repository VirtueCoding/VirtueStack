package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVMStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"provisioning", VMStatusProvisioning, "provisioning"},
		{"running", VMStatusRunning, "running"},
		{"stopped", VMStatusStopped, "stopped"},
		{"suspended", VMStatusSuspended, "suspended"},
		{"migrating", VMStatusMigrating, "migrating"},
		{"reinstalling", VMStatusReinstalling, "reinstalling"},
		{"error", VMStatusError, "error"},
		{"deleted", VMStatusDeleted, "deleted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.constant)
		})
	}
}

func TestVMStatusConstants_Unique(t *testing.T) {
	statuses := []string{
		VMStatusProvisioning,
		VMStatusRunning,
		VMStatusStopped,
		VMStatusSuspended,
		VMStatusMigrating,
		VMStatusReinstalling,
		VMStatusError,
		VMStatusDeleted,
	}

	seen := make(map[string]bool)
	for _, s := range statuses {
		assert.False(t, seen[s], "VM status %q should be unique", s)
		seen[s] = true
	}
	assert.Len(t, seen, 8, "should have exactly 8 VM statuses")
}

func TestVMCreateRequest_Fields(t *testing.T) {
	req := VMCreateRequest{
		CustomerID: "cust-123",
		PlanID:     "plan-456",
		Hostname:   "my-server",
		TemplateID: "tmpl-789",
		Password:   "securePassword123",
		SSHKeys:    []string{"ssh-rsa AAAA..."},
	}

	assert.Equal(t, "cust-123", req.CustomerID)
	assert.Equal(t, "plan-456", req.PlanID)
	assert.Equal(t, "my-server", req.Hostname)
	assert.Equal(t, "tmpl-789", req.TemplateID)
	assert.Equal(t, "securePassword123", req.Password)
	assert.Len(t, req.SSHKeys, 1)
}

func TestVMListFilter_Fields(t *testing.T) {
	customerID := "cust-123"
	nodeID := "node-456"
	status := "running"
	search := "my-vm"

	filter := VMListFilter{
		CustomerID: &customerID,
		NodeID:     &nodeID,
		Status:     &status,
		Search:     &search,
		VMIDs:      []string{"vm-1", "vm-2"},
	}

	assert.Equal(t, &customerID, filter.CustomerID)
	assert.Equal(t, &nodeID, filter.NodeID)
	assert.Equal(t, &status, filter.Status)
	assert.Equal(t, &search, filter.Search)
	assert.Len(t, filter.VMIDs, 2)
}

func TestVMDetail_EmbeddedVM(t *testing.T) {
	detail := VMDetail{
		VM: VM{
			ID:       "vm-123",
			Hostname: "test-server",
			Status:   VMStatusRunning,
		},
		PlanName: "Basic Plan",
	}

	assert.Equal(t, "vm-123", detail.ID)
	assert.Equal(t, "test-server", detail.Hostname)
	assert.Equal(t, VMStatusRunning, detail.Status)
	assert.Equal(t, "Basic Plan", detail.PlanName)
}

func TestVMMetrics_Fields(t *testing.T) {
	metrics := VMMetrics{
		VMID:             "vm-123",
		CPUUsagePercent:  55.5,
		MemoryUsageBytes: 1073741824,
		MemoryTotalBytes: 2147483648,
		UptimeSeconds:    3600,
	}

	assert.Equal(t, "vm-123", metrics.VMID)
	assert.InDelta(t, 55.5, metrics.CPUUsagePercent, 0.001)
	assert.Equal(t, int64(1073741824), metrics.MemoryUsageBytes)
	assert.Equal(t, int64(2147483648), metrics.MemoryTotalBytes)
	assert.Equal(t, int64(3600), metrics.UptimeSeconds)
}

func TestVM_RootPasswordNotSerialized(t *testing.T) {
	encrypted := "encrypted-password"
	vm := VM{
		RootPasswordEncrypted: &encrypted,
	}
	assert.NotNil(t, vm.RootPasswordEncrypted)
	assert.Equal(t, "encrypted-password", *vm.RootPasswordEncrypted)
}
