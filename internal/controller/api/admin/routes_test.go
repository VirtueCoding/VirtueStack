package admin

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminLogoutRequiresAuthenticatedAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewAdminHandler(AdminHandlerConfig{
		JWTSecret: "test-secret-key-that-is-32-bytes-long!!",
		Issuer:    "virtuestack",
		Logger:    slog.Default(),
	})
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterAdminRoutes(api, handler, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/logout", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminLogoutRejectsCustomerJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const jwtSecret = "test-secret-key-that-is-32-bytes-long!!"
	handler := NewAdminHandler(AdminHandlerConfig{
		JWTSecret: jwtSecret,
		Issuer:    "virtuestack",
		Logger:    slog.Default(),
	})
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterAdminRoutes(api, handler, nil)

	token, err := middleware.GenerateAccessToken(handler.AuthConfig(), "customer-1", "customer", "", time.Minute)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: middleware.AccessTokenCookieName, Value: token})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestAdminAuthMutationsRequireCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const jwtSecret = "test-secret-key-that-is-32-bytes-long!!"
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "logout without csrf", path: "/api/v1/admin/auth/logout", body: ""},
		{name: "reauth without csrf", path: "/api/v1/admin/auth/reauth", body: `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewAdminHandler(AdminHandlerConfig{JWTSecret: jwtSecret, Issuer: "virtuestack", Logger: slog.Default()})
			router := gin.New()
			api := router.Group("/api/v1")
			RegisterAdminRoutes(api, handler, nil)

			token, err := middleware.GenerateAccessToken(handler.AuthConfig(), "admin-1", "admin", "admin", time.Minute)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: middleware.AccessTokenCookieName, Value: token})
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusForbidden, w.Code)
		})
	}
}
