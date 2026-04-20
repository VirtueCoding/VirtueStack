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

func TestSanitizeRequest(t *testing.T) {
	tests := []struct {
		name      string
		req       UpdateProfileRequest
		wantName  *string
		wantEmail *string
		wantPhone *string
	}{
		{
			name:      "trims name with spaces",
			req:       UpdateProfileRequest{Name: strPtrCustomer("  John Doe  ")},
			wantName:  strPtrCustomer("John Doe"),
			wantEmail: nil,
			wantPhone: nil,
		},
		{
			name:      "trims email with spaces",
			req:       UpdateProfileRequest{Email: strPtrCustomer("  user@example.com  ")},
			wantName:  nil,
			wantEmail: strPtrCustomer("user@example.com"),
			wantPhone: nil,
		},
		{
			name:      "trims phone with spaces",
			req:       UpdateProfileRequest{Phone: strPtrCustomer("  +1234567890  ")},
			wantName:  nil,
			wantEmail: nil,
			wantPhone: strPtrCustomer("+1234567890"),
		},
		{
			name:      "nil fields unchanged",
			req:       UpdateProfileRequest{},
			wantName:  nil,
			wantEmail: nil,
			wantPhone: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitizeRequest(&tt.req)
			assert.Equal(t, tt.wantName, tt.req.Name)
			assert.Equal(t, tt.wantEmail, tt.req.Email)
			assert.Equal(t, tt.wantPhone, tt.req.Phone)
		})
	}
}

func strPtrCustomer(s string) *string { return &s }

func TestUpdateProfile_NoUserID(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{logger: logger}
	router.PUT("/profile", handler.UpdateProfile)

	body := `{"name": "Test"}`
	req := httptest.NewRequest(http.MethodPut, "/profile", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "UNAUTHORIZED", errObj["code"])
}

func TestUpdateProfile_InvalidBody(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{logger: logger}
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-user-id")
		c.Next()
	})
	router.PUT("/profile", handler.UpdateProfile)

	req := httptest.NewRequest(http.MethodPut, "/profile", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateProfile_EmptyUpdate(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{logger: logger}
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-user-id")
		c.Next()
	})
	router.PUT("/profile", handler.UpdateProfile)

	body := `{}`
	req := httptest.NewRequest(http.MethodPut, "/profile", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
	assert.Contains(t, errObj["message"], "at least one field")
}

func TestGetProfile_NoUserID(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{logger: logger}
	router.GET("/profile", handler.GetProfile)

	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	errObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "UNAUTHORIZED", errObj["code"])
}
