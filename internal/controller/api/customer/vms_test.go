package customer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVM_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{logger: logger}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.GET("/vms/:id", handler.GetVM)

	req := httptest.NewRequest(http.MethodGet, "/vms/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_VM_ID", errObj["code"])
}

func TestListVMs_InvalidStatus(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{logger: logger}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.GET("/vms", handler.ListVMs)

	req := httptest.NewRequest(http.MethodGet, "/vms?status=invalid_status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_STATUS", errObj["code"])
}

func TestListVMs_SearchTooLong(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{logger: logger}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-customer-id")
		c.Next()
	})
	router.GET("/vms", handler.ListVMs)

	longSearch := strings.Repeat("a", 101)
	req := httptest.NewRequest(http.MethodGet, "/vms?search="+longSearch, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_SEARCH", errObj["code"])
}
