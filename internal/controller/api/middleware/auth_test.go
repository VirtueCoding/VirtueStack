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

func TestCustomerAPIKeyAuth(t *testing.T) {
	tests := []struct {
		name             string
		validator        CustomerAPIKeyValidator
		setupRequest     func(req *http.Request)
		wantStatus       int
		wantBodyContains string
		wantUserID       string
		wantPermissions  []string
	}{
		{
			name: "valid key sets context",
			validator: func(ctx context.Context, keyHash string) (CustomerAPIKeyInfo, error) {
				return CustomerAPIKeyInfo{
					KeyID:       "key-1",
					CustomerID:  "customer-123",
					Permissions: []string{"vm:read", "vm:write"},
				}, nil
			},
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-API-Key", "my-customer-key")
			},
			wantStatus:      http.StatusOK,
			wantUserID:      "customer-123",
			wantPermissions: []string{"vm:read", "vm:write"},
		},
		{
			name: "invalid key returns error",
			validator: func(ctx context.Context, keyHash string) (CustomerAPIKeyInfo, error) {
				return CustomerAPIKeyInfo{}, fmt.Errorf("key not found")
			},
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-API-Key", "bad-key")
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "INVALID_API_KEY",
		},
		{
			name: "missing key returns error",
			validator: func(ctx context.Context, keyHash string) (CustomerAPIKeyInfo, error) {
				return CustomerAPIKeyInfo{}, nil
			},
			setupRequest: func(req *http.Request) {
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "MISSING_API_KEY",
		},
		{
			name: "expired key returns error",
			validator: func(ctx context.Context, keyHash string) (CustomerAPIKeyInfo, error) {
				return CustomerAPIKeyInfo{}, fmt.Errorf("API key has expired")
			},
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-API-Key", "expired-key")
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "INVALID_API_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(CustomerAPIKeyAuth(tt.validator))
			r.GET("/protected", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{
					"user_id":     GetUserID(c),
					"user_type":   GetUserType(c),
					"api_key_id":  c.GetString("api_key_id"),
					"permissions": GetPermissions(c),
				})
			})

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
			if tt.wantStatus == http.StatusOK {
				var body map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &body)
				require.NoError(t, err)
				assert.Equal(t, tt.wantUserID, body["user_id"])
				assert.Equal(t, "customer", body["user_type"])
			}
		})
	}
}

