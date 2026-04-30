package provisioning

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVMUsage_InvalidVMID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ProvisioningHandler{
		logger: testProvisioningLogger(),
	}

	tests := []struct {
		name     string
		vmID     string
		wantHTTP int
		errCode  string
	}{
		{
			name:     "invalid UUID",
			vmID:     "not-a-uuid",
			wantHTTP: http.StatusBadRequest,
			errCode:  "INVALID_VM_ID",
		},
		{
			name:     "empty string",
			vmID:     "",
			wantHTTP: http.StatusBadRequest,
			errCode:  "INVALID_VM_ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/vms/:id/usage", handler.GetVMUsage)

			path := "/vms/" + tt.vmID + "/usage"
			if tt.vmID == "" {
				path = "/vms//usage"
			}

			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantHTTP, w.Code)

			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]interface{})
			require.True(t, ok, "expected error object in response")
			assert.Equal(t, tt.errCode, errObj["code"])
		})
	}
}

func TestBytesToGB(t *testing.T) {
	tests := []struct {
		name   string
		bytes  uint64
		wantGB float64
	}{
		{
			name:   "zero bytes",
			bytes:  0,
			wantGB: 0,
		},
		{
			name:   "exactly 1 GB",
			bytes:  1024 * 1024 * 1024,
			wantGB: 1.0,
		},
		{
			name:   "10 GB",
			bytes:  10 * 1024 * 1024 * 1024,
			wantGB: 10.0,
		},
		{
			name:   "fractional GB",
			bytes:  1536 * 1024 * 1024, // 1.5 GB
			wantGB: 1.5,
		},
		{
			name:   "small amount",
			bytes:  100 * 1024 * 1024, // ~0.1 GB
			wantGB: 0.1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bytesToGB(tt.bytes)
			assert.InDelta(t, tt.wantGB, got, 0.01)
		})
	}
}

func TestVMUsageResponse_JSONFormat(t *testing.T) {
	resp := VMUsageResponse{
		VMID:             "550e8400-e29b-41d4-a716-446655440000",
		BandwidthUsedGB:  12.34,
		BandwidthLimitGB: 1000,
		DiskUsedGB:       25,
		DiskLimitGB:      50,
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", decoded["vm_id"])
	assert.Equal(t, 12.34, decoded["bandwidth_used_gb"])
	assert.Equal(t, float64(1000), decoded["bandwidth_limit_gb"])
	assert.Equal(t, float64(25), decoded["disk_used_gb"])
	assert.Equal(t, float64(50), decoded["disk_limit_gb"])
}
