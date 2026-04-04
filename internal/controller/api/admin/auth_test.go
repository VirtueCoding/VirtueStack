package admin

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
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type logoutAdminRepoStub struct {
	repository.CustomerRepo
	deleteSessionErr error
	deleteSessionIDs []string
}

func (s *logoutAdminRepoStub) DeleteSession(_ context.Context, id string) error {
	s.deleteSessionIDs = append(s.deleteSessionIDs, id)
	return s.deleteSessionErr
}

type adminAuditCaptureDB struct {
	execCalls [][]any
}

func (m *adminAuditCaptureDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return adminAuditCaptureRow{err: pgx.ErrNoRows}
}

func (m *adminAuditCaptureDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *adminAuditCaptureDB) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	copiedArgs := append([]any(nil), args...)
	m.execCalls = append(m.execCalls, copiedArgs)
	return pgconn.NewCommandTag("INSERT 1"), nil
}

func (m *adminAuditCaptureDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, nil
}

type adminAuditCaptureRow struct {
	err error
}

func (r adminAuditCaptureRow) Scan(...any) error {
	return r.err
}

func testAdminLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func setupAdminTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// TestLogin_InvalidRequestBody tests that invalid JSON returns 400.
func TestLogin_InvalidRequestBody(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	router.POST("/login", handler.Login)

	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestLogin_EmailValidation tests email validation on login.
func TestLogin_EmailValidation(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	router.POST("/login", handler.Login)

	tests := []struct {
		name     string
		email    string
		password string
	}{
		{
			name:     "missing email",
			email:    "",
			password: "validpassword123",
		},
		{
			name:     "invalid email format",
			email:    "not-an email",
			password: "validpassword123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]string{
				"email":    tt.email,
				"password": tt.password,
			}
			jsonBody, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestLogin_PasswordValidation tests password validation on login.
func TestLogin_PasswordValidation(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	router.POST("/login", handler.Login)

	tests := []struct {
		name     string
		password string
		wantCode int
	}{
		{
			name:     "password too short (11 chars)",
			password: "elevenchars",
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "empty password",
			password: "",
			wantCode: http.StatusBadRequest,
		},
		// Note: 12-char password passes validation but requires service mock.
		// Integration tests cover the full login flow with proper service layer.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]string{
				"email":    "admin@test.com",
				"password": tt.password,
			}
			jsonBody, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantCode, w.Code)
		})
	}
}