func TestJWTOrCustomerAPIKeyAuth(t *testing.T) {
	config := testAuthConfig()
	validKeyValidator := func(ctx context.Context, keyHash string) (CustomerAPIKeyInfo, error) {
		return CustomerAPIKeyInfo{
			KeyID:       "key-1",
			CustomerID:  "customer-api",
			AllowedIPs:  []string{"198.51.100.0/24"},
			Permissions: []string{"vm:read"},
		}, nil
	}
	invalidKeyValidator := func(ctx context.Context, keyHash string) (CustomerAPIKeyInfo, error) {
		return CustomerAPIKeyInfo{}, fmt.Errorf("not found")
	}

	tests := []struct {
		name             string
		keyValidator     CustomerAPIKeyValidator
		setupRequest     func(t *testing.T, req *http.Request)
		wantStatus       int
		wantBodyContains string
		wantUserID       string
		wantPermissions  any
	}{
		{
			name:         "JWT token accepted",
			keyValidator: invalidKeyValidator,
			setupRequest: func(t *testing.T, req *http.Request) {
				token := testAccessToken(t, config, "user-jwt", "customer", "", 15*time.Minute)
				req.Header.Set("Authorization", "Bearer "+token)
			},
			wantStatus:      http.StatusOK,
			wantUserID:      "user-jwt",
			wantPermissions: nil, // JWT has full permissions (nil)
		},
		{
			name:         "API key accepted when no JWT",
			keyValidator: validKeyValidator,
			setupRequest: func(t *testing.T, req *http.Request) {
				req.Header.Set("X-API-Key", "my-api-key")
				req.RemoteAddr = "198.51.100.10:1234"
			},
			wantStatus:      http.StatusOK,
			wantUserID:      "customer-api",
			wantPermissions: []any{"vm:read"},
		},
		{
			name:         "API key rejected when source IP is not allowed",
			keyValidator: validKeyValidator,
			setupRequest: func(t *testing.T, req *http.Request) {
				req.Header.Set("X-API-Key", "my-api-key")
				req.RemoteAddr = "203.0.113.10:1234"
			},
			wantStatus:       http.StatusForbidden,
			wantBodyContains: "IP_NOT_ALLOWED",
		},
		{
			name:         "JWT preferred over API key",
			keyValidator: validKeyValidator,
			setupRequest: func(t *testing.T, req *http.Request) {
				token := testAccessToken(t, config, "user-jwt-pref", "customer", "", 15*time.Minute)
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("X-API-Key", "my-api-key")
			},
			wantStatus:      http.StatusOK,
			wantUserID:      "user-jwt-pref", // JWT user, not API key customer
			wantPermissions: nil,
		},
		{
			name:         "no auth returns error",
			keyValidator: validKeyValidator,
			setupRequest: func(t *testing.T, req *http.Request) {
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "MISSING_AUTH",
		},
		{
			// F-006: When a JWT token is present but invalid, the middleware must
			// abort immediately with 401 and must NOT fall through to API key auth.
			// Falling through would allow an attacker who holds an expired/tampered
			// token to bypass JWT validation by also supplying a valid API key.
			name:         "invalid JWT aborts immediately, does not fall through to API key",
			keyValidator: validKeyValidator,
			setupRequest: func(t *testing.T, req *http.Request) {
				req.Header.Set("Authorization", "Bearer invalid.token")
				req.Header.Set("X-API-Key", "my-api-key")
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "INVALID_TOKEN",
		},
		{
			// F-006: Invalid JWT aborts regardless of whether the API key is valid.
			name:         "invalid JWT aborts with INVALID_TOKEN even when API key is also invalid",
			keyValidator: invalidKeyValidator,
			setupRequest: func(t *testing.T, req *http.Request) {
				req.Header.Set("Authorization", "Bearer invalid.token")
				req.Header.Set("X-API-Key", "bad-key")
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "INVALID_TOKEN",
		},
		{
			// F-006: A temp token (purpose=2fa) is an invalid access token and must
			// abort the request immediately rather than falling through to API key auth.
			name:         "temp token rejected with INVALID_TOKEN, does not fall through to API key",
			keyValidator: validKeyValidator,
			setupRequest: func(t *testing.T, req *http.Request) {
				token, err := GenerateTempToken(config, "temp-user", "customer")
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("X-API-Key", "my-api-key")
			},
			wantStatus:       http.StatusUnauthorized,
			wantBodyContains: "INVALID_TOKEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(JWTOrCustomerAPIKeyAuth(config, tt.keyValidator))
			r.GET("/protected", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{
					"user_id":     GetUserID(c),
					"user_type":   GetUserType(c),
					"permissions": GetPermissions(c),
				})
			})

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
				assert.Equal(t, tt.wantUserID, body["user_id"])
				assert.Equal(t, "customer", body["user_type"])
			}
		})
	}
}

func TestRequireVMScopeSupportsVMIDParam(t *testing.T) {
	t.Run("allows scoped vmId path parameter", func(t *testing.T) {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(vmIDsContextKey, []string{"vm-1"})
			c.Next()
		})
		r.GET("/ws/:vmId", RequireVMScope(), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/ws/vm-1", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("rejects vmId outside API key scope", func(t *testing.T) {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(vmIDsContextKey, []string{"vm-1"})
			c.Next()
		})
		r.GET("/ws/:vmId", RequireVMScope(), func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/ws/vm-2", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "VM_NOT_IN_SCOPE")
	})
}

