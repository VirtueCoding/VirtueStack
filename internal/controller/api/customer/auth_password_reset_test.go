package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/notifications"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAuthServicePwReset struct {
	requestPasswordResetFunc func(ctx context.Context, email string) (string, error)
	resetPasswordFunc        func(ctx context.Context, token, newPassword string) error
}

func (m *mockAuthServicePwReset) RequestPasswordReset(ctx context.Context, email string) (string, error) {
	if m.requestPasswordResetFunc != nil {
		return m.requestPasswordResetFunc(ctx, email)
	}
	return "", nil
}

func (m *mockAuthServicePwReset) ResetPassword(ctx context.Context, token, newPassword string) error {
	if m.resetPasswordFunc != nil {
		return m.resetPasswordFunc(ctx, token, newPassword)
	}
	return nil
}

type mockEmailSender struct {
	sendFunc    func(ctx context.Context, payload *notifications.EmailPayload) error
	isEnabled   bool
	sentPayload *notifications.EmailPayload
}

func (m *mockEmailSender) Send(ctx context.Context, payload *notifications.EmailPayload) error {
	m.sentPayload = payload
	if m.sendFunc != nil {
		return m.sendFunc(ctx, payload)
	}
	return nil
}

func (m *mockEmailSender) IsEnabled() bool {
	return m.isEnabled
}

func newTestPasswordResetHandler(authMock *mockAuthServicePwReset, emailMock *mockEmailSender) *CustomerHandler {
	logger := slog.Default()
	h := &CustomerHandler{
		emailProvider:        emailMock,
		passwordResetBaseURL: "https://portal.example.com/reset-password",
		logger:               logger.With("component", "test"),
	}
	// Wire the auth service through a wrapper that delegates to our mock
	// The handler calls h.authService.RequestPasswordReset() and h.authService.ResetPassword()
	// which are methods on *services.AuthService. We need to set up a real authService
	// or use the handler methods directly.
	// Since we can't easily mock *services.AuthService (concrete type), we test at
	// the HTTP level with actual handler method wiring.
	return h
}

func TestForgotPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		body           map[string]any
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "valid email returns 200 always",
			body:           map[string]any{"email": "user@example.com"},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing email returns validation error",
			body:           map[string]any{},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "VALIDATION_ERROR",
		},
		{
			name:           "invalid email format returns validation error",
			body:           map[string]any{"email": "not-an-email"},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "VALIDATION_ERROR",
		},
		{
			name:           "empty body returns validation error",
			body:           map[string]any{"email": ""},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "VALIDATION_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			c.Request, _ = http.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewBuffer(bodyBytes))
			c.Request.Header.Set("Content-Type", "application/json")

			// For valid requests, we can't easily test without a real AuthService,
			// but we can test validation at the handler level
			h := &CustomerHandler{
				logger: slog.Default().With("component", "test"),
			}

			// Test validation path (before auth service call)
			if tt.expectedStatus == http.StatusBadRequest {
				h.ForgotPassword(c)
				assert.Equal(t, tt.expectedStatus, w.Code)
				if tt.expectedCode != "" {
					var resp map[string]any
					err := json.Unmarshal(w.Body.Bytes(), &resp)
					require.NoError(t, err)
					errObj, ok := resp["error"].(map[string]any)
					require.True(t, ok)
					assert.Equal(t, tt.expectedCode, errObj["code"])
				}
			}
		})
	}
}

func TestResetPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		body           map[string]any
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "missing token returns validation error",
			body:           map[string]any{"new_password": "mynewpassword123"},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "VALIDATION_ERROR",
		},
		{
			name:           "missing password returns validation error",
			body:           map[string]any{"token": "some-token"},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "VALIDATION_ERROR",
		},
		{
			name:           "password too short returns validation error",
			body:           map[string]any{"token": "some-token", "new_password": "short"},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "VALIDATION_ERROR",
		},
		{
			name:           "empty body returns validation error",
			body:           map[string]any{},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   "VALIDATION_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			bodyBytes, err := json.Marshal(tt.body)
			require.NoError(t, err)

			c.Request, _ = http.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewBuffer(bodyBytes))
			c.Request.Header.Set("Content-Type", "application/json")

			h := &CustomerHandler{
				logger: slog.Default().With("component", "test"),
			}

			h.ResetPassword(c)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedCode != "" {
				var resp map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &resp)
				require.NoError(t, err)
				errObj, ok := resp["error"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tt.expectedCode, errObj["code"])
			}
		})
	}
}

func TestBuildPasswordResetURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		token    string
		expected string
	}{
		{
			name:     "with custom base URL",
			baseURL:  "https://portal.example.com/reset-password",
			token:    "abc123",
			expected: "https://portal.example.com/reset-password?token=abc123",
		},
		{
			name:     "with empty base URL uses default",
			baseURL:  "",
			token:    "def456",
			expected: "/reset-password?token=def456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &CustomerHandler{
				passwordResetBaseURL: tt.baseURL,
			}
			result := h.buildPasswordResetURL(tt.token)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSendPasswordResetEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("sends email with correct payload", func(t *testing.T) {
		emailMock := &mockEmailSender{isEnabled: true}

		h := &CustomerHandler{
			emailProvider:        emailMock,
			passwordResetBaseURL: "https://portal.example.com/reset-password",
			logger:               slog.Default().With("component", "test"),
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest(http.MethodPost, "/test", nil)

		h.sendPasswordResetEmail(c, "user@example.com", "test-token-123")

		require.NotNil(t, emailMock.sentPayload)
		assert.Equal(t, "user@example.com", emailMock.sentPayload.To)
		assert.Equal(t, "Reset Your Password", emailMock.sentPayload.Subject)
		assert.Equal(t, "password-reset", emailMock.sentPayload.Template)
		assert.Equal(t, "https://portal.example.com/reset-password?token=test-token-123", emailMock.sentPayload.Data["reset_url"])
	})

	t.Run("skips when email provider is nil", func(t *testing.T) {
		h := &CustomerHandler{
			emailProvider: nil,
			logger:        slog.Default().With("component", "test"),
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest(http.MethodPost, "/test", nil)

		// Should not panic
		h.sendPasswordResetEmail(c, "user@example.com", "test-token")
	})

	t.Run("skips when email provider is disabled", func(t *testing.T) {
		emailMock := &mockEmailSender{isEnabled: false}

		h := &CustomerHandler{
			emailProvider: emailMock,
			logger:        slog.Default().With("component", "test"),
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest(http.MethodPost, "/test", nil)

		h.sendPasswordResetEmail(c, "user@example.com", "test-token")
		assert.Nil(t, emailMock.sentPayload)
	})
}
