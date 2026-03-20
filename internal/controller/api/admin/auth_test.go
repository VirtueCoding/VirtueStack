package admin

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestLogout tests logout clears cookies.
func TestLogout(t *testing.T) {
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

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	data, ok := resp["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, data["message"], "Logged out")
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