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

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
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

	firstReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	firstW := httptest.NewRecorder()
	router.ServeHTTP(firstW, firstReq)
	require.Equal(t, http.StatusNoContent, firstW.Code)

	secondReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	secondW := httptest.NewRecorder()
	router.ServeHTTP(secondW, secondReq)
	require.Equal(t, http.StatusTooManyRequests, secondW.Code)
}

func TestSelectedRateLimitHandlesNilRedisClientValues(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		client RedisClient
	}{
		{
			name:   "nil interface",
			client: nil,
		},
		{
			name:   "typed nil pointer",
			client: (*mockRedisRateLimitClient)(nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ConfigureDistributedRateLimitBackend(tt.client)
			t.Cleanup(func() {
				ConfigureDistributedRateLimitBackend(nil)
			})

			router := gin.New()
			router.Use(selectedRateLimit(RateLimitConfig{
				Requests: 1,
				Window:   time.Minute,
				KeyFunc:  keyByIP,
			}, "ratelimit:test:"))
			router.GET("/", func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})

			firstReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			firstW := httptest.NewRecorder()
			router.ServeHTTP(firstW, firstReq)
			require.Equal(t, http.StatusNoContent, firstW.Code)

			secondReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			secondW := httptest.NewRecorder()
			router.ServeHTTP(secondW, secondReq)
			require.Equal(t, http.StatusTooManyRequests, secondW.Code)
		})
	}
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
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/forgot-password", bytes.NewReader(body))
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

func TestPasswordResetRateLimit_ForgotPasswordDoesNotRunHandlerAfterEmailLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ConfigureDistributedRateLimitBackend(nil)

	router := gin.New()
	handlerCalls := 0
	router.Use(PasswordResetRateLimit())
	router.POST("/auth/forgot-password", func(c *gin.Context) {
		handlerCalls++
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"message": "If an account with that email exists, a password reset link has been sent.",
			},
		})
	})

	makeReq := func(email string) *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]string{"email": email})
		require.NoError(t, err)
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/forgot-password", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.55:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	for i := 0; i < 3; i++ {
		resp := makeReq("user@example.com")
		require.Equal(t, http.StatusOK, resp.Code)
	}

	resp := makeReq("user@example.com")
	require.Equal(t, http.StatusTooManyRequests, resp.Code)
	require.Equal(t, 3, handlerCalls)
	require.NotContains(t, resp.Body.String(), `"data"`)
	require.Contains(t, resp.Body.String(), `"RATE_LIMIT_EXCEEDED"`)
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
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/auth/reset-password", bytes.NewReader(body))
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

func TestCustomerRateLimits_NotificationReadsDoNotConsumeGeneralReadQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ConfigureDistributedRateLimitBackend(nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "customer-1")
		c.Next()
	})
	router.Use(CustomerRateLimits())
	router.GET("/api/v1/customer/notifications/unread-count", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	router.GET("/api/v1/customer/vms/vm-1", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for i := 0; i < 100; i++ {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/customer/notifications/unread-count", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equalf(t, http.StatusNoContent, w.Code, "unread-count request %d should stay within its own bucket", i+1)
	}

	vmReq := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/customer/vms/vm-1", nil)
	vmResp := httptest.NewRecorder()
	router.ServeHTTP(vmResp, vmReq)
	require.Equal(t, http.StatusNoContent, vmResp.Code)
}
