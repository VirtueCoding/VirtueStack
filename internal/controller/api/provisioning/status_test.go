package provisioning

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
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

func TestGetVMByExternalServiceIDRequiresOwnershipAssertion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	vmID := "550e8400-e29b-41d4-a716-446655440010"
	customerID := "550e8400-e29b-41d4-a716-446655440000"
	db := &provisioningFakeDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM vms WHERE external_service_id = $1"):
				return &provisioningFakeRow{values: provisioningVMRow(vmID, customerID, 123)}
			case strings.Contains(sql, "FROM customers WHERE id = $1"):
				return &provisioningFakeRow{values: provisioningCustomerRow(customerID, 42)}
			default:
				return &provisioningFakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}
	handler := &ProvisioningHandler{
		vmRepo:       repository.NewVMRepository(db),
		customerRepo: repository.NewCustomerRepository(db),
		logger:       testProvisioningLogger(),
	}
	router := gin.New()
	router.GET("/vms/by-service/:service_id", handler.GetVMByExternalServiceID)

	tests := []struct {
		name     string
		query    string
		wantCode int
		errCode  string
	}{
		{name: "missing customer assertion", query: "", wantCode: http.StatusBadRequest, errCode: "OWNERSHIP_ASSERTION_REQUIRED"},
		{name: "mismatched external client", query: "?external_client_id=99", wantCode: http.StatusNotFound, errCode: "VM_NOT_FOUND"},
		{name: "matching external client", query: "?external_client_id=42", wantCode: http.StatusOK},
		{name: "matching customer id", query: "?customer_id=" + customerID, wantCode: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/vms/by-service/123"+tt.query, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
			if tt.errCode == "" {
				assert.Contains(t, w.Body.String(), vmID)
				return
			}
			var resp map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.errCode, errObj["code"])
			assert.NotContains(t, w.Body.String(), vmID)
		})
	}
}
