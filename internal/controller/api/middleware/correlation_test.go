package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestIsValidCorrelationID(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		valid bool
	}{
		{"valid UUID", "550e8400-e29b-41d4-a716-446655440000", true},
		{"alphanumeric short", "req-abc-123", true},
		{"single char", "a", true},
		{"64 char alphanumeric", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"65 chars too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaab", false},
		{"empty string", "", false},
		{"contains spaces", "req abc", false},
		{"SQL injection", "'; DROP TABLE vms; --", false},
		{"contains newline", "req\nabc", false},
		{"underscores allowed", "req_abc_123", true},
		{"hyphens allowed", "req-abc-123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidCorrelationID(tt.id))
		})
	}
}

func TestCorrelationID_Middleware(t *testing.T) {
	t.Run("generates new ID when header is missing", func(t *testing.T) {
		r := gin.New()
		r.Use(CorrelationID())
		r.GET("/test", func(c *gin.Context) {
			id := GetCorrelationID(c)
			assert.NotEmpty(t, id)
			c.JSON(http.StatusOK, gin.H{"id": id})
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		// Response should include the correlation ID header
		assert.NotEmpty(t, w.Header().Get(CorrelationIDHeader))
	})

	t.Run("propagates valid existing header", func(t *testing.T) {
		r := gin.New()
		r.Use(CorrelationID())
		r.GET("/test", func(c *gin.Context) {
			id := GetCorrelationID(c)
			assert.Equal(t, "my-trace-id", id)
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(CorrelationIDHeader, "my-trace-id")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "my-trace-id", w.Header().Get(CorrelationIDHeader))
	})

	t.Run("rejects invalid correlation ID and generates new one", func(t *testing.T) {
		r := gin.New()
		r.Use(CorrelationID())
		r.GET("/test", func(c *gin.Context) {
			id := GetCorrelationID(c)
			// Should not be the injected value
			assert.NotEqual(t, "'; DROP TABLE vms; --", id)
			assert.NotEmpty(t, id)
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(CorrelationIDHeader, "'; DROP TABLE vms; --")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("propagates valid UUID in header", func(t *testing.T) {
		r := gin.New()
		r.Use(CorrelationID())
		r.GET("/test", func(c *gin.Context) {
			id := GetCorrelationID(c)
			assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", id)
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(CorrelationIDHeader, "550e8400-e29b-41d4-a716-446655440000")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("stores correlation ID on request context", func(t *testing.T) {
		r := gin.New()
		r.Use(CorrelationID())
		r.GET("/test", func(c *gin.Context) {
			assert.Equal(t, "my-trace-id", GetCorrelationIDFromContext(c.Request.Context()))
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(CorrelationIDHeader, "my-trace-id")
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestGetCorrelationID(t *testing.T) {
	t.Run("returns empty string when not set", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		assert.Empty(t, GetCorrelationID(c))
	})

	t.Run("returns value when set", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(CorrelationIDContextKey, "test-id")
		assert.Equal(t, "test-id", GetCorrelationID(c))
	})

	t.Run("returns empty string for non-string value", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(CorrelationIDContextKey, 12345)
		assert.Empty(t, GetCorrelationID(c))
	})
}