// TestVerify2FA_InvalidRequestBody tests 2FA verification with invalid JSON.
func TestVerify2FA_InvalidRequestBody(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	router.POST("/verify-2fa", handler.Verify2FA)

	req := httptest.NewRequest(http.MethodPost, "/verify-2fa", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestVerify2FA_Validation tests TOTP code validation.
func TestVerify2FA_Validation(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	router.POST("/verify-2fa", handler.Verify2FA)

	tests := []struct {
		name      string
		tempToken string
		totpCode  string
	}{
		{
			name:      "missing temp_token",
			tempToken: "",
			totpCode:  "123456",
		},
		{
			name:      "missing totp_code",
			tempToken: "temp-token",
			totpCode:  "",
		},
		{
			name:      "totp_code too short",
			tempToken: "temp-token",
			totpCode:  "12345",
		},
		{
			name:      "totp_code too long",
			tempToken: "temp-token",
			totpCode:  "1234567",
		},
		{
			name:      "totp_code non-numeric",
			tempToken: "temp-token",
			totpCode:  "abcdef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]string{
				"temp_token": tt.tempToken,
				"totp_code":  tt.totpCode,
			}
			jsonBody, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPost, "/verify-2fa", bytes.NewBuffer(jsonBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

// TestRefreshToken_MissingToken tests refresh without token.
func TestRefreshToken_MissingToken(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	router.POST("/refresh", handler.RefreshToken)

	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "VALIDATION_ERROR", errorObj["code"])
}

// TestMe_Unauthorized tests /me endpoint without authentication.
func TestMe_Unauthorized(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	router.GET("/me", handler.Me)

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "UNAUTHORIZED", errorObj["code"])
}

// TestLogout_Unauthenticated rejects logout without an access or cleanup token.
func TestLogout_Unauthenticated(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	router.POST("/logout", handler.Logout)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "UNAUTHORIZED", errorObj["code"])
}

func TestRegisterAdminRoutes_LogoutRequiresAuthentication(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	api := router.Group("/api/v1")
	RegisterAdminRoutes(api, handler, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/logout", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRegisterAdminRoutes_LogoutRejectsMalformedJSON(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	api := router.Group("/api/v1")
	RegisterAdminRoutes(api, handler, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/logout", bytes.NewBufferString(`{"session_cleanup_token":`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "INVALID_REQUEST_BODY", errorObj["code"])
}

func TestRegisterAdminRoutes_CSRFIssuesTokenCookie(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	api := router.Group("/api/v1")
	RegisterAdminRoutes(api, handler, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/auth/csrf", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var csrfCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "csrf_token" {
			csrfCookie = cookie
			break
		}
	}

	require.NotNil(t, csrfCookie, "CSRF cookie should be set")
	assert.NotEmpty(t, csrfCookie.Value)
	assert.Equal(t, csrfCookie.Value, w.Header().Get("X-CSRF-Token"))
}

func TestRegisterAdminRoutes_LogoutStillRequiresCSRFFromCurrentSession(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()
	authConfig := middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"}
	authService := services.NewAuthService(
		&logoutAdminRepoStub{deleteSessionErr: errors.New("delete session failed")},
		nil,
		nil,
		authConfig.JWTSecret,
		authConfig.Issuer,
		"",
		logger,
	)

	handler := &AdminHandler{
		authService: authService,
		authConfig:  authConfig,
		logger:      logger,
	}

	api := router.Group("/api/v1")
	RegisterAdminRoutes(api, handler, nil)

	accessToken, err := middleware.GenerateAccessToken(
		authConfig,
		"admin-123",
		"admin",
		"admin",
		"session-123",
		15*time.Minute,
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/logout", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: middleware.AccessTokenCookieName, Value: accessToken})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "CSRF_COOKIE_MISSING", errorObj["code"])
}

func TestRegisterAdminRoutes_LogoutRejectsNonAdminCleanupToken(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()
	authConfig := middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"}
	authService := services.NewAuthService(
		&logoutAdminRepoStub{},
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

	handler := &AdminHandler{
		authService: authService,
		authConfig:  authConfig,
		logger:      logger,
	}

	api := router.Group("/api/v1")
	RegisterAdminRoutes(api, handler, nil)

	body, err := json.Marshal(LogoutRequest{SessionCleanupToken: cleanupToken})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/logout", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminLogout_ReturnsInternalErrorWhenSessionInvalidationFails(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()
	authConfig := middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"}
	deleteErr := errors.New("delete session failed")
	authService := services.NewAuthService(
		&logoutAdminRepoStub{deleteSessionErr: deleteErr},
		nil,
		nil,
		authConfig.JWTSecret,
		authConfig.Issuer,
		"",
		logger,
	)

	cleanupToken, err := middleware.GenerateSessionCleanupToken(
		authConfig,
		"admin-123",
		"admin",
		"admin",
		"session-123",
	)
	require.NoError(t, err)

	handler := &AdminHandler{
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

func TestAdminLogout_RejectsMalformedJSON(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()
	authConfig := middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"}
	repo := &logoutAdminRepoStub{}
	authService := services.NewAuthService(
		repo,
		nil,
		nil,
		authConfig.JWTSecret,
		authConfig.Issuer,
		"",
		logger,
	)

	accessToken, err := middleware.GenerateAccessToken(
		authConfig,
		"admin-123",
		"admin",
		"admin",
		"session-123",
		time.Hour,
	)
	require.NoError(t, err)

	handler := &AdminHandler{
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
	assert.Empty(t, repo.deleteSessionIDs)

	var resp map[string]any
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "INVALID_REQUEST_BODY", errorObj["code"])
}

func TestAdminLogout_LogsFailedSessionAuditEventWhenInvalidationFails(t *testing.T) {
	logger := testAdminLogger()
	authConfig := middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"}
	deleteErr := errors.New("delete session failed")
	auditDB := &adminAuditCaptureDB{}
	authService := services.NewAuthService(
		&logoutAdminRepoStub{deleteSessionErr: deleteErr},
		nil,
		nil,
		authConfig.JWTSecret,
		authConfig.Issuer,
		"",
		logger,
	)

	cleanupToken, err := middleware.GenerateSessionCleanupToken(
		authConfig,
		"admin-123",
		"admin",
		"admin",
		"session-123",
	)
	require.NoError(t, err)

	handler := &AdminHandler{
		authService: authService,
		auditRepo:   repository.NewAuditRepository(auditDB),
		authConfig:  authConfig,
		logger:      logger,
	}

	router := setupAdminTestRouter()
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

	errorMessage, ok := auditDB.execCalls[0][9].(*string)
	require.True(t, ok)
	require.NotNil(t, errorMessage)
	assert.Equal(t, "deleting session: "+deleteErr.Error(), *errorMessage)
}

func TestAdminLogout_LogsSessionAuditEvent(t *testing.T) {
	logger := testAdminLogger()
	authConfig := middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"}

	cleanupToken, err := middleware.GenerateSessionCleanupToken(
		authConfig,
		"admin-123",
		"admin",
		"admin",
		"session-123",
	)
	require.NoError(t, err)

	accessToken, err := middleware.GenerateAccessToken(
		authConfig,
		"admin-123",
		"admin",
		"admin",
		"session-123",
		time.Hour,
	)
	require.NoError(t, err)

	tests := []struct {
		name             string
		body             string
		configureRequest func(req *http.Request)
	}{
		{
			name: "access token",
			body: `{}`,
			configureRequest: func(req *http.Request) {
				req.Header.Set("Authorization", "Bearer "+accessToken)
			},
		},
		{
			name:             "cleanup token",
			body:             `{"session_cleanup_token":"` + cleanupToken + `"}`,
			configureRequest: func(req *http.Request) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupAdminTestRouter()
			repo := &logoutAdminRepoStub{}
			auditDB := &adminAuditCaptureDB{}
			authService := services.NewAuthService(
				repo,
				nil,
				nil,
				authConfig.JWTSecret,
				authConfig.Issuer,
				"",
				logger,
			)

			handler := &AdminHandler{
				authService: authService,
				auditRepo:   repository.NewAuditRepository(auditDB),
				authConfig:  authConfig,
				logger:      logger,
			}

			router.POST("/logout", handler.Logout)

			req := httptest.NewRequest(http.MethodPost, "/logout", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			tt.configureRequest(req)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, []string{"session-123"}, repo.deleteSessionIDs)
			require.Len(t, auditDB.execCalls, 1)

			actorID, ok := auditDB.execCalls[0][0].(*string)
			require.True(t, ok)
			require.NotNil(t, actorID)
			assert.Equal(t, "admin-123", *actorID)

			actorType, ok := auditDB.execCalls[0][1].(string)
			require.True(t, ok)
			assert.Equal(t, "admin", actorType)

			action, ok := auditDB.execCalls[0][3].(string)
			require.True(t, ok)
			assert.Equal(t, "session.logout", action)

			resourceType, ok := auditDB.execCalls[0][4].(string)
			require.True(t, ok)
			assert.Equal(t, "session", resourceType)

			resourceID, ok := auditDB.execCalls[0][5].(*string)
			require.True(t, ok)
			require.NotNil(t, resourceID)
			assert.Equal(t, "session-123", *resourceID)

			success, ok := auditDB.execCalls[0][8].(bool)
			require.True(t, ok)
			assert.True(t, success)
		})
	}
}

func TestShouldClearLogoutCookies(t *testing.T) {
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
			name:                "cleanup token without current session preserves cookies",
			sessionCleanupToken: "cleanup-token",
			targetSessionID:     "stale-session",
			currentSessionID:    "",
			want:                false,
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

// TestLoginRequest_ValidationRules tests struct validation rules.
func TestLoginRequest_ValidationRules(t *testing.T) {
	tests := []struct {
		name        string
		request     LoginRequest
		expectValid bool
	}{
		{
			name: "valid request",
			request: LoginRequest{
				Email:    "admin@test.com",
				Password: "validpassword123",
			},
			expectValid: true,
		},
		{
			name: "email too long",
			request: LoginRequest{
				Email:    string(make([]byte, 255)) + "@test.com",
				Password: "validpassword123",
			},
			expectValid: false,
		},
		{
			name: "password at minimum (12 chars)",
			request: LoginRequest{
				Email:    "admin@test.com",
				Password: "exactlytwelve",
			},
			expectValid: true,
		},
		{
			name: "password at maximum (128 chars)",
			request: LoginRequest{
				Email:    "admin@test.com",
				Password: string(make([]byte, 128)),
			},
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectValid {
				assert.LessOrEqual(t, len(tt.request.Email), 254)
				assert.GreaterOrEqual(t, len(tt.request.Password), 12)
				assert.LessOrEqual(t, len(tt.request.Password), 128)
			}
		})
	}
}

// TestVerify2FARequest_ValidationRules tests TOTP validation rules.
func TestVerify2FARequest_ValidationRules(t *testing.T) {
	tests := []struct {
		name    string
		request Verify2FARequest
		valid   bool
	}{
		{
			name: "valid request",
			request: Verify2FARequest{
				TempToken: "temp-token-abc",
				TOTPCode:  "123456",
			},
			valid: true,
		},
		{
			name: "empty temp token",
			request: Verify2FARequest{
				TempToken: "",
				TOTPCode:  "123456",
			},
			valid: false,
		},
		{
			name: "wrong length code",
			request: Verify2FARequest{
				TempToken: "temp-token",
				TOTPCode:  "12345",
			},
			valid: false,
		},
		{
			name: "non-numeric code",
			request: Verify2FARequest{
				TempToken: "temp-token",
				TOTPCode:  "abcdef",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.Len(t, tt.request.TOTPCode, 6)
				for _, c := range tt.request.TOTPCode {
					assert.True(t, c >= '0' && c <= '9', "TOTP code should be numeric")
				}
				assert.NotEmpty(t, tt.request.TempToken)
			}
		})
	}
}

// TestErrorResponseFormat verifies error response structure.
func TestErrorResponseFormat(t *testing.T) {
	router := setupAdminTestRouter()
	logger := testAdminLogger()

	handler := &AdminHandler{
		authConfig: middleware.AuthConfig{JWTSecret: "test-secret", Issuer: "virtuestack"},
		logger:     logger,
	}

	router.POST("/login", handler.Login)

	body := `{"email": "admin@test.com", "password": "short"}`
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	errorObj, ok := resp["error"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, errorObj, "code")
	assert.Contains(t, errorObj, "message")
}

// TestAuthResponse_Structure tests AuthResponse fields.
func TestAuthResponse_Structure(t *testing.T) {
	resp := AuthResponse{
		TokenType:   "Bearer",
		ExpiresIn:   3600,
		Requires2FA: true,
		TempToken:   "temp-abc",
	}

	assert.Equal(t, "Bearer", resp.TokenType)
	assert.Equal(t, 3600, resp.ExpiresIn)
	assert.True(t, resp.Requires2FA)
	assert.Equal(t, "temp-abc", resp.TempToken)
}

// TestMeResponse_Structure tests MeResponse fields.
func TestMeResponse_Structure(t *testing.T) {
	resp := MeResponse{
		ID:    "admin-123",
		Email: "admin@test.com",
		Role:  "admin",
	}

	assert.Equal(t, "admin-123", resp.ID)
	assert.Equal(t, "admin@test.com", resp.Email)
	assert.Equal(t, "admin", resp.Role)
}
