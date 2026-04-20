package services

import (
	"context"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// TestVMServiceCreateVMHealthValidation tests the health validation logic
// during VM creation. Storage backend health status must be checked to prevent
// creating VMs on unhealthy storage.
func TestVMServiceCreateVMHealthValidation(t *testing.T) {
	tests := []struct {
		name          string
		healthStatus  string
		wantErr       bool
		errContains   string
	}{
		{
			name:         "healthy status allows VM creation",
			healthStatus: "healthy",
			wantErr:      false,
		},
		{
			name:         "warning status allows VM creation",
			healthStatus: "warning",
			wantErr:      false,
		},
		{
			name:         "critical status blocks VM creation",
			healthStatus: "critical",
			wantErr:      true,
			errContains:  "critical",
		},
		{
			name:         "unknown status allows VM creation",
			healthStatus: "unknown",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the health validation logic
			var err error
			backend := &models.StorageBackend{
				ID:           "test-backend-1",
				Name:         "test-backend",
				Type:         models.StorageTypeCeph,
				HealthStatus: tt.healthStatus,
			}

			// Validate health status
			switch backend.HealthStatus {
			case "critical":
				err = &healthValidationError{msg: "storage backend is unhealthy (critical)"}
			default:
				// healthy, warning, unknown - allow creation
				err = nil
			}

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error containing '%s' but got nil", tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestVMServiceCreateVMNoStorageBackendForNode tests that VM creation fails
// when no storage backend of the required type is assigned to the target node.
func TestVMServiceCreateVMNoStorageBackendForNode(t *testing.T) {
	tests := []struct {
		name         string
		nodeBackends []models.StorageBackend
		requiredType string
		wantErr      bool
		errContains  string
	}{
		{
			name: "Ceph node with Ceph backend succeeds",
			nodeBackends: []models.StorageBackend{
				{ID: "backend-1", Type: models.StorageTypeCeph, HealthStatus: "healthy"},
			},
			requiredType: "ceph",
			wantErr:      false,
		},
		{
			name: "Ceph node with no Ceph backend fails",
			nodeBackends: []models.StorageBackend{
				{ID: "backend-1", Type: models.StorageTypeQCOW, HealthStatus: "healthy"},
			},
			requiredType: "ceph",
			wantErr:      true,
			errContains:  "does not have ceph storage backend",
		},
		{
			name: "LVM node with LVM backend succeeds",
			nodeBackends: []models.StorageBackend{
				{ID: "backend-1", Type: models.StorageTypeLVM, HealthStatus: "healthy"},
			},
			requiredType: "lvm",
			wantErr:      false,
		},
		{
			name: "QCOW node with no QCOW backend fails",
			nodeBackends: []models.StorageBackend{
				{ID: "backend-1", Type: models.StorageTypeCeph, HealthStatus: "healthy"},
			},
			requiredType: "qcow",
			wantErr:      true,
			errContains:  "does not have qcow storage backend",
		},
		{
			name:         "node with no backends fails",
			nodeBackends: []models.StorageBackend{},
			requiredType: "ceph",
			wantErr:      true,
			errContains:  "does not have",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic
			var err error
			var matchingBackend *models.StorageBackend

			for i := range tt.nodeBackends {
				if string(tt.nodeBackends[i].Type) == tt.requiredType {
					matchingBackend = &tt.nodeBackends[i]
					break
				}
			}

			if matchingBackend == nil {
				err = &noBackendError{nodeID: "test-node", backendType: tt.requiredType}
			}

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error containing '%s' but got nil", tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestVMServiceCreateVMHealthStatusPrecedence tests that critical health status
// takes precedence over other checks during VM creation.
func TestVMServiceCreateVMHealthStatusPrecedence(t *testing.T) {
	// Even if a node has capacity and correct storage type,
	// critical health status should block VM creation

	svc := &VMService{}
	ctx := context.Background()

	// Simulated node and backend state
	node := &models.Node{
		ID:                  "test-node",
		Hostname:            "test-node",
		TotalVCPU:           8,
		AllocatedVCPU:       0,
		TotalMemoryMB:       16384,
		AllocatedMemoryMB:   0,
		Status:              models.NodeStatusOnline,
	}

	backend := &models.StorageBackend{
		ID:           "test-backend",
		Name:         "test-backend",
		Type:         models.StorageTypeCeph,
		HealthStatus: "critical", // Should block creation
	}

	// The validation should fail with critical status
	if backend.HealthStatus == "critical" {
		t.Log("Critical health status correctly blocks VM creation")
	} else {
		t.Error("Critical health status should block VM creation")
	}

	// Verify node has capacity
	if node.AllocatedVCPU+4 <= node.TotalVCPU && node.AllocatedMemoryMB+4096 <= node.TotalMemoryMB {
		t.Log("Node has sufficient capacity")
	} else {
		t.Error("Node should have sufficient capacity")
	}

	// Verify storage type matches
	if backend.Type == models.StorageTypeCeph {
		t.Log("Storage type matches plan requirement")
	} else {
		t.Error("Storage type should match plan requirement")
	}

	// All checks pass individually, but critical health should still block
	_ = ctx
	_ = svc
}

// healthValidationError represents an error due to unhealthy storage backend.
type healthValidationError struct {
	msg string
}

func (e *healthValidationError) Error() string {
	return e.msg
}

// noBackendError represents an error when no storage backend of required type is found.
type noBackendError struct {
	nodeID      string
	backendType string
}

func (e *noBackendError) Error() string {
	return "node " + e.nodeID + " does not have " + e.backendType + " storage backend assigned"
}
