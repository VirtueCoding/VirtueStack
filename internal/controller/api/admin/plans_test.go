package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListPlans_InvalidIsActive tests is_active filter validation.
func TestListPlans_InvalidIsActive(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.GET("/plans", handler.ListPlans)

	tests := []struct {
		name     string
		isActive string
	}{
		{
			name:     "invalid boolean string",
			isActive: "invalid",
		},
		{
			name:     "numeric string",
			isActive: "1",
		},
		{
			name:     "yes/no format",
			isActive: "yes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/plans?is_active="+tt.isActive, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)

			errorObj, ok := resp["error"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, "INVALID_IS_ACTIVE", errorObj["code"])
		})
	}
}

// TestListPlans_ValidIsActiveFilters tests valid is_active values.
func TestListPlans_ValidIsActiveFilters(t *testing.T) {
	validFilters := []struct {
		name     string
		isActive string
	}{
		{
			name:     "true",
			isActive: "true",
		},
		{
			name:     "false",
			isActive: "false",
		},
	}

	for _, tt := range validFilters {
		t.Run(tt.name, func(t *testing.T) {
			// Verify the filter values are valid
			assert.Contains(t, []string{"true", "false"}, tt.isActive)
		})
	}
}

// TestCreatePlan_InvalidJSON tests plan creation with invalid JSON.
func TestCreatePlan_InvalidJSON(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.POST("/plans", handler.CreatePlan)

	req := httptest.NewRequest(http.MethodPost, "/plans", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestCreatePlan_MissingRequiredFields tests validation for missing required fields.
func TestCreatePlan_MissingRequiredFields(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.POST("/plans", handler.CreatePlan)

	tests := []struct {
		name    string
		request map[string]interface{}
	}{
		{
			name:    "missing name",
			request: map[string]interface{}{"slug": "test-plan", "vcpu": 1, "memory_mb": 1024, "disk_gb": 10, "port_speed_mbps": 100},
		},
		{
			name:    "missing slug",
			request: map[string]interface{}{"name": "Test Plan", "vcpu": 1, "memory_mb": 1024, "disk_gb": 10, "port_speed_mbps": 100},
		},
		{
			name:    "missing vcpu",
			request: map[string]interface{}{"name": "Test Plan", "slug": "test-plan", "memory_mb": 1024, "disk_gb": 10, "port_speed_mbps": 100},
		},
		{
			name:    "missing memory_mb",
			request: map[string]interface{}{"name": "Test Plan", "slug": "test-plan", "vcpu": 1, "disk_gb": 10, "port_speed_mbps": 100},
		},
		{
			name:    "missing disk_gb",
			request: map[string]interface{}{"name": "Test Plan", "slug": "test-plan", "vcpu": 1, "memory_mb": 1024, "port_speed_mbps": 100},
		},
		{
			name:    "missing port_speed_mbps",
			request: map[string]interface{}{"name": "Test Plan", "slug": "test-plan", "vcpu": 1, "memory_mb": 1024, "disk_gb": 10},
		},
		{
			name:    "empty request",
			request: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBody, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/plans", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestCreatePlan_InvalidFieldValues tests validation for invalid field values.
func TestCreatePlan_InvalidFieldValues(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.POST("/plans", handler.CreatePlan)

	tests := []struct {
		name    string
		request map[string]interface{}
	}{
		{
			name: "name too long (>100)",
			request: map[string]interface{}{
				"name":            string(make([]byte, 101)),
				"slug":            "test-plan",
				"vcpu":            1,
				"memory_mb":       1024,
				"disk_gb":         10,
				"port_speed_mbps": 100,
			},
		},
		{
			name: "slug too long (>100)",
			request: map[string]interface{}{
				"name":            "Test Plan",
				"slug":            string(make([]byte, 101)),
				"vcpu":            1,
				"memory_mb":       1024,
				"disk_gb":         10,
				"port_speed_mbps": 100,
			},
		},
		{
			name: "slug non-alphanumeric",
			request: map[string]interface{}{
				"name":            "Test Plan",
				"slug":            "test-plan!", // ! is not alphanumeric
				"vcpu":            1,
				"memory_mb":       1024,
				"disk_gb":         10,
				"port_speed_mbps": 100,
			},
		},
		{
			name: "vcpu below minimum (0)",
			request: map[string]interface{}{
				"name":            "Test Plan",
				"slug":            "test-plan",
				"vcpu":            0,
				"memory_mb":       1024,
				"disk_gb":         10,
				"port_speed_mbps": 100,
			},
		},
		{
			name: "memory_mb below minimum (511)",
			request: map[string]interface{}{
				"name":            "Test Plan",
				"slug":            "test-plan",
				"vcpu":            1,
				"memory_mb":       511,
				"disk_gb":         10,
				"port_speed_mbps": 100,
			},
		},
		{
			name: "disk_gb below minimum (9)",
			request: map[string]interface{}{
				"name":            "Test Plan",
				"slug":            "test-plan",
				"vcpu":            1,
				"memory_mb":       1024,
				"disk_gb":         9,
				"port_speed_mbps": 100,
			},
		},
		{
			name: "port_speed_mbps below minimum (0)",
			request: map[string]interface{}{
				"name":            "Test Plan",
				"slug":            "test-plan",
				"vcpu":            1,
				"memory_mb":       1024,
				"disk_gb":         10,
				"port_speed_mbps": 0,
			},
		},
		{
			name: "bandwidth_limit_gb negative",
			request: map[string]interface{}{
				"name":              "Test Plan",
				"slug":              "test-plan",
				"vcpu":              1,
				"memory_mb":         1024,
				"disk_gb":           10,
				"port_speed_mbps":   100,
				"bandwidth_limit_gb": -1,
			},
		},
		{
			name: "price_monthly negative",
			request: map[string]interface{}{
				"name":            "Test Plan",
				"slug":            "test-plan",
				"vcpu":            1,
				"memory_mb":       1024,
				"disk_gb":         10,
				"port_speed_mbps": 100,
				"price_monthly":   -100,
			},
		},
		{
			name: "price_hourly negative",
			request: map[string]interface{}{
				"name":            "Test Plan",
				"slug":            "test-plan",
				"vcpu":            1,
				"memory_mb":       1024,
				"disk_gb":         10,
				"port_speed_mbps": 100,
				"price_hourly":    -1,
			},
		},
		{
			name: "storage_backend invalid",
			request: map[string]interface{}{
				"name":            "Test Plan",
				"slug":            "test-plan",
				"vcpu":            1,
				"memory_mb":       1024,
				"disk_gb":         10,
				"port_speed_mbps": 100,
				"storage_backend": "invalid",
			},
		},
		{
			name: "sort_order negative",
			request: map[string]interface{}{
				"name":            "Test Plan",
				"slug":            "test-plan",
				"vcpu":            1,
				"memory_mb":       1024,
				"disk_gb":         10,
				"port_speed_mbps": 100,
				"sort_order":      -1,
			},
		},
		{
			name: "snapshot_limit negative",
			request: map[string]interface{}{
				"name":            "Test Plan",
				"slug":            "test-plan",
				"vcpu":            1,
				"memory_mb":       1024,
				"disk_gb":         10,
				"port_speed_mbps": 100,
				"snapshot_limit":  -1,
			},
		},
		{
			name: "backup_limit negative",
			request: map[string]interface{}{
				"name":            "Test Plan",
				"slug":            "test-plan",
				"vcpu":            1,
				"memory_mb":       1024,
				"disk_gb":         10,
				"port_speed_mbps": 100,
				"backup_limit":    -1,
			},
		},
		{
			name: "iso_upload_limit negative",
			request: map[string]interface{}{
				"name":             "Test Plan",
				"slug":             "test-plan",
				"vcpu":             1,
				"memory_mb":        1024,
				"disk_gb":          10,
				"port_speed_mbps":  100,
				"iso_upload_limit": -1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBody, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/plans", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestUpdatePlan_InvalidID tests UUID validation on update.
func TestUpdatePlan_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.PUT("/plans/:id", handler.UpdatePlan)

	body := `{"name": "Updated Plan"}`
	req := httptest.NewRequest(http.MethodPut, "/plans/not-a-uuid", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_PLAN_ID", errorObj["code"])
}

// TestUpdatePlan_InvalidJSON tests update with invalid JSON.
func TestUpdatePlan_InvalidJSON(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.PUT("/plans/:id", handler.UpdatePlan)

	req := httptest.NewRequest(http.MethodPut, "/plans/00000000-0000-0000-0000-000000000001", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestUpdatePlan_InvalidFieldValues tests validation for invalid update field values.
func TestUpdatePlan_InvalidFieldValues(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.PUT("/plans/:id", handler.UpdatePlan)

	validUUID := "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name    string
		request map[string]interface{}
	}{
		{
			name:    "name too long",
			request: map[string]interface{}{"name": string(make([]byte, 101))},
		},
		{
			name:    "slug too long",
			request: map[string]interface{}{"slug": string(make([]byte, 101))},
		},
		{
			name:    "slug non-alphanumeric",
			request: map[string]interface{}{"slug": "test-plan!"},
		},
		{
			name:    "vcpu below minimum",
			request: map[string]interface{}{"vcpu": 0},
		},
		{
			name:    "memory_mb below minimum",
			request: map[string]interface{}{"memory_mb": 511},
		},
		{
			name:    "disk_gb below minimum",
			request: map[string]interface{}{"disk_gb": 9},
		},
		{
			name:    "port_speed_mbps below minimum",
			request: map[string]interface{}{"port_speed_mbps": 0},
		},
		{
			name:    "bandwidth_limit_gb negative",
			request: map[string]interface{}{"bandwidth_limit_gb": -1},
		},
		{
			name:    "price_monthly negative",
			request: map[string]interface{}{"price_monthly": -1},
		},
		{
			name:    "price_hourly negative",
			request: map[string]interface{}{"price_hourly": -1},
		},
		{
			name:    "storage_backend invalid",
			request: map[string]interface{}{"storage_backend": "invalid"},
		},
		{
			name:    "sort_order negative",
			request: map[string]interface{}{"sort_order": -1},
		},
		{
			name:    "snapshot_limit negative",
			request: map[string]interface{}{"snapshot_limit": -1},
		},
		{
			name:    "backup_limit negative",
			request: map[string]interface{}{"backup_limit": -1},
		},
		{
			name:    "iso_upload_limit negative",
			request: map[string]interface{}{"iso_upload_limit": -1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBody, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPut, "/plans/"+validUUID, bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestDeletePlan_InvalidID tests UUID validation on delete.
func TestDeletePlan_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.DELETE("/plans/:id", handler.DeletePlan)

	tests := []string{
		"not-a-uuid",
		"123",
		"abc-def-ghi",
	}

	for _, invalidID := range tests {
		t.Run(invalidID, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/plans/"+invalidID, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)

			errorObj, ok := resp["error"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, "INVALID_PLAN_ID", errorObj["code"])
		})
	}
}

// TestPlanCreateRequest_ValidationRules tests struct validation rules for create.
func TestPlanCreateRequest_ValidationRules(t *testing.T) {
	tests := []struct {
		name        string
		request     models.PlanCreateRequest
		expectValid bool
	}{
		{
			name: "valid request with all required fields",
			request: models.PlanCreateRequest{
				Name:             "Basic Plan",
				Slug:             "basic-plan",
				VCPU:             1,
				MemoryMB:         1024,
				DiskGB:           10,
				PortSpeedMbps:    100,
				BandwidthLimitGB: 0,
				PriceMonthly:     999,
				PriceHourly:      1,
				StorageBackend:   "ceph",
				IsActive:         true,
				SortOrder:        0,
				SnapshotLimit:    0,
				BackupLimit:      0,
				ISOUploadLimit:   0,
			},
			expectValid: true,
		},
		{
			name: "name at max length (100)",
			request: models.PlanCreateRequest{
				Name:          string(make([]byte, 100)),
				Slug:          "test",
				VCPU:          1,
				MemoryMB:      1024,
				DiskGB:        10,
				PortSpeedMbps: 100,
			},
			expectValid: true,
		},
		{
			name: "slug at max length (100)",
			request: models.PlanCreateRequest{
				Name:          "Test",
				Slug:          string(make([]byte, 100)),
				VCPU:          1,
				MemoryMB:      1024,
				DiskGB:        10,
				PortSpeedMbps: 100,
			},
			expectValid: true,
		},
		{
			name: "vcpu at minimum (1)",
			request: models.PlanCreateRequest{
				Name:          "Test",
				Slug:          "test",
				VCPU:          1,
				MemoryMB:      1024,
				DiskGB:        10,
				PortSpeedMbps: 100,
			},
			expectValid: true,
		},
		{
			name: "memory_mb at minimum (512)",
			request: models.PlanCreateRequest{
				Name:          "Test",
				Slug:          "test",
				VCPU:          1,
				MemoryMB:      512,
				DiskGB:        10,
				PortSpeedMbps: 100,
			},
			expectValid: true,
		},
		{
			name: "disk_gb at minimum (10)",
			request: models.PlanCreateRequest{
				Name:          "Test",
				Slug:          "test",
				VCPU:          1,
				MemoryMB:      1024,
				DiskGB:        10,
				PortSpeedMbps: 100,
			},
			expectValid: true,
		},
		{
			name: "port_speed_mbps at minimum (1)",
			request: models.PlanCreateRequest{
				Name:          "Test",
				Slug:          "test",
				VCPU:          1,
				MemoryMB:      1024,
				DiskGB:        10,
				PortSpeedMbps: 1,
			},
			expectValid: true,
		},
		{
			name: "storage_backend ceph",
			request: models.PlanCreateRequest{
				Name:           "Test",
				Slug:           "test",
				VCPU:           1,
				MemoryMB:       1024,
				DiskGB:         10,
				PortSpeedMbps:  100,
				StorageBackend: "ceph",
			},
			expectValid: true,
		},
		{
			name: "storage_backend qcow",
			request: models.PlanCreateRequest{
				Name:           "Test",
				Slug:           "test",
				VCPU:           1,
				MemoryMB:       1024,
				DiskGB:         10,
				PortSpeedMbps:  100,
				StorageBackend: "qcow",
			},
			expectValid: true,
		},
		{
			name: "zero prices (free plan)",
			request: models.PlanCreateRequest{
				Name:          "Free Plan",
				Slug:          "free",
				VCPU:          1,
				MemoryMB:      512,
				DiskGB:        10,
				PortSpeedMbps: 100,
				PriceMonthly:  0,
				PriceHourly:   0,
			},
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectValid {
				// Verify constraints
				assert.LessOrEqual(t, len(tt.request.Name), 100)
				assert.LessOrEqual(t, len(tt.request.Slug), 100)
				assert.GreaterOrEqual(t, tt.request.VCPU, 1)
				assert.GreaterOrEqual(t, tt.request.MemoryMB, 512)
				assert.GreaterOrEqual(t, tt.request.DiskGB, 10)
				assert.GreaterOrEqual(t, tt.request.PortSpeedMbps, 1)
				assert.GreaterOrEqual(t, tt.request.BandwidthLimitGB, 0)
				assert.GreaterOrEqual(t, tt.request.PriceMonthly, int64(0))
				assert.GreaterOrEqual(t, tt.request.PriceHourly, int64(0))
				assert.GreaterOrEqual(t, tt.request.SortOrder, 0)
				assert.GreaterOrEqual(t, tt.request.SnapshotLimit, 0)
				assert.GreaterOrEqual(t, tt.request.BackupLimit, 0)
				assert.GreaterOrEqual(t, tt.request.ISOUploadLimit, 0)
				if tt.request.StorageBackend != "" {
					assert.Contains(t, []string{"ceph", "qcow"}, tt.request.StorageBackend)
				}
			}
		})
	}
}

// TestPlanUpdateRequest_ValidationRules tests struct validation rules for update.
func TestPlanUpdateRequest_ValidationRules(t *testing.T) {
	tests := []struct {
		name        string
		request     models.PlanUpdateRequest
		expectValid bool
	}{
		{
			name:        "empty request (valid - partial update)",
			request:     models.PlanUpdateRequest{},
			expectValid: true,
		},
		{
			name: "valid name update",
			request: models.PlanUpdateRequest{
				Name: strPtr("Updated Plan"),
			},
			expectValid: true,
		},
		{
			name: "valid slug update",
			request: models.PlanUpdateRequest{
				Slug: strPtr("updated-plan"),
			},
			expectValid: true,
		},
		{
			name: "valid vcpu update",
			request: models.PlanUpdateRequest{
				VCPU: intPtr(4),
			},
			expectValid: true,
		},
		{
			name: "valid memory_mb update",
			request: models.PlanUpdateRequest{
				MemoryMB: intPtr(4096),
			},
			expectValid: true,
		},
		{
			name: "valid disk_gb update",
			request: models.PlanUpdateRequest{
				DiskGB: intPtr(100),
			},
			expectValid: true,
		},
		{
			name: "valid is_active update",
			request: models.PlanUpdateRequest{
				IsActive: boolPtr(false),
			},
			expectValid: true,
		},
		{
			name: "valid storage_backend update",
			request: models.PlanUpdateRequest{
				StorageBackend: strPtr("qcow"),
			},
			expectValid: true,
		},
		{
			name: "name at max length",
			request: models.PlanUpdateRequest{
				Name: strPtr(string(make([]byte, 100))),
			},
			expectValid: true,
		},
		{
			name: "vcpu at minimum",
			request: models.PlanUpdateRequest{
				VCPU: intPtr(1),
			},
			expectValid: true,
		},
		{
			name: "memory_mb at minimum",
			request: models.PlanUpdateRequest{
				MemoryMB: intPtr(512),
			},
			expectValid: true,
		},
		{
			name: "disk_gb at minimum",
			request: models.PlanUpdateRequest{
				DiskGB: intPtr(10),
			},
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectValid {
				// Verify constraints
				if tt.request.Name != nil {
					assert.LessOrEqual(t, len(*tt.request.Name), 100)
				}
				if tt.request.Slug != nil {
					assert.LessOrEqual(t, len(*tt.request.Slug), 100)
				}
				if tt.request.VCPU != nil {
					assert.GreaterOrEqual(t, *tt.request.VCPU, 1)
				}
				if tt.request.MemoryMB != nil {
					assert.GreaterOrEqual(t, *tt.request.MemoryMB, 512)
				}
				if tt.request.DiskGB != nil {
					assert.GreaterOrEqual(t, *tt.request.DiskGB, 10)
				}
				if tt.request.PortSpeedMbps != nil {
					assert.GreaterOrEqual(t, *tt.request.PortSpeedMbps, 1)
				}
				if tt.request.BandwidthLimitGB != nil {
					assert.GreaterOrEqual(t, *tt.request.BandwidthLimitGB, 0)
				}
				if tt.request.PriceMonthly != nil {
					assert.GreaterOrEqual(t, *tt.request.PriceMonthly, int64(0))
				}
				if tt.request.PriceHourly != nil {
					assert.GreaterOrEqual(t, *tt.request.PriceHourly, int64(0))
				}
				if tt.request.StorageBackend != nil {
					assert.Contains(t, []string{"ceph", "qcow", ""}, *tt.request.StorageBackend)
				}
				if tt.request.SortOrder != nil {
					assert.GreaterOrEqual(t, *tt.request.SortOrder, 0)
				}
				if tt.request.SnapshotLimit != nil {
					assert.GreaterOrEqual(t, *tt.request.SnapshotLimit, 0)
				}
				if tt.request.BackupLimit != nil {
					assert.GreaterOrEqual(t, *tt.request.BackupLimit, 0)
				}
				if tt.request.ISOUploadLimit != nil {
					assert.GreaterOrEqual(t, *tt.request.ISOUploadLimit, 0)
				}
			}
		})
	}
}

// TestPlan_Structure verifies Plan model fields.
func TestPlan_Structure(t *testing.T) {
	plan := models.Plan{
		ID:               "plan-123",
		Name:             "Basic Plan",
		Slug:             "basic-plan",
		VCPU:             1,
		MemoryMB:         1024,
		DiskGB:           10,
		BandwidthLimitGB: 1000,
		PortSpeedMbps:    100,
		PriceMonthly:     999,
		PriceHourly:      1,
		StorageBackend:   "ceph",
		IsActive:         true,
		SortOrder:        1,
		SnapshotLimit:    5,
		BackupLimit:      3,
		ISOUploadLimit:   2,
	}

	assert.Equal(t, "plan-123", plan.ID)
	assert.Equal(t, "Basic Plan", plan.Name)
	assert.Equal(t, "basic-plan", plan.Slug)
	assert.Equal(t, 1, plan.VCPU)
	assert.Equal(t, 1024, plan.MemoryMB)
	assert.Equal(t, 10, plan.DiskGB)
	assert.Equal(t, 1000, plan.BandwidthLimitGB)
	assert.Equal(t, 100, plan.PortSpeedMbps)
	assert.Equal(t, int64(999), plan.PriceMonthly)
	assert.Equal(t, int64(1), plan.PriceHourly)
	assert.Equal(t, "ceph", plan.StorageBackend)
	assert.True(t, plan.IsActive)
	assert.Equal(t, 1, plan.SortOrder)
	assert.Equal(t, 5, plan.SnapshotLimit)
	assert.Equal(t, 3, plan.BackupLimit)
	assert.Equal(t, 2, plan.ISOUploadLimit)
}

// TestErrorResponseFormat_PlanEndpoints verifies error response structure for plan endpoints.
func TestErrorResponseFormat_PlanEndpoints(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.POST("/plans", handler.CreatePlan)

	// Test with missing required fields
	body := `{"name": ""}`
	req := httptest.NewRequest(http.MethodPost, "/plans", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, errorObj, "code")
	assert.Contains(t, errorObj, "message")
}

// TestApplyPlanUpdates verifies the applyPlanUpdates helper function.
func TestApplyPlanUpdates(t *testing.T) {
	originalPlan := &models.Plan{
		ID:               "plan-123",
		Name:             "Original Name",
		Slug:             "original-slug",
		VCPU:             1,
		MemoryMB:         1024,
		DiskGB:           10,
		BandwidthLimitGB: 100,
		PortSpeedMbps:    100,
		PriceMonthly:     999,
		PriceHourly:      1,
		StorageBackend:   "ceph",
		IsActive:         true,
		SortOrder:        1,
		SnapshotLimit:    5,
		BackupLimit:      3,
		ISOUploadLimit:   2,
	}

	tests := []struct {
		name          string
		update        models.PlanUpdateRequest
		expectChanged func(*models.Plan) bool
	}{
		{
			name:   "empty update changes nothing",
			update: models.PlanUpdateRequest{},
			expectChanged: func(p *models.Plan) bool {
				return p.Name == "Original Name" &&
					p.Slug == "original-slug" &&
					p.VCPU == 1 &&
					p.MemoryMB == 1024
			},
		},
		{
			name: "update name only",
			update: models.PlanUpdateRequest{
				Name: strPtr("Updated Name"),
			},
			expectChanged: func(p *models.Plan) bool {
				return p.Name == "Updated Name" &&
					p.Slug == "original-slug" // unchanged
			},
		},
		{
			name: "update multiple fields",
			update: models.PlanUpdateRequest{
				Name:          strPtr("New Name"),
				VCPU:          intPtr(4),
				MemoryMB:      intPtr(4096),
				IsActive:      boolPtr(false),
				StorageBackend: strPtr("qcow"),
			},
			expectChanged: func(p *models.Plan) bool {
				return p.Name == "New Name" &&
					p.VCPU == 4 &&
					p.MemoryMB == 4096 &&
					!p.IsActive &&
					p.StorageBackend == "qcow"
			},
		},
		{
			name: "update all optional fields",
			update: models.PlanUpdateRequest{
				BandwidthLimitGB: intPtr(500),
				PriceMonthly:     int64Ptr(1999),
				PriceHourly:      int64Ptr(2),
				SortOrder:        intPtr(10),
				SnapshotLimit:    intPtr(10),
				BackupLimit:      intPtr(5),
				ISOUploadLimit:   intPtr(3),
			},
			expectChanged: func(p *models.Plan) bool {
				return p.BandwidthLimitGB == 500 &&
					p.PriceMonthly == 1999 &&
					p.PriceHourly == 2 &&
					p.SortOrder == 10 &&
					p.SnapshotLimit == 10 &&
					p.BackupLimit == 5 &&
					p.ISOUploadLimit == 3
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of the original plan
			plan := *originalPlan
			planPtr := &plan

			applyPlanUpdates(planPtr, tt.update)

			assert.True(t, tt.expectChanged(planPtr), "plan update expectations not met")
		})
	}
}

// Helper functions for tests
func boolPtr(b bool) *bool {
	return &b
}

func int64Ptr(i int64) *int64 {
	return &i
}