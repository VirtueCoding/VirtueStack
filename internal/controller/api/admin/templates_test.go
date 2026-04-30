package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistributeTemplate_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.POST("/templates/:id/distribute", handler.DistributeTemplate)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/templates/not-a-uuid/distribute",
		strings.NewReader(`{"node_ids":["550e8400-e29b-41d4-a716-446655440000"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_ID", errorObj["code"])
}

func TestDistributeTemplate_EmptyNodeIDs(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.POST("/templates/:id/distribute", handler.DistributeTemplate)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/templates/550e8400-e29b-41d4-a716-446655440000/distribute",
		strings.NewReader(`{"node_ids":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetTemplateCacheStatus_InvalidID(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		logger: logger,
	}

	router.GET("/templates/:id/cache-status", handler.GetTemplateCacheStatus)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/templates/not-a-uuid/cache-status", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "INVALID_ID", errorObj["code"])
}

func TestCreateTemplate_MinimalDialogPayloadPassesValidation(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/templates", bytes.NewBufferString(`{
		"name":"Current UI Template",
		"os_family":"debian"
	}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	var payload models.TemplateCreateRequest
	err := middleware.BindAndValidate(c, &payload)

	require.NoError(t, err)
	assert.Equal(t, "Current UI Template", payload.Name)
	assert.Equal(t, "debian", payload.OSFamily)
	assert.Empty(t, payload.OSVersion)
	assert.Empty(t, payload.RBDImage)
	assert.Empty(t, payload.RBDSnapshot)
	assert.Zero(t, payload.MinDiskGB)
	assert.Nil(t, payload.SupportsCloudInit)
	assert.Nil(t, payload.IsActive)
}
