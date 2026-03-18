package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func testAuthConfig() AuthConfig {
	return AuthConfig{
		JWTSecret: "test-secret-key-that-is-32-bytes-long!!",
		Issuer:    "virtuestack",
	}
}

func testAccessToken(t *testing.T, config AuthConfig, userID, userType, role string, expiresIn time.Duration) string {
	t.Helper()
	token, err := GenerateAccessToken(config, userID, userType, role, expiresIn)
	require.NoError(t, err)
	return token
}

func setupRouter(mws ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(mws...)
	r.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": GetUserID(c), "role": GetRole(c)})
	})
	return r
}

func TestJWTAuth(t *testing.T) {
	config := testAuthConfig()

	tests := []struct {
		name             string
		setupRequest     func(t *testing.T, req *http.Request)
		wantStatus       int
		wantBodyContains string
	}{
		{
			name: "valid token from cookie",
			setupRequest: func(t *testing.T, req *http.Request) {
				t.Helper()
				token := testAccessToken(t, config, "user-1", "admin", "admin", 15*time.Minute)
				req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: token})
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "valid token from authorization header",
			setupRequest: func(t *testing.T, req *http.Request) {
				t.Helper()
				token := testAccessToken(t, config, "user-2", "customer", "", 15*time.Minute)
				req.Header.Set("Authorization", "Bearer "+token)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing token",
			setupRequest: func(t *testing.T, req *http.Request) {
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "MISSING_TOKEN",
		},
		{
			name: "expired token",
			setupRequest: func(t *testing.T, req *http.Request) {
				t.Helper()
				token := testAccessToken(t, config, "user-3", "admin", "admin", -time.Second)
				req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: token})
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "INVALID_TOKEN",
		},
		{
			name: "invalid signature",
			setupRequest: func(t *testing.T, req *http.Request) {
				t.Helper()
				claims := &JWTClaims{
					UserID:   "user-4",
					UserType: "admin",
					Role:     "admin",
					RegisteredClaims: jwt.RegisteredClaims{
						Subject:   "user-4",
						Issuer:    config.Issuer,
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
					},
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				signed, err := token.SignedString([]byte("wrong-secret-key-!!"))
				require.NoError(t, err)
				req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: signed})
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "INVALID_TOKEN",
		},
		{
			name: "temp token rejected",
			setupRequest: func(t *testing.T, req *http.Request) {
				t.Helper()
				token, err := GenerateTempToken(config, "user-5", "admin")
				require.NoError(t, err)
				req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: token})
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "INVALID_TOKEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupRouter(JWTAuth(config))
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			tt.setupRequest(t, req)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantBodyContains != "" {
				var body ErrorResponse
				err := json.Unmarshal(w.Body.Bytes(), &body)
				require.NoError(t, err)
				assert.Contains(t, body.Error.Code, tt.wantBodyContains)
			}
			if tt.wantStatus == http.StatusOK {
				var body map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &body)
				require.NoError(t, err)
				assert.Contains(t, body, "user_id")
			}
		})
	}
}

func TestAPIKeyAuth(t *testing.T) {
	tests := []struct {
		name             string
		validator        APIKeyValidator
		setupRequest     func(req *http.Request)
		wantStatus       int
		wantBodyContains string
	}{
		{
			name: "valid key",
			validator: func(ctx context.Context, keyHash string) (string, []string, error) {
				return "key-1", nil, nil
			},
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-API-Key", "my-provisioning-key")
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "invalid key",
			validator: func(ctx context.Context, keyHash string) (string, []string, error) {
				return "", nil, fmt.Errorf("not found")
			},
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-API-Key", "bad-key")
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "INVALID_API_KEY",
		},
		{
			name: "missing key",
			validator: func(ctx context.Context, keyHash string) (string, []string, error) {
				return "", nil, nil
			},
			setupRequest: func(req *http.Request) {
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "MISSING_API_KEY",
		},
		{
			name: "key with IP restriction - allowed",
			validator: func(ctx context.Context, keyHash string) (string, []string, error) {
				return "key-2", []string{"192.168.1.1"}, nil
			},
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-API-Key", "restricted-key")
				req.RemoteAddr = "192.168.1.1:12345"
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "key with IP restriction - denied",
			validator: func(ctx context.Context, keyHash string) (string, []string, error) {
				return "key-3", []string{"10.0.0.1"}, nil
			},
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-API-Key", "restricted-key")
				req.RemoteAddr = "192.168.1.1:12345"
			},
			wantStatus:       http.StatusForbidden,
			wantBodyContains: "IP_NOT_ALLOWED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupRouter(APIKeyAuth(tt.validator))
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			tt.setupRequest(req)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantBodyContains != "" {
				var body ErrorResponse
				err := json.Unmarshal(w.Body.Bytes(), &body)
				require.NoError(t, err)
				assert.Contains(t, body.Error.Code, tt.wantBodyContains)
			}
		})
	}
}

