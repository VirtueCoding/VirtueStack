package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSettingsDB implements repository.DB for test audit repo usage.
type mockSettingsDB struct {
	execFunc func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (m *mockSettingsDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row { return nil }
func (m *mockSettingsDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}
func (m *mockSettingsDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}
func (m *mockSettingsDB) Begin(_ context.Context) (pgx.Tx, error) { return nil, nil }

func TestIsSensitiveSetting(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"smtp_password is sensitive", "smtp_password", true},
		{"jwt_secret is sensitive", "jwt_secret", true},
		{"maintenance_mode is not sensitive", "maintenance_mode", false},
		{"empty string is not sensitive", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isSensitiveSetting(tt.key))
		})
	}
}

func TestSettingValidators_MaintenanceMode(t *testing.T) {
	v := settingValidators["maintenance_mode"]
	require.NotNil(t, v)

	tests := []struct {
		value   string
		wantErr bool
	}{
		{"true", false},
		{"false", false},
		{"yes", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := v(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSettingValidators_DefaultBackupRetentionDays(t *testing.T) {
	v := settingValidators["default_backup_retention_days"]
	require.NotNil(t, v)

	tests := []struct {
		value   string
		wantErr bool
	}{
		{"1", false},
		{"365", false},
		{"0", true},
		{"366", true},
		{"abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := v(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSettingValidators_MaxVMsPerCustomer(t *testing.T) {
	v := settingValidators["max_vms_per_customer"]
	require.NotNil(t, v)

	tests := []struct {
		value   string
		wantErr bool
	}{
		{"1", false},
		{"10000", false},
		{"0", true},
		{"10001", true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := v(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSettingValidators_BandwidthOverageRate(t *testing.T) {
	v := settingValidators["bandwidth_overage_rate"]
	require.NotNil(t, v)

	tests := []struct {
		value   string
		wantErr bool
	}{
		{"0", false},
		{"0.05", false},
		{"-1", true},
		{"abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := v(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSettingValidators_SMTPPort(t *testing.T) {
	v := settingValidators["smtp_port"]
	require.NotNil(t, v)

	tests := []struct {
		value   string
		wantErr bool
	}{
		{"1", false},
		{"65535", false},
		{"0", true},
		{"65536", true},
		{"abc", true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := v(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSettingValidators_NodeHeartbeatTimeoutSeconds(t *testing.T) {
	v := settingValidators["node_heartbeat_timeout_seconds"]
	require.NotNil(t, v)

	tests := []struct {
		value   string
		wantErr bool
	}{
		{"10", false},
		{"86400", false},
		{"9", true},
		{"86401", true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := v(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSettingValidators_BackupScheduleHour(t *testing.T) {
	v := settingValidators["backup_schedule_hour"]
	require.NotNil(t, v)

	tests := []struct {
		value   string
		wantErr bool
	}{
		{"0", false},
		{"23", false},
		{"-1", true},
		{"24", true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			err := v(tt.value)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetSettings_DefaultsWhenRepoNil(t *testing.T) {
	router := setupAdminTestRouter()
	handler := &AdminHandler{
		settingsRepo: nil,
		logger:       testAdminLogger(),
	}

	router.GET("/settings", handler.GetSettings)

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, err := json.Marshal(resp.Data)
	require.NoError(t, err)

	var settings []Setting
	err = json.Unmarshal(data, &settings)
	require.NoError(t, err)

	assert.Equal(t, len(defaultSettings), len(settings))

	// Verify a known default value
	found := false
	for _, s := range settings {
		if s.Key == "maintenance_mode" {
			assert.Equal(t, "false", s.Value)
			found = true
			break
		}
	}
	assert.True(t, found, "expected maintenance_mode in settings")
}

func TestUpdateSetting_InvalidKey(t *testing.T) {
	router := setupAdminTestRouter()
	handler := &AdminHandler{
		settingsRepo: nil,
		logger:       testAdminLogger(),
	}

	router.PUT("/settings/:key", handler.UpdateSetting)

	body, _ := json.Marshal(SettingUpdateRequest{Value: "true"})
	req := httptest.NewRequest(http.MethodPut, "/settings/nonexistent_key", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "SETTING_NOT_FOUND", errorObj["code"])
}

func TestUpdateSetting_InvalidBody(t *testing.T) {
	router := setupAdminTestRouter()
	handler := &AdminHandler{
		settingsRepo: nil,
		logger:       testAdminLogger(),
	}

	router.PUT("/settings/:key", handler.UpdateSetting)

	req := httptest.NewRequest(http.MethodPut, "/settings/maintenance_mode", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateSetting_ValidMaintenanceMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	mockDB := &mockSettingsDB{}
	auditRepo := repository.NewAuditRepository(mockDB)

	handler := &AdminHandler{
		settingsRepo: nil,
		auditRepo:    auditRepo,
		logger:       testAdminLogger(),
	}

	router.PUT("/settings/:key", handler.UpdateSetting)

	body, _ := json.Marshal(SettingUpdateRequest{Value: "true"})
	req := httptest.NewRequest(http.MethodPut, "/settings/maintenance_mode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp models.Response
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, err := json.Marshal(resp.Data)
	require.NoError(t, err)

	var setting Setting
	err = json.Unmarshal(data, &setting)
	require.NoError(t, err)
	assert.Equal(t, "maintenance_mode", setting.Key)
	assert.Equal(t, "true", setting.Value)
}

func TestUpdateSetting_InvalidMaintenanceModeValue(t *testing.T) {
	router := setupAdminTestRouter()
	handler := &AdminHandler{
		settingsRepo: nil,
		logger:       testAdminLogger(),
	}

	router.PUT("/settings/:key", handler.UpdateSetting)

	body, _ := json.Marshal(SettingUpdateRequest{Value: "yes"})
	req := httptest.NewRequest(http.MethodPut, "/settings/maintenance_mode", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_SETTING_VALUE", errorObj["code"])
}
