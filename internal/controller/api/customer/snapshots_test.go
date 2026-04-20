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

func TestCreateSnapshot_InvalidBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"malformed JSON", `{invalid`},
		{"empty object", `{}`},
		{"missing vm_id", `{"name":"snap-1"}`},
		{"missing name", `{"vm_id":"550e8400-e29b-41d4-a716-446655440000"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			handler := &CustomerHandler{logger: testAuthHandlerLogger()}

			router.Use(func(c *gin.Context) {
				c.Set("user_id", "test-customer-id")
				c.Next()
			})
			router.POST("/snapshots", handler.CreateSnapshot)

			req := httptest.NewRequest(http.MethodPost, "/snapshots", bytes.NewBufferString(tt.body))
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

func TestCreateSnapshot_InvalidVMUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{logger: testAuthHandlerLogger()}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.POST("/snapshots", handler.CreateSnapshot)

	body := `{"vm_id":"not-a-valid-uuid","name":"snap-1"}`
	req := httptest.NewRequest(http.MethodPost, "/snapshots", bytes.NewBufferString(body))
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

func TestDeleteSnapshot_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{logger: testAuthHandlerLogger()}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.DELETE("/snapshots/:id", handler.DeleteSnapshot)

	req := httptest.NewRequest(http.MethodDelete, "/snapshots/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_SNAPSHOT_ID", errObj["code"])
}

func TestRestoreSnapshot_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{logger: testAuthHandlerLogger()}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.POST("/snapshots/:id/restore", handler.RestoreSnapshot)

	req := httptest.NewRequest(http.MethodPost, "/snapshots/not-a-uuid/restore", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_SNAPSHOT_ID", errObj["code"])
}

func TestSnapshotUUIDValidation_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		reqPath string
		handler func(h *CustomerHandler) gin.HandlerFunc
		code    string
	}{
		{"DeleteSnapshot", http.MethodDelete, "/snapshots/:id", "/snapshots/bad-id", func(h *CustomerHandler) gin.HandlerFunc { return h.DeleteSnapshot }, "INVALID_SNAPSHOT_ID"},
		{"RestoreSnapshot", http.MethodPost, "/snapshots/:id/restore", "/snapshots/bad-id/restore", func(h *CustomerHandler) gin.HandlerFunc { return h.RestoreSnapshot }, "INVALID_SNAPSHOT_ID"},
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
