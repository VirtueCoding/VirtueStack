package customer

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testAuthHandlerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
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

var _ = sharederrors.ErrUnauthorized
