package provisioning

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testProvisioningLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestBuildVMStatusResponse(t *testing.T) {
	tests := []struct {
		name       string
		vm         *models.VM
		wantStatus string
		wantNodeID string
	}{
		{
			name:       "running VM with node ID",
			vm:         &models.VM{Status: "running", NodeID: strPtr("node-abc")},
			wantStatus: "running",
			wantNodeID: "node-abc",
		},
		{
			name:       "stopped VM with nil node ID",
			vm:         &models.VM{Status: "stopped", NodeID: nil},
			wantStatus: "stopped",
			wantNodeID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := buildVMStatusResponse(tt.vm)
			assert.Equal(t, tt.wantStatus, resp.Status)
			assert.Equal(t, tt.wantNodeID, resp.NodeID)
		})
	}
}

func strPtr(s string) *string { return &s }

func TestGetVMByExternalServiceID_InvalidServiceID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &ProvisioningHandler{
		logger: testProvisioningLogger(),
	}

	tests := []struct {
		name      string
		serviceID string
		wantCode  int
		errCode   string
	}{
		{name: "non-numeric", serviceID: "abc", wantCode: http.StatusBadRequest, errCode: "INVALID_SERVICE_ID"},
		{name: "zero", serviceID: "0", wantCode: http.StatusBadRequest, errCode: "INVALID_SERVICE_ID"},
		{name: "negative", serviceID: "-1", wantCode: http.StatusBadRequest, errCode: "INVALID_SERVICE_ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/vms/by-service/:service_id", handler.GetVMByExternalServiceID)

			req := httptest.NewRequest(http.MethodGet, "/vms/by-service/"+tt.serviceID, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)

			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, tt.errCode, errObj["code"])
		})
	}
}
