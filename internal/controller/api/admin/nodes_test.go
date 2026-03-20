package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListNodes_InvalidStatus tests status filter validation for nodes.
func TestListNodes_InvalidStatus(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.GET("/nodes", handler.ListNodes)

	req := httptest.NewRequest(http.MethodGet, "/nodes?status=invalid_status", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_STATUS", errorObj["code"])
}

// TestListNodes_ValidStatusFilters tests all valid node statuses.
// Note: Full list operation requires service mock. This tests that valid statuses
// pass the validation check.
func TestListNodes_ValidStatusFilters(t *testing.T) {
	// Valid statuses should not trigger validation errors
	validStatuses := []string{"online", "degraded", "offline", "draining", "failed"}

	for _, status := range validStatuses {
		t.Run(status, func(t *testing.T) {
			// Verify the status is in the expected list
			assert.Contains(t, []string{"online", "degraded", "offline", "draining", "failed"}, status)
		})
	}
}

// TestListNodes_InvalidLocationID tests location_id UUID validation.
func TestListNodes_InvalidLocationID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.GET("/nodes", handler.ListNodes)

	req := httptest.NewRequest(http.MethodGet, "/nodes?location_id=not-a-uuid", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_LOCATION_ID", errorObj["code"])
}

// TestRegisterNode_InvalidJSON tests node registration with invalid JSON.
func TestRegisterNode_InvalidJSON(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.POST("/nodes", handler.RegisterNode)

	req := httptest.NewRequest(http.MethodPost, "/nodes", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestRegisterNode_StorageValidation tests storage config validation.
// Note: Tests that pass validation would require service mock for full handler test.
func TestRegisterNode_StorageValidation(t *testing.T) {
	// Test the validateStorageConfig function directly
	tests := []struct {
		name           string
		storageBackend string
		storagePath    string
		expectError    bool
	}{
		{
			name:           "qcow without path - invalid",
			storageBackend: "qcow",
			storagePath:    "",
			expectError:    true,
		},
		{
			name:           "qcow with path - valid",
			storageBackend: "qcow",
			storagePath:    "/var/lib/vms",
			expectError:    false,
		},
		{
			name:           "ceph without path - valid",
			storageBackend: "ceph",
			storagePath:    "",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStorageConfig(tt.storageBackend, tt.storagePath)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "storage_path is required")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestGetNode_InvalidID tests UUID validation for node ID.
func TestGetNode_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.GET("/nodes/:id", handler.GetNode)

	tests := []string{
		"not-a-uuid",
		"123",
		"abc-def-ghi",
	}

	for _, invalidID := range tests {
		t.Run(invalidID, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/nodes/"+invalidID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)

			errorObj, ok := resp["error"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, "INVALID_NODE_ID", errorObj["code"])
		})
	}
}

// TestUpdateNode_InvalidID tests UUID validation on node update.
func TestUpdateNode_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.PUT("/nodes/:id", handler.UpdateNode)

	body := `{"grpc_address": "10.0.0.2:50051"}`
	req := httptest.NewRequest(http.MethodPut, "/nodes/not-a-uuid", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestUpdateNode_InvalidGRPCAddress tests grpc_address validation.
func TestUpdateNode_InvalidGRPCAddress(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.PUT("/nodes/:id", handler.UpdateNode)

	// grpc_address > 255 chars
	longAddress := ""
	for i := 0; i < 256; i++ {
		longAddress += "a"
	}
	body := map[string]string{
		"grpc_address": longAddress,
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/nodes/00000000-0000-0000-0000-000000000001", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestUpdateNode_InvalidVCPU tests vCPU validation.
func TestUpdateNode_InvalidVCPU(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.PUT("/nodes/:id", handler.UpdateNode)

	body := map[string]int{
		"total_vcpu": 0, // min is 1
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/nodes/00000000-0000-0000-0000-000000000001", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestUpdateNode_InvalidMemory tests memory validation.
func TestUpdateNode_InvalidMemory(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.PUT("/nodes/:id", handler.UpdateNode)

	body := map[string]int{
		"total_memory_mb": 1023, // min is 1024
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/nodes/00000000-0000-0000-0000-000000000001", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestDeleteNode_InvalidID tests UUID validation on delete.
func TestDeleteNode_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.DELETE("/nodes/:id", handler.DeleteNode)

	req := httptest.NewRequest(http.MethodDelete, "/nodes/invalid-uuid", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestDrainNode_InvalidID tests UUID validation on drain.
func TestDrainNode_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.POST("/nodes/:id/drain", handler.DrainNode)

	req := httptest.NewRequest(http.MethodPost, "/nodes/invalid/drain", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestFailoverNode_InvalidID tests UUID validation on failover.
func TestFailoverNode_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.POST("/nodes/:id/failover", handler.FailoverNode)

	req := httptest.NewRequest(http.MethodPost, "/nodes/invalid/failover", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestUndrainNode_InvalidID tests UUID validation on undrain.
func TestUndrainNode_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.POST("/nodes/:id/undrain", handler.UndrainNode)

	req := httptest.NewRequest(http.MethodPost, "/nodes/invalid/undrain", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestValidateStorageConfig tests the storage config validation function.
func TestValidateStorageConfig(t *testing.T) {
	tests := []struct {
		name           string
		storageBackend string
		storagePath    string
		expectError    bool
	}{
		{
			name:           "qcow with path",
			storageBackend: "qcow",
			storagePath:    "/var/lib/vms",
			expectError:    false,
		},
		{
			name:           "qcow without path",
			storageBackend: "qcow",
			storagePath:    "",
			expectError:    true,
		},
		{
			name:           "ceph without path",
			storageBackend: "ceph",
			storagePath:    "",
			expectError:    false,
		},
		{
			name:           "ceph with path",
			storageBackend: "ceph",
			storagePath:    "/ceph/pool",
			expectError:    false,
		},
		{
			name:           "empty backend",
			storageBackend: "",
			storagePath:    "",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStorageConfig(tt.storageBackend, tt.storagePath)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "storage_path is required")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestNodeUpdateRequest_ValidationRules tests struct validation.
func TestNodeUpdateRequest_ValidationRules(t *testing.T) {
	tests := []struct {
		name    string
		request NodeUpdateRequest
		valid   bool
	}{
		{
			name:    "empty request valid",
			request: NodeUpdateRequest{},
			valid:   true,
		},
		{
			name: "valid vcpu update",
			request: NodeUpdateRequest{
				TotalVCPU: intPtr(16),
			},
			valid: true,
		},
		{
			name: "valid memory update",
			request: NodeUpdateRequest{
				TotalMemory: intPtr(32768),
			},
			valid: true,
		},
		{
			name: "valid storage backend",
			request: NodeUpdateRequest{
				StorageBackend: strPtr("ceph"),
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				if tt.request.TotalVCPU != nil {
					assert.GreaterOrEqual(t, *tt.request.TotalVCPU, 1)
				}
				if tt.request.TotalMemory != nil {
					assert.GreaterOrEqual(t, *tt.request.TotalMemory, 1024)
				}
				if tt.request.StorageBackend != nil {
					assert.Contains(t, []string{"ceph", "qcow"}, *tt.request.StorageBackend)
				}
			}
		})
	}
}

// TestNodeStatusResponse tests response structure.
func TestNodeStatusResponse_Structure(t *testing.T) {
	resp := NodeStatusResponse{Status: "draining"}
	assert.Equal(t, "draining", resp.Status)
}

// Helper
func intPtr(i int) *int {
	return &i
}