func TestPermissions(t *testing.T) {
	t.Run("GetPermissions returns nil for JWT auth", func(t *testing.T) {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(userIDContextKey, "user-1")
			c.Set(userTypeContextKey, "customer")
			c.Next()
		})
		r.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"permissions": GetPermissions(c)})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		var body map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &body)
		require.NoError(t, err)
		assert.Nil(t, body["permissions"])
	})

	t.Run("GetPermissions returns permissions for API key auth", func(t *testing.T) {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(userIDContextKey, "customer-1")
			c.Set(permissionsContextKey, []string{"vm:read", "vm:write"})
			c.Next()
		})
		r.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"permissions": GetPermissions(c)})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		var body map[string]any
		err := json.Unmarshal(w.Body.Bytes(), &body)
		require.NoError(t, err)
		assert.Equal(t, []any{"vm:read", "vm:write"}, body["permissions"])
	})
}

func TestHasPermission(t *testing.T) {
	tests := []struct {
		name         string
		setupContext func(c *gin.Context)
		permission   string
		wantResult   bool
	}{
		{
			name: "JWT auth has all permissions",
			setupContext: func(c *gin.Context) {
				c.Set(userIDContextKey, "user-1")
			},
			permission: "vm:read",
			wantResult: true,
		},
		{
			name: "API key has matching permission",
			setupContext: func(c *gin.Context) {
				c.Set(permissionsContextKey, []string{"vm:read", "vm:write"})
			},
			permission: "vm:read",
			wantResult: true,
		},
		{
			name: "API key lacks permission",
			setupContext: func(c *gin.Context) {
				c.Set(permissionsContextKey, []string{"vm:read"})
			},
			permission: "vm:write",
			wantResult: false,
		},
		{
			name: "empty permissions denies all",
			setupContext: func(c *gin.Context) {
				c.Set(permissionsContextKey, []string{})
			},
			permission: "vm:read",
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				tt.setupContext(c)
				c.Next()
			})
			r.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"has_permission": HasPermission(c, tt.permission)})
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			var body map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &body)
			require.NoError(t, err)
			assert.Equal(t, tt.wantResult, body["has_permission"])
		})
	}
}

func TestRequirePermission(t *testing.T) {
	tests := []struct {
		name             string
		setupContext     func(c *gin.Context)
		permission       string
		wantStatus       int
		wantBodyContains string
	}{
		{
			name: "JWT auth passes permission check",
			setupContext: func(c *gin.Context) {
				c.Set(userIDContextKey, "user-1")
				c.Set(userTypeContextKey, "customer")
			},
			permission: "vm:write",
			wantStatus: http.StatusOK,
		},
		{
			name: "API key with permission passes",
			setupContext: func(c *gin.Context) {
				c.Set(userIDContextKey, "customer-1")
				c.Set(userTypeContextKey, "customer")
				c.Set(permissionsContextKey, []string{"vm:read", "vm:write"})
			},
			permission: "vm:write",
			wantStatus: http.StatusOK,
		},
		{
			name: "API key without permission fails",
			setupContext: func(c *gin.Context) {
				c.Set(userIDContextKey, "customer-1")
				c.Set(userTypeContextKey, "customer")
				c.Set(actorTypeContextKey, "customer_api_key")
				c.Set(permissionsContextKey, []string{"vm:read"})
			},
			permission:       "vm:write",
			wantStatus:       http.StatusForbidden,
			wantBodyContains: "INSUFFICIENT_PERMISSIONS",
		},
		{
			name: "API key with nil permissions fails",
			setupContext: func(c *gin.Context) {
				c.Set(userIDContextKey, "customer-1")
				c.Set(userTypeContextKey, "customer")
				c.Set(actorTypeContextKey, "customer_api_key")
			},
			permission:       "vm:write",
			wantStatus:       http.StatusForbidden,
			wantBodyContains: "INSUFFICIENT_PERMISSIONS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				tt.setupContext(c)
				c.Next()
			})
			r.Use(RequirePermission(tt.permission))
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