func TestRequireRole(t *testing.T) {
	tests := []struct {
		name             string
		role             string
		allowedRoles     []string
		wantStatus       int
		wantBodyContains string
	}{
		{
			name:         "matching role",
			role:         "admin",
			allowedRoles: []string{"admin", "super_admin"},
			wantStatus:   http.StatusOK,
		},
		{
			name:         "super_admin allowed",
			role:         "super_admin",
			allowedRoles: []string{"admin", "super_admin"},
			wantStatus:   http.StatusOK,
		},
		{
			name:             "role not allowed",
			role:             "customer",
			allowedRoles:     []string{"admin", "super_admin"},
			wantStatus:       http.StatusForbidden,
			wantBodyContains: "INSUFFICIENT_ROLE",
		},
		{
			name:             "empty role rejected",
			role:             "",
			allowedRoles:     []string{"admin"},
			wantStatus:       http.StatusForbidden,
			wantBodyContains: "INSUFFICIENT_ROLE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()

			r.Use(func(c *gin.Context) {
				c.Set("role", tt.role)
				c.Next()
			})
			r.Use(RequireRole(tt.allowedRoles...))
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantBodyContains != "" {
				var body ErrorResponse
				err := json.Unmarshal(w.Body.Bytes(), &body)
				require.NoError(t, err)
				assert.Contains(t, body.Error.Code, tt.wantBodyContains)
			}
		})
	}
}

func TestRequireUserType(t *testing.T) {
	tests := []struct {
		name             string
		userType         string
		allowedTypes     []string
		wantStatus       int
		wantBodyContains string
	}{
		{
			name:         "customer allowed",
			userType:     "customer",
			allowedTypes: []string{"customer"},
			wantStatus:   http.StatusOK,
		},
		{
			name:             "admin denied for customer endpoint",
			userType:         "admin",
			allowedTypes:     []string{"customer"},
			wantStatus:       http.StatusForbidden,
			wantBodyContains: "INSUFFICIENT_PERMISSIONS",
		},
		{
			name:         "admin allowed for admin endpoint",
			userType:     "admin",
			allowedTypes: []string{"admin"},
			wantStatus:   http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set("user_type", tt.userType)
				c.Next()
			})
			r.Use(RequireUserType(tt.allowedTypes...))
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
			if tt.wantBodyContains != "" {
				var body ErrorResponse
				err := json.Unmarshal(w.Body.Bytes(), &body)
				require.NoError(t, err)
				assert.Contains(t, body.Error.Code, tt.wantBodyContains)
			}
		})
	}
}

func TestOptionalJWTAuth(t *testing.T) {
	config := testAuthConfig()

	t.Run("no token proceeds anonymously", func(t *testing.T) {
		r := setupRouter(OptionalJWTAuth(config))
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &body)
		require.NoError(t, err)
		assert.Equal(t, "", body["user_id"])
	})

	t.Run("valid token sets context", func(t *testing.T) {
		r := setupRouter(OptionalJWTAuth(config))
		token := testAccessToken(t, config, "user-opt", "admin", "admin", 15*time.Minute)
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var body map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &body)
		require.NoError(t, err)
		assert.Equal(t, "user-opt", body["user_id"])
	})

	t.Run("invalid token proceeds anonymously", func(t *testing.T) {
		r := setupRouter(OptionalJWTAuth(config))
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer invalid.token.here")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestGenerateAndValidateAccessToken(t *testing.T) {
	config := testAuthConfig()

	t.Run("roundtrip success", func(t *testing.T) {
		token := testAccessToken(t, config, "user-rt", "admin", "super_admin", 15*time.Minute)
		claims, err := parseAndValidateJWT(config, token)
		require.NoError(t, err)
		assert.Equal(t, "user-rt", claims.UserID)
		assert.Equal(t, "admin", claims.UserType)
		assert.Equal(t, "super_admin", claims.Role)
	})

	t.Run("expired token fails validation", func(t *testing.T) {
		token := testAccessToken(t, config, "user-exp", "admin", "admin", -time.Minute)
		_, err := parseAndValidateJWT(config, token)
		require.Error(t, err)
	})
}

func TestValidateTempToken(t *testing.T) {
	config := testAuthConfig()

	t.Run("valid temp token", func(t *testing.T) {
		token, err := GenerateTempToken(config, "user-tmp", "admin")
		require.NoError(t, err)
		claims, err := ValidateTempToken(config, token)
		require.NoError(t, err)
		assert.Equal(t, "user-tmp", claims.UserID)
		assert.Equal(t, "2fa", claims.Purpose)
	})

	t.Run("regular access token not accepted as temp", func(t *testing.T) {
		token := testAccessToken(t, config, "user-reg", "admin", "admin", 15*time.Minute)
		_, err := ValidateTempToken(config, token)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a temp token")
	})
}

func TestGenerateRefreshToken(t *testing.T) {
	t.Run("generates 64-char hex string", func(t *testing.T) {
		token, err := GenerateRefreshToken()
		require.NoError(t, err)
		assert.Len(t, token, 64)
	})

	t.Run("tokens are unique", func(t *testing.T) {
		t1, err := GenerateRefreshToken()
		require.NoError(t, err)
		t2, err := GenerateRefreshToken()
		require.NoError(t, err)
		assert.NotEqual(t, t1, t2)
	})
}
