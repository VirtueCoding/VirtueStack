package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	req := httptest.NewRequest(http.MethodPost, "/templates/not-a-uuid/distribute",
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

	req := httptest.NewRequest(http.MethodPost, "/templates/550e8400-e29b-41d4-a716-446655440000/distribute",
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

	req := httptest.NewRequest(http.MethodGet, "/templates/not-a-uuid/cache-status", nil)
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
