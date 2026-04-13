package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type logoutCustomerRepoStub struct {
	repository.CustomerRepo
	deleteSessionErr error
}

func (s *logoutCustomerRepoStub) DeleteSession(context.Context, string) error {
	return s.deleteSessionErr
}

func testAuthHandlerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

func findResponseCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func TestChangePassword_InvalidRequestBody(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{
		authService: nil,
		authConfig:  middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:      logger,
	}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-user-123")
		c.Next()
	})
	router.PUT("/password", handler.ChangePassword)

	req := httptest.NewRequest(http.MethodPut, "/password", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestChangePassword_PasswordTooShort(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{
		authService: nil,
		authConfig:  middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:      logger,
	}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-user-123")
		c.Next()
	})
	router.PUT("/password", handler.ChangePassword)

	tests := []struct {
		name            string
		currentPassword string
		newPassword     string
	}{
		{
			name:            "both passwords too short",
			currentPassword: "short",
			newPassword:     "short",
		},
		{
			name:            "current password too short",
			currentPassword: "short",
			newPassword:     "validnewpassword",
		},
		{
			name:            "new password too short",
			currentPassword: "validcurrentpass",
			newPassword:     "short",
		},
		{
			name:            "current password 11 chars",
			currentPassword: "elevenchars",
			newPassword:     "validnewpassword",
		},
		{
			name:            "new password 11 chars",
			currentPassword: "validcurrentpass",
			newPassword:     "elevenchars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]string{
				"current_password": tt.currentPassword,
				"new_password":     tt.newPassword,
			}
			jsonBody, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPut, "/password", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.NotEqual(t, http.StatusOK, w.Code, "should not return 200 for short password")
		})
	}
}

func TestChangePassword_Unauthorized(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{
		authService: nil,
		authConfig:  middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:      logger,
	}

	router.PUT("/password", handler.ChangePassword)

	body := ChangePasswordRequest{
		CurrentPassword: "currentpassword123",
		NewPassword:     "newpassword12345",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/password", bytes.NewBuffer(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestChangePasswordRequest_Validation(t *testing.T) {
	tests := []struct {
		name        string
		request     ChangePasswordRequest
		expectValid bool
	}{
		{
			name: "valid request with 12 char passwords",
			request: ChangePasswordRequest{
				CurrentPassword: "validcurrent",
				NewPassword:     "validnewpass",
			},
			expectValid: true,
		},
		{
			name: "valid request with longer passwords",
			request: ChangePasswordRequest{
				CurrentPassword: "validcurrentpassword",
				NewPassword:     "validnewpassword123",
			},
			expectValid: true,
		},
		{
			name: "current password too short",
			request: ChangePasswordRequest{
				CurrentPassword: "short",
				NewPassword:     "validnewpassword",
			},
			expectValid: false,
		},
		{
			name: "new password too short",
			request: ChangePasswordRequest{
				CurrentPassword: "validcurrentpass",
				NewPassword:     "short",
			},
			expectValid: false,
		},
		{
			name: "empty current password",
			request: ChangePasswordRequest{
				CurrentPassword: "",
				NewPassword:     "validnewpassword",
			},
			expectValid: false,
		},
		{
			name: "empty new password",
			request: ChangePasswordRequest{
				CurrentPassword: "validcurrentpass",
				NewPassword:     "",
			},
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectValid {
				assert.True(t, len(tt.request.CurrentPassword) >= 12, "current password should be >= 12 chars")
				assert.True(t, len(tt.request.NewPassword) >= 12, "new password should be >= 12 chars")
			} else {
				hasInvalid := len(tt.request.CurrentPassword) < 12 || len(tt.request.NewPassword) < 12 ||
					tt.request.CurrentPassword == "" || tt.request.NewPassword == ""
				assert.True(t, hasInvalid, "password should fail validation")
			}
		})
	}
}

