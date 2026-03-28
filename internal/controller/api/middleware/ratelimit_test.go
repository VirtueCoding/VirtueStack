package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mockRedisRateLimitClient struct {
	result any
	err    error
	calls  int
}

func (m *mockRedisRateLimitClient) Eval(context.Context, string, []string, ...any) (any, error) {
	m.calls++
	return m.result, m.err
}

func TestValidateDistributedRateLimitConfiguration(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateDistributedRateLimitConfiguration(false, false))
	require.NoError(t, ValidateDistributedRateLimitConfiguration(true, true))
	require.Error(t, ValidateDistributedRateLimitConfiguration(true, false))
}

func TestSelectedRateLimitUsesRedisBackendWhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	client := &mockRedisRateLimitClient{result: int64(4)}
	ConfigureDistributedRateLimitBackend(client)
	t.Cleanup(func() {
		ConfigureDistributedRateLimitBackend(nil)
	})

	router := gin.New()
	router.Use(LoginRateLimit())
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, 1, client.calls)
}

func TestSelectedRateLimitFallsBackToInMemoryLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ConfigureDistributedRateLimitBackend(nil)

	router := gin.New()
	router.Use(selectedRateLimit(RateLimitConfig{
		Requests: 1,
		Window:   time.Minute,
		KeyFunc:  keyByIP,
	}, "ratelimit:test:"))
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	firstReq := httptest.NewRequest(http.MethodGet, "/", nil)
	firstW := httptest.NewRecorder()
	router.ServeHTTP(firstW, firstReq)
	require.Equal(t, http.StatusNoContent, firstW.Code)

	secondReq := httptest.NewRequest(http.MethodGet, "/", nil)
	secondW := httptest.NewRecorder()
	router.ServeHTTP(secondW, secondReq)
	require.Equal(t, http.StatusTooManyRequests, secondW.Code)
}

func TestPasswordResetRateLimit_ForgotPasswordEmailLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ConfigureDistributedRateLimitBackend(nil)

	router := gin.New()
	router.Use(PasswordResetRateLimit())
	router.POST("/auth/forgot-password", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	makeReq := func(email string, ip string) int {
		body, err := json.Marshal(map[string]string{"email": email})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusNoContent, makeReq("user@example.com", "198.51.100.10:1234"))
	}
	require.Equal(t, http.StatusTooManyRequests, makeReq("user@example.com", "198.51.100.10:1234"))
	require.Equal(t, http.StatusNoContent, makeReq("other@example.com", "198.51.100.10:1234"))
}

func TestPasswordResetRateLimit_ResetPasswordUsesIPLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ConfigureDistributedRateLimitBackend(nil)

	router := gin.New()
	router.Use(PasswordResetRateLimit())
	router.POST("/auth/reset-password", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	makeReq := func(ip string) int {
		body, err := json.Marshal(map[string]string{
			"token":        "token-123",
			"new_password": "ValidPassword123!",
		})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	for i := 0; i < 10; i++ {
		require.Equal(t, http.StatusNoContent, makeReq("203.0.113.20:5678"))
	}
	require.Equal(t, http.StatusTooManyRequests, makeReq("203.0.113.20:5678"))
}
