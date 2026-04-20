package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecovery_NoPanic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	r := gin.New()
	r.Use(Recovery(logger))
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRecovery_PanicRecovered(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	r := gin.New()
	r.Use(CorrelationID()) // Add correlation ID for the error response
	r.Use(Recovery(logger))
	r.GET("/panic", func(_ *gin.Context) {
		panic("something went wrong")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "internal error")
	assert.NotEmpty(t, resp.Error.CorrelationID)
}

func TestRespondWithError(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		code       string
		message    string
		wantStatus int
	}{
		{
			name:       "bad request",
			status:     http.StatusBadRequest,
			code:       "VALIDATION_ERROR",
			message:    "Invalid hostname",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not found",
			status:     http.StatusNotFound,
			code:       "NOT_FOUND",
			message:    "VM not found",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "internal error",
			status:     http.StatusInternalServerError,
			code:       "INTERNAL_ERROR",
			message:    "Something went wrong",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "forbidden",
			status:     http.StatusForbidden,
			code:       "FORBIDDEN",
			message:    "Access denied",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
			// Set a correlation ID
			c.Set(CorrelationIDContextKey, "test-correlation-123")

			RespondWithError(c, tt.status, tt.code, tt.message)

			assert.Equal(t, tt.wantStatus, w.Code)

			var resp ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.Equal(t, tt.code, resp.Error.Code)
			assert.Equal(t, tt.message, resp.Error.Message)
			assert.Equal(t, "test-correlation-123", resp.Error.CorrelationID)
		})
	}
}

func TestRespondWithError_WithoutCorrelationID(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	RespondWithError(c, http.StatusBadRequest, "TEST_ERROR", "Test message")

	var resp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "TEST_ERROR", resp.Error.Code)
	assert.Empty(t, resp.Error.CorrelationID)
}

func TestRespondWithError_AbortsContext(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	RespondWithError(c, http.StatusForbidden, "FORBIDDEN", "Access denied")
	assert.True(t, c.IsAborted())
}
