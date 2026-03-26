package customer

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateBackup_InvalidBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"malformed JSON", `{invalid`},
		{"empty object", `{}`},
		{"missing vm_id", `{"name":"backup-1"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			handler := &CustomerHandler{logger: testAuthHandlerLogger()}

			router.Use(func(c *gin.Context) {
				c.Set("user_id", "test-customer-id")
				c.Next()
			})
			router.POST("/backups", handler.CreateBackup)

			req := httptest.NewRequest(http.MethodPost, "/backups", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]interface{})
			require.True(t, ok, "response should contain error object")
			assert.NotEmpty(t, errObj["code"])
		})
	}
}

func TestCreateBackup_InvalidVMUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{logger: testAuthHandlerLogger()}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.POST("/backups", handler.CreateBackup)

	body := `{"vm_id":"not-a-valid-uuid","name":"backup-1"}`
	req := httptest.NewRequest(http.MethodPost, "/backups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	// BindAndValidate catches the invalid uuid format via validate:"required,uuid"
	assert.NotEmpty(t, errObj["code"])
}

func TestGetBackup_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{logger: testAuthHandlerLogger()}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.GET("/backups/:id", handler.GetBackup)

	req := httptest.NewRequest(http.MethodGet, "/backups/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_BACKUP_ID", errObj["code"])
}

func TestDeleteBackup_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{logger: testAuthHandlerLogger()}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.DELETE("/backups/:id", handler.DeleteBackup)

	req := httptest.NewRequest(http.MethodDelete, "/backups/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_BACKUP_ID", errObj["code"])
}

func TestRestoreBackup_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{logger: testAuthHandlerLogger()}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.POST("/backups/:id/restore", handler.RestoreBackup)

	req := httptest.NewRequest(http.MethodPost, "/backups/not-a-uuid/restore", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_BACKUP_ID", errObj["code"])
}

func TestListBackups_InvalidStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{"unknown status", "invalid_status"},
		{"uppercase", "COMPLETED"},
		{"typo", "complted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			handler := &CustomerHandler{logger: testAuthHandlerLogger()}

			router.Use(func(c *gin.Context) {
				c.Set("user_id", "test-customer-id")
				c.Next()
			})
			router.GET("/backups", handler.ListBackups)

			req := httptest.NewRequest(http.MethodGet, "/backups?status="+tt.status, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, "INVALID_STATUS", errObj["code"])
		})
	}
}

func TestBackupUUIDValidation_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		reqPath string
		handler func(h *CustomerHandler) gin.HandlerFunc
		code    string
	}{
		{"GetBackup", http.MethodGet, "/backups/:id", "/backups/bad-id", func(h *CustomerHandler) gin.HandlerFunc { return h.GetBackup }, "INVALID_BACKUP_ID"},
		{"DeleteBackup", http.MethodDelete, "/backups/:id", "/backups/bad-id", func(h *CustomerHandler) gin.HandlerFunc { return h.DeleteBackup }, "INVALID_BACKUP_ID"},
		{"RestoreBackup", http.MethodPost, "/backups/:id/restore", "/backups/bad-id/restore", func(h *CustomerHandler) gin.HandlerFunc { return h.RestoreBackup }, "INVALID_BACKUP_ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_invalid_uuid", func(t *testing.T) {
			router := setupTestRouter()
			handler := &CustomerHandler{logger: testAuthHandlerLogger()}

			router.Use(func(c *gin.Context) {
				c.Set("user_id", "test-customer-id")
				c.Next()
			})
			router.Handle(tt.method, tt.path, tt.handler(handler))

			req := httptest.NewRequest(tt.method, tt.reqPath, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, tt.code, errObj["code"])
		})
	}
}
