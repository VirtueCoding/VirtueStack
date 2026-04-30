package provisioning

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(n int) *int { return &n }

func TestValidateVMID(t *testing.T) {
	tests := []struct {
		name    string
		vmID    string
		wantErr bool
		errCode string
	}{
		{
			name:    "valid UUID",
			vmID:    "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "invalid UUID",
			vmID:    "not-a-uuid",
			wantErr: true,
			errCode: "INVALID_VM_ID",
		},
		{
			name:    "empty string",
			vmID:    "",
			wantErr: true,
			errCode: "INVALID_VM_ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validateVMID(tt.vmID)
			if tt.wantErr {
				require.Error(t, err)
				var ve vmValidationError
				require.True(t, errors.As(err, &ve))
				assert.Equal(t, http.StatusBadRequest, ve.status)
				assert.Equal(t, tt.errCode, ve.errCode)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.vmID, result)
			}
		})
	}
}

func TestCalculateResizeValues(t *testing.T) {
	baseVM := &models.VM{VCPU: 2, MemoryMB: 1024, DiskGB: 20}

	tests := []struct {
		name       string
		req        ResizeRequest
		wantVCPU   int
		wantMemory int
		wantDisk   int
	}{
		{
			name:       "all nil keeps current",
			req:        ResizeRequest{},
			wantVCPU:   2,
			wantMemory: 1024,
			wantDisk:   20,
		},
		{
			name:       "only VCPU set",
			req:        ResizeRequest{VCPU: intPtr(4)},
			wantVCPU:   4,
			wantMemory: 1024,
			wantDisk:   20,
		},
		{
			name:       "only MemoryMB set",
			req:        ResizeRequest{MemoryMB: intPtr(2048)},
			wantVCPU:   2,
			wantMemory: 2048,
			wantDisk:   20,
		},
		{
			name:       "all fields set",
			req:        ResizeRequest{VCPU: intPtr(8), MemoryMB: intPtr(4096), DiskGB: intPtr(40)},
			wantVCPU:   8,
			wantMemory: 4096,
			wantDisk:   40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vcpu, mem, disk := calculateResizeValues(baseVM, tt.req)
			assert.Equal(t, tt.wantVCPU, vcpu)
			assert.Equal(t, tt.wantMemory, mem)
			assert.Equal(t, tt.wantDisk, disk)
		})
	}
}

func TestValidateDiskResize(t *testing.T) {
	tests := []struct {
		name      string
		currentGB int
		newGB     int
		wantErr   bool
	}{
		{name: "expansion allowed", currentGB: 20, newGB: 40, wantErr: false},
		{name: "same size allowed", currentGB: 20, newGB: 20, wantErr: false},
		{name: "shrink not allowed", currentGB: 20, newGB: 10, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDiskResize(tt.currentGB, tt.newGB)
			if tt.wantErr {
				require.Error(t, err)
				var ve vmValidationError
				require.True(t, errors.As(err, &ve))
				assert.Equal(t, http.StatusBadRequest, ve.status)
				assert.Equal(t, "VALIDATION_ERROR", ve.errCode)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateResizeRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     ResizeRequest
		wantErr bool
	}{
		{name: "all nil returns error", req: ResizeRequest{}, wantErr: true},
		{name: "PlanID set", req: ResizeRequest{PlanID: "550e8400-e29b-41d4-a716-446655440000"}, wantErr: false},
		{name: "VCPU set", req: ResizeRequest{VCPU: intPtr(4)}, wantErr: false},
		{name: "MemoryMB set", req: ResizeRequest{MemoryMB: intPtr(2048)}, wantErr: false},
		{name: "DiskGB set", req: ResizeRequest{DiskGB: intPtr(40)}, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResizeRequest(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				var ve vmValidationError
				require.True(t, errors.As(err, &ve))
				assert.Equal(t, http.StatusBadRequest, ve.status)
				assert.Equal(t, "VALIDATION_ERROR", ve.errCode)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestResizeResponseStatus(t *testing.T) {
	tests := []struct {
		name   string
		taskID string
		want   int
	}{
		{name: "empty task ID returns 200", taskID: "", want: http.StatusOK},
		{name: "non-empty task ID returns 202", taskID: "abc", want: http.StatusAccepted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resizeResponseStatus(tt.taskID))
		})
	}
}

func TestValidatePowerOperation(t *testing.T) {
	tests := []struct {
		name      string
		vmStatus  string
		operation string
		wantErr   bool
		errCode   string
	}{
		{name: "running VM start", vmStatus: models.VMStatusRunning, operation: "start", wantErr: false},
		{name: "running VM stop", vmStatus: models.VMStatusRunning, operation: "stop", wantErr: false},
		{name: "suspended VM start allowed", vmStatus: models.VMStatusSuspended, operation: "start", wantErr: false},
		{name: "suspended VM stop blocked", vmStatus: models.VMStatusSuspended, operation: "stop", wantErr: true, errCode: "VM_SUSPENDED"},
		{name: "suspended VM restart blocked", vmStatus: models.VMStatusSuspended, operation: "restart", wantErr: true, errCode: "VM_SUSPENDED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := &models.VM{Status: tt.vmStatus}
			err := validatePowerOperation(vm, tt.operation)
			if tt.wantErr {
				require.Error(t, err)
				var ve vmValidationError
				require.True(t, errors.As(err, &ve))
				assert.Equal(t, http.StatusBadRequest, ve.status)
				assert.Equal(t, tt.errCode, ve.errCode)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRespondWithValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("vmValidationError produces correct response", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		ve := vmValidationError{status: http.StatusBadRequest, errCode: "INVALID_VM_ID", errMsg: "VM ID must be a valid UUID"}
		respondWithValidationError(c, ve)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		errObj, ok := resp["error"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "INVALID_VM_ID", errObj["code"])
	})

	t.Run("regular error produces 500", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		respondWithValidationError(c, errors.New("something went wrong"))

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		errObj, ok := resp["error"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "INTERNAL_ERROR", errObj["code"])
	})
}

func TestPowerOperationResponse(t *testing.T) {
	resp := powerOperationResponse("vm-123", "start")
	assert.Equal(t, "vm-123", resp["vm_id"])
	assert.Equal(t, "start", resp["operation"])
	assert.Equal(t, "Power operation completed successfully", resp["message"])
}
