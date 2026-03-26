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

func TestCreateWebhook_InvalidBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"malformed JSON", `{invalid`},
		{"empty object", `{}`},
		{"missing url", `{"secret":"abcdefghijklmnop","events":["vm.started"]}`},
		{"missing secret", `{"url":"https://example.com/hook","events":["vm.started"]}`},
		{"missing events", `{"url":"https://example.com/hook","secret":"abcdefghijklmnop"}`},
		{"empty events array", `{"url":"https://example.com/hook","secret":"abcdefghijklmnop","events":[]}`},
		{"secret too short", `{"url":"https://example.com/hook","secret":"short","events":["vm.started"]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			handler := &CustomerHandler{logger: testAuthHandlerLogger()}

			router.Use(func(c *gin.Context) {
				c.Set("user_id", "test-customer-id")
				c.Next()
			})
			router.POST("/webhooks", handler.CreateWebhook)

			req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewBufferString(tt.body))
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

func TestGetWebhook_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{logger: testAuthHandlerLogger()}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.GET("/webhooks/:id", handler.GetWebhook)

	req := httptest.NewRequest(http.MethodGet, "/webhooks/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_WEBHOOK_ID", errObj["code"])
}

func TestUpdateWebhook_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{logger: testAuthHandlerLogger()}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.PUT("/webhooks/:id", handler.UpdateWebhook)

	body := `{"is_active": false}`
	req := httptest.NewRequest(http.MethodPut, "/webhooks/not-a-uuid", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_WEBHOOK_ID", errObj["code"])
}

func TestDeleteWebhook_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{logger: testAuthHandlerLogger()}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.DELETE("/webhooks/:id", handler.DeleteWebhook)

	req := httptest.NewRequest(http.MethodDelete, "/webhooks/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_WEBHOOK_ID", errObj["code"])
}

func TestListWebhookDeliveries_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{logger: testAuthHandlerLogger()}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.GET("/webhooks/:id/deliveries", handler.ListWebhookDeliveries)

	req := httptest.NewRequest(http.MethodGet, "/webhooks/not-a-uuid/deliveries", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_WEBHOOK_ID", errObj["code"])
}

func TestTestWebhook_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{logger: testAuthHandlerLogger()}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.POST("/webhooks/:id/test", handler.TestWebhook)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/not-a-uuid/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_WEBHOOK_ID", errObj["code"])
}

func TestWebhookUUIDValidation_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		reqPath string
		handler func(h *CustomerHandler) gin.HandlerFunc
	}{
		{"GetWebhook", http.MethodGet, "/webhooks/:id", "/webhooks/bad-id", func(h *CustomerHandler) gin.HandlerFunc { return h.GetWebhook }},
		{"UpdateWebhook", http.MethodPut, "/webhooks/:id", "/webhooks/bad-id", func(h *CustomerHandler) gin.HandlerFunc { return h.UpdateWebhook }},
		{"DeleteWebhook", http.MethodDelete, "/webhooks/:id", "/webhooks/bad-id", func(h *CustomerHandler) gin.HandlerFunc { return h.DeleteWebhook }},
		{"ListDeliveries", http.MethodGet, "/webhooks/:id/deliveries", "/webhooks/bad-id/deliveries", func(h *CustomerHandler) gin.HandlerFunc { return h.ListWebhookDeliveries }},
		{"TestWebhook", http.MethodPost, "/webhooks/:id/test", "/webhooks/bad-id/test", func(h *CustomerHandler) gin.HandlerFunc { return h.TestWebhook }},
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

			var req *http.Request
			if tt.method == http.MethodPut {
				req = httptest.NewRequest(tt.method, tt.reqPath, bytes.NewBufferString(`{}`))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.reqPath, nil)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, "INVALID_WEBHOOK_ID", errObj["code"])
		})
	}
}
