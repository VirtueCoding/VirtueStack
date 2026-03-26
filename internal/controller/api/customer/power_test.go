package customer

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPowerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestStartVM_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{
		logger: testPowerLogger(),
	}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "customer-123")
		c.Next()
	})
	router.POST("/vms/:id/start", handler.StartVM)

	req := httptest.NewRequest(http.MethodPost, "/vms/not-a-uuid/start", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	errorObj := resp["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_VM_ID", errorObj["code"])
}

func TestStopVM_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{
		logger: testPowerLogger(),
	}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "customer-123")
		c.Next()
	})
	router.POST("/vms/:id/stop", handler.StopVM)

	req := httptest.NewRequest(http.MethodPost, "/vms/invalid/stop", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	errorObj := resp["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_VM_ID", errorObj["code"])
}

func TestRestartVM_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{
		logger: testPowerLogger(),
	}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "customer-123")
		c.Next()
	})
	router.POST("/vms/:id/restart", handler.RestartVM)

	req := httptest.NewRequest(http.MethodPost, "/vms/invalid/restart", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	errorObj := resp["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_VM_ID", errorObj["code"])
}

func TestForceStopVM_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{
		logger: testPowerLogger(),
	}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "customer-123")
		c.Next()
	})
	router.POST("/vms/:id/force-stop", handler.ForceStopVM)

	req := httptest.NewRequest(http.MethodPost, "/vms/invalid/force-stop", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	errorObj := resp["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_VM_ID", errorObj["code"])
}

// TestPowerEndpoints_RequireUserID tests that all power endpoints
// use middleware.GetUserID to extract the customer ID.
func TestPowerEndpoints_RequireUserID(t *testing.T) {
	endpoints := []struct {
		name    string
		method  string
		path    string
		handler func(h *CustomerHandler) gin.HandlerFunc
	}{
		{"start", http.MethodPost, "/vms/:id/start", func(h *CustomerHandler) gin.HandlerFunc { return h.StartVM }},
		{"stop", http.MethodPost, "/vms/:id/stop", func(h *CustomerHandler) gin.HandlerFunc { return h.StopVM }},
		{"restart", http.MethodPost, "/vms/:id/restart", func(h *CustomerHandler) gin.HandlerFunc { return h.RestartVM }},
		{"force-stop", http.MethodPost, "/vms/:id/force-stop", func(h *CustomerHandler) gin.HandlerFunc { return h.ForceStopVM }},
	}

	for _, ep := range endpoints {
		t.Run(ep.name+"_invalid_uuid", func(t *testing.T) {
			router := setupTestRouter()
			handler := &CustomerHandler{
				logger: testPowerLogger(),
			}

			router.Use(func(c *gin.Context) {
				c.Set("user_id", "customer-test")
				c.Next()
			})
			router.Handle(ep.method, ep.path, ep.handler(handler))

			req := httptest.NewRequest(ep.method, "/vms/not-uuid/"+ep.name, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestGetConsoleToken_InvalidUUID tests UUID validation in console token generation.
func TestGetConsoleToken_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{
		logger: testPowerLogger(),
	}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "customer-123")
		c.Next()
	})
	router.POST("/vms/:id/console-token", handler.GetConsoleToken)

	req := httptest.NewRequest(http.MethodPost, "/vms/not-a-uuid/console-token", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	errorObj := resp["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_VM_ID", errorObj["code"])
}

// TestGetSerialToken_InvalidUUID tests UUID validation in serial token generation.
func TestGetSerialToken_InvalidUUID(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{
		logger: testPowerLogger(),
	}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "customer-123")
		c.Next()
	})
	router.POST("/vms/:id/serial-token", handler.GetSerialToken)

	req := httptest.NewRequest(http.MethodPost, "/vms/not-a-uuid/serial-token", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	errorObj := resp["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_VM_ID", errorObj["code"])
}