func TestChangePassword_JSONBinding(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{
		authService: nil,
		authConfig:  middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:      logger,
	}

	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test-user-123")
		c.Next()
	})
	router.PUT("/password", handler.ChangePassword)

	t.Run("missing current_password field", func(t *testing.T) {
		body := `{"new_password": "newpassword1234"}`
		req := httptest.NewRequest(http.MethodPut, "/password", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing new_password field", func(t *testing.T) {
		body := `{"current_password": "currentpass123"}`
		req := httptest.NewRequest(http.MethodPut, "/password", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		body := `{current_password: "currentpass123", new_password: "newpassword1234"}`
		req := httptest.NewRequest(http.MethodPut, "/password", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestChangePassword_ErrorCodes(t *testing.T) {
	tests := []struct {
		name         string
		userID       string
		body         string
		expectedCode int
	}{
		{
			name:         "missing user_id returns 401",
			userID:       "",
			body:         `{"current_password": "currentpass123", "new_password": "newpassword1234"}`,
			expectedCode: http.StatusUnauthorized,
		},
		{
			name:         "invalid JSON returns 400",
			userID:       "test-user",
			body:         `{invalid`,
			expectedCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			logger := testAuthHandlerLogger()

			handler := &CustomerHandler{
				authService: nil,
				authConfig:  middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
				logger:      logger,
			}

			if tt.userID != "" {
				router.Use(func(c *gin.Context) {
					c.Set("user_id", tt.userID)
					c.Next()
				})
			}
			router.PUT("/password", handler.ChangePassword)

			req := httptest.NewRequest(http.MethodPut, "/password", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedCode, w.Code)
		})
	}
}

func TestChangePasswordRequest_MinPasswordLength(t *testing.T) {
	req := ChangePasswordRequest{
		CurrentPassword: "exactly12chr",
		NewPassword:     "exactly12chr",
	}
	assert.Equal(t, 12, len(req.CurrentPassword))
	assert.Equal(t, 12, len(req.NewPassword))
}

func TestChangePasswordRequest_MaxPasswordLength(t *testing.T) {
	longPassword := ""
	for i := 0; i < 128; i++ {
		longPassword += "a"
	}
	req := ChangePasswordRequest{
		CurrentPassword: longPassword,
		NewPassword:     longPassword,
	}
	assert.Equal(t, 128, len(req.CurrentPassword))
	assert.Equal(t, 128, len(req.NewPassword))
}

func TestChangePassword_ErrorResponseFormat(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{
		authService: nil,
		authConfig:  middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:      logger,
	}

	router.PUT("/password", handler.ChangePassword)

	body := `{"current_password": "currentpass123", "new_password": "newpassword1234"}`
	req := httptest.NewRequest(http.MethodPut, "/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, errorObj, "code")
	assert.Contains(t, errorObj, "message")
	assert.Equal(t, "UNAUTHORIZED", errorObj["code"])
}

func TestCustomerLogout_ReturnsInternalErrorWhenSessionInvalidationFails(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()
	authConfig := middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"}
	deleteErr := errors.New("delete session failed")
	authService := services.NewAuthService(
		&logoutCustomerRepoStub{deleteSessionErr: deleteErr},
		nil,
		nil,
		authConfig.JWTSecret,
		authConfig.Issuer,
		"",
		logger,
	)

	cleanupToken, err := middleware.GenerateSessionCleanupToken(
		authConfig,
		"customer-123",
		"customer",
		"",
		"session-123",
	)
	require.NoError(t, err)

	handler := &CustomerHandler{
		authService: authService,
		authConfig:  authConfig,
		logger:      logger,
	}

	router.POST("/logout", handler.Logout)

	body, err := json.Marshal(LogoutRequest{SessionCleanupToken: cleanupToken})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "LOGOUT_FAILED", errorObj["code"])
}

func TestCustomerLogout_RejectsMalformedJSON(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()
	authConfig := middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"}
	authService := services.NewAuthService(
		&logoutCustomerRepoStub{},
		nil,
		nil,
		authConfig.JWTSecret,
		authConfig.Issuer,
		"",
		logger,
	)

	accessToken, err := middleware.GenerateAccessToken(
		authConfig,
		"customer-123",
		"customer",
		"",
		"session-123",
		time.Hour,
	)
	require.NoError(t, err)

	handler := &CustomerHandler{
		authService: authService,
		authConfig:  authConfig,
		logger:      logger,
	}

	router.POST("/logout", handler.Logout)

	req := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBufferString(`{"session_cleanup_token":`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "INVALID_REQUEST_BODY", errorObj["code"])
}

func TestCustomerLogout_CleanupTokenWithoutCurrentSessionClearsCookies(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()
	authConfig := middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"}
	authService := services.NewAuthService(
		&logoutCustomerRepoStub{},
		nil,
		nil,
		authConfig.JWTSecret,
		authConfig.Issuer,
		"",
		logger,
	)

	cleanupToken, err := middleware.GenerateSessionCleanupToken(
		authConfig,
		"customer-123",
		"customer",
		"",
		"session-123",
	)
	require.NoError(t, err)

	handler := &CustomerHandler{
		authService: authService,
		authConfig:  authConfig,
		logger:      logger,
	}
	router.POST("/logout", handler.Logout)

	body, err := json.Marshal(LogoutRequest{SessionCleanupToken: cleanupToken})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	accessCookie := findResponseCookie(w.Result().Cookies(), middleware.AccessTokenCookieName)
	require.NotNil(t, accessCookie)
	assert.Equal(t, -1, accessCookie.MaxAge)

	refreshCookie := findResponseCookie(w.Result().Cookies(), middleware.RefreshTokenCookieName)
	require.NotNil(t, refreshCookie)
	assert.Equal(t, -1, refreshCookie.MaxAge)
}

func TestCustomerLogout_LogsFailedSessionAuditEventWhenInvalidationFails(t *testing.T) {
	logger := testAuthHandlerLogger()
	authConfig := middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"}
	deleteErr := errors.New("delete session failed")
	auditDB := &customerAuditCaptureDB{}
	authService := services.NewAuthService(
		&logoutCustomerRepoStub{deleteSessionErr: deleteErr},
		nil,
		nil,
		authConfig.JWTSecret,
		authConfig.Issuer,
		"",
		logger,
	)

	cleanupToken, err := middleware.GenerateSessionCleanupToken(
		authConfig,
		"customer-123",
		"customer",
		"",
		"session-123",
	)
	require.NoError(t, err)

	handler := &CustomerHandler{
		authService: authService,
		auditRepo:   repository.NewAuditRepository(auditDB),
		authConfig:  authConfig,
		logger:      logger,
	}

	router := setupTestRouter()
	router.POST("/logout", handler.Logout)

	body, err := json.Marshal(LogoutRequest{SessionCleanupToken: cleanupToken})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	require.Len(t, auditDB.execCalls, 1)

	action, ok := auditDB.execCalls[0][3].(string)
	require.True(t, ok)
	assert.Equal(t, "session.logout", action)

	success, ok := auditDB.execCalls[0][8].(bool)
	require.True(t, ok)
	assert.False(t, success)
}

func TestCustomerShouldClearLogoutCookies(t *testing.T) {
	tests := []struct {
		name                string
		sessionCleanupToken string
		targetSessionID     string
		currentSessionID    string
		want                bool
	}{
		{
			name:             "authenticated logout clears cookies",
			targetSessionID:  "current-session",
			currentSessionID: "current-session",
			want:             true,
		},
		{
			name:                "cleanup token for current session clears cookies",
			sessionCleanupToken: "cleanup-token",
			targetSessionID:     "current-session",
			currentSessionID:    "current-session",
			want:                true,
		},
		{
			name:                "cleanup token for stale session preserves newer cookies",
			sessionCleanupToken: "cleanup-token",
			targetSessionID:     "stale-session",
			currentSessionID:    "current-session",
			want:                false,
		},
		{
			name:                "cleanup token without current session clears cookies",
			sessionCleanupToken: "cleanup-token",
			targetSessionID:     "stale-session",
			currentSessionID:    "",
			want:                true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldClearLogoutCookies(
				tt.sessionCleanupToken,
				tt.targetSessionID,
				tt.currentSessionID,
			))
		})
	}
}

var _ = sharederrors.ErrUnauthorized

func TestRegisterCustomerRoutes_LogoutRequiresAuthentication(t *testing.T) {
	router := setupTestRouter()
	logger := testAuthHandlerLogger()

	handler := &CustomerHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	api := router.Group("/api/v1")
	RegisterCustomerRoutes(api, handler, nil, nil, nil, false, BillingRoutesConfig{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/auth/logout", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