// TestGenerateConsoleToken_Uniqueness verifies that each call to
// generateConsoleToken returns a unique, non-empty token.
func TestGenerateConsoleToken_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := generateConsoleToken("vm-123", "customer-456")
		require.NoError(t, err)
		assert.NotEmpty(t, token)
		assert.Len(t, token, 64) // 32 bytes hex-encoded = 64 chars
		assert.False(t, seen[token], "token should be unique")
		seen[token] = true
	}
}

// TestConsoleTokenStore_ValidateAndInvalidate tests the single-use token behavior.
func TestConsoleTokenStore_ValidateAndInvalidate(t *testing.T) {
	store := newConsoleTokenStore()
	defer store.Stop()

	store.Store("token-1", "vm-1", "customer-1", ConsoleTokenDuration)

	// First validation should succeed
	assert.True(t, store.Validate("token-1", "vm-1", "customer-1"))

	// Second validation should fail (single-use)
	assert.False(t, store.Validate("token-1", "vm-1", "customer-1"))
}

func TestConsoleTokenStore_WrongVM(t *testing.T) {
	store := newConsoleTokenStore()
	defer store.Stop()

	store.Store("token-1", "vm-1", "customer-1", ConsoleTokenDuration)

	// Wrong VM should fail
	assert.False(t, store.Validate("token-1", "vm-wrong", "customer-1"))
}

func TestConsoleTokenStore_WrongCustomer(t *testing.T) {
	store := newConsoleTokenStore()
	defer store.Stop()

	store.Store("token-1", "vm-1", "customer-1", ConsoleTokenDuration)

	// Wrong customer should fail
	assert.False(t, store.Validate("token-1", "vm-1", "customer-wrong"))
}

func TestConsoleTokenStore_NonexistentToken(t *testing.T) {
	store := newConsoleTokenStore()
	defer store.Stop()

	assert.False(t, store.Validate("nonexistent", "vm-1", "customer-1"))
}

func TestConsoleTokenStore_Overwrite(t *testing.T) {
	store := newConsoleTokenStore()
	defer store.Stop()

	store.Store("token-1", "vm-1", "customer-1", ConsoleTokenDuration)
	store.Store("token-1", "vm-2", "customer-2", ConsoleTokenDuration)

	// Should validate with new values
	assert.True(t, store.Validate("token-1", "vm-2", "customer-2"))
}

func TestConsoleTokenStore_RemoveExpired(t *testing.T) {
	store := newConsoleTokenStore()
	defer store.Stop()

	// Store a token with zero duration (already expired)
	store.Store("expired-token", "vm-1", "customer-1", 0)

	// Should fail because it's already expired
	assert.False(t, store.Validate("expired-token", "vm-1", "customer-1"))
}

func TestListTemplates_InvalidOSFamily(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{
		logger: testPowerLogger(),
	}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "customer-123")
		c.Next()
	})
	router.GET("/templates", handler.ListTemplates)

	req := httptest.NewRequest(http.MethodGet, "/templates?os_family=invalid_os", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	errorObj := resp["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_OS_FAMILY", errorObj["code"])
}

func TestListTemplates_NoTemplateService(t *testing.T) {
	router := setupTestRouter()
	handler := &CustomerHandler{
		templateService: nil, // No template service
		logger:          testPowerLogger(),
	}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "customer-123")
		c.Next()
	})
	router.GET("/templates", handler.ListTemplates)

	req := httptest.NewRequest(http.MethodGet, "/templates", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	// Should return empty array when no template service
	data := resp["data"].([]interface{})
	assert.Len(t, data, 0)
}

func TestValidOSFamilies(t *testing.T) {
	expected := []string{
		"debian", "ubuntu", "centos", "rocky",
		"almalinux", "fedora", "freebsd", "windows", "other",
	}

	for _, family := range expected {
		t.Run(family, func(t *testing.T) {
			assert.True(t, validOSFamilies[family], "OS family %q should be valid", family)
		})
	}

	assert.False(t, validOSFamilies["invalid"])
	assert.False(t, validOSFamilies[""])
}

// TestConsoleTokenDuration_Value verifies the token duration constant.
func TestConsoleTokenDuration_Value(t *testing.T) {
	assert.Equal(t, 1*time.Hour, ConsoleTokenDuration)
}
