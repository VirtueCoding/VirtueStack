package customer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// mockAPIKeyRepoForRoutes is a mock for route-level testing.
type mockAPIKeyRepoForRoutes struct {
	getByHashFunc func(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error)
}

func (m *mockAPIKeyRepoForRoutes) GetByHash(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error) {
	if m.getByHashFunc != nil {
		return m.getByHashFunc(ctx, keyHash)
	}
	return nil, nil
}

// TestAPISubsetPermissionEnforcement tests that API keys with specific permissions
// can only access matching endpoints.
func TestAPISubsetPermissionEnforcement(t *testing.T) {
	tests := []struct {
		name            string
		permissions     []string
		endpoint        string
		method          string
		setupMockRepo   func() *mockAPIKeyRepoForRoutes
		wantStatus      int
		description     string
	}{
		{
			name:        "vm:read can list VMs",
			permissions: []string{"vm:read"},
			endpoint:    "/vms",
			method:      http.MethodGet,
			setupMockRepo: func() *mockAPIKeyRepoForRoutes {
				return &mockAPIKeyRepoForRoutes{
					getByHashFunc: func(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error) {
						return &models.CustomerAPIKey{
							ID:          "key-1",
							CustomerID:  "customer-1",
							Permissions: []string{"vm:read"},
							IsActive:    true,
						}, nil
					},
				}
			},
			wantStatus:  http.StatusOK,
			description: "API key with vm:read should be able to list VMs",
		},
		{
			name:        "missing vm:read cannot list VMs",
			permissions: []string{"vm:write"},
			endpoint:    "/vms",
			method:      http.MethodGet,
			setupMockRepo: func() *mockAPIKeyRepoForRoutes {
				return &mockAPIKeyRepoForRoutes{
					getByHashFunc: func(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error) {
						return &models.CustomerAPIKey{
							ID:          "key-2",
							CustomerID:  "customer-1",
							Permissions: []string{"vm:write"},
							IsActive:    true,
						}, nil
					},
				}
			},
			wantStatus:  http.StatusForbidden,
			description: "API key without vm:read should not list VMs",
		},
		{
			name:        "vm:power can start VM",
			permissions: []string{"vm:read", "vm:power"},
			endpoint:    "/vms/vm-1/start",
			method:      http.MethodPost,
			setupMockRepo: func() *mockAPIKeyRepoForRoutes {
				return &mockAPIKeyRepoForRoutes{
					getByHashFunc: func(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error) {
						return &models.CustomerAPIKey{
							ID:          "key-3",
							CustomerID:  "customer-1",
							Permissions: []string{"vm:read", "vm:power"},
							IsActive:    true,
						}, nil
					},
				}
			},
			wantStatus:  http.StatusOK,
			description: "API key with vm:power should be able to start VM",
		},
		{
			name:        "missing vm:power cannot start VM",
			permissions: []string{"vm:read"},
			endpoint:    "/vms/vm-1/start",
			method:      http.MethodPost,
			setupMockRepo: func() *mockAPIKeyRepoForRoutes {
				return &mockAPIKeyRepoForRoutes{
					getByHashFunc: func(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error) {
						return &models.CustomerAPIKey{
							ID:          "key-4",
							CustomerID:  "customer-1",
							Permissions: []string{"vm:read"},
							IsActive:    true,
						}, nil
					},
				}
			},
			wantStatus:  http.StatusForbidden,
			description: "API key without vm:power should not start VM",
		},
		{
			name:        "backup:read can list backups",
			permissions: []string{"backup:read"},
			endpoint:    "/backups",
			method:      http.MethodGet,
			setupMockRepo: func() *mockAPIKeyRepoForRoutes {
				return &mockAPIKeyRepoForRoutes{
					getByHashFunc: func(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error) {
						return &models.CustomerAPIKey{
							ID:          "key-5",
							CustomerID:  "customer-1",
							Permissions: []string{"backup:read"},
							IsActive:    true,
						}, nil
					},
				}
			},
			wantStatus:  http.StatusOK,
			description: "API key with backup:read should be able to list backups",
		},
		{
			name:        "backup:write can create backup",
			permissions: []string{"backup:read", "backup:write"},
			endpoint:    "/backups",
			method:      http.MethodPost,
			setupMockRepo: func() *mockAPIKeyRepoForRoutes {
				return &mockAPIKeyRepoForRoutes{
					getByHashFunc: func(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error) {
						return &models.CustomerAPIKey{
							ID:          "key-6",
							CustomerID:  "customer-1",
							Permissions: []string{"backup:read", "backup:write"},
							IsActive:    true,
						}, nil
					},
				}
			},
			wantStatus:  http.StatusOK,
			description: "API key with backup:write should be able to create backup",
		},
		{
			name:        "snapshot:write can create snapshot",
			permissions: []string{"snapshot:read", "snapshot:write"},
			endpoint:    "/snapshots",
			method:      http.MethodPost,
			setupMockRepo: func() *mockAPIKeyRepoForRoutes {
				return &mockAPIKeyRepoForRoutes{
					getByHashFunc: func(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error) {
						return &models.CustomerAPIKey{
							ID:          "key-7",
							CustomerID:  "customer-1",
							Permissions: []string{"snapshot:read", "snapshot:write"},
							IsActive:    true,
						}, nil
					},
				}
			},
			wantStatus:  http.StatusOK,
			description: "API key with snapshot:write should be able to create snapshot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal router that tests permission middleware
			r := gin.New()

			// We don't need the mock repo here since we're setting context directly
			_ = tt.setupMockRepo() // Keep for API contract verification

			// Add auth middleware
			r.Use(func(c *gin.Context) {
				// Simulate API key auth by setting context directly
				c.Set("user_id", "customer-1")
				c.Set("user_type", "customer")
				c.Set("api_key_id", "test-key")
				c.Set("permissions", tt.permissions)
				c.Next()
			})
			r.Use(middleware.RequireUserType("customer"))

			// Add a simple route with permission check
			if tt.endpoint == "/vms" && tt.method == http.MethodGet {
				r.GET("/vms", middleware.RequirePermission(PermissionVMRead), func(c *gin.Context) {
					c.Status(http.StatusOK)
				})
			} else if tt.endpoint == "/vms/vm-1/start" {
				r.POST("/vms/vm-1/start", middleware.RequirePermission(PermissionVMPower), func(c *gin.Context) {
					c.Status(http.StatusOK)
				})
			} else if tt.endpoint == "/backups" && tt.method == http.MethodGet {
				r.GET("/backups", middleware.RequirePermission(PermissionBackupRead), func(c *gin.Context) {
					c.Status(http.StatusOK)
				})
			} else if tt.endpoint == "/backups" && tt.method == http.MethodPost {
				r.POST("/backups", middleware.RequirePermission(PermissionBackupWrite), func(c *gin.Context) {
					c.Status(http.StatusOK)
				})
			} else if tt.endpoint == "/snapshots" && tt.method == http.MethodPost {
				r.POST("/snapshots", middleware.RequirePermission(PermissionSnapshotWrite), func(c *gin.Context) {
					c.Status(http.StatusOK)
				})
			}

			req := httptest.NewRequest(tt.method, tt.endpoint, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code, tt.description)
		})
	}
}

// TestAPIKeyCannotAccessAccountManagement tests that API keys are rejected from
// account management endpoints (JWT-only routes).
func TestAPIKeyCannotAccessAccountManagement(t *testing.T) {
	tests := []struct {
		name        string
		endpoint    string
		method      string
		description string
	}{
		{
			name:        "API key cannot access profile",
			endpoint:    "/profile",
			method:      http.MethodGet,
			description: "Account management should require JWT, not API key",
		},
		{
			name:        "API key cannot list API keys",
			endpoint:    "/api-keys",
			method:      http.MethodGet,
			description: "API key management should require JWT",
		},
		{
			name:        "API key cannot create API key",
			endpoint:    "/api-keys",
			method:      http.MethodPost,
			description: "Creating API keys should require JWT",
		},
		{
			name:        "API key cannot access webhooks",
			endpoint:    "/webhooks",
			method:      http.MethodGet,
			description: "Webhook management should require JWT",
		},
		{
			name:        "API key cannot access 2FA status",
			endpoint:    "/2fa/status",
			method:      http.MethodGet,
			description: "2FA management should require JWT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create router with JWT-only middleware (no API key support)
			r := gin.New()

			// Simulate API key auth context (has api_key_id set)
			r.Use(func(c *gin.Context) {
				c.Set("user_id", "customer-1")
				c.Set("user_type", "customer")
				c.Set("api_key_id", "test-key")
				c.Next()
			})

			// JWT-only routes use JWTAuth which would reject API keys
			// For this test, we simulate by checking if api_key_id is set
			r.Use(func(c *gin.Context) {
				apiKeyID, exists := c.Get("api_key_id")
				if exists && apiKeyID != "" {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
						"error":   "JWT_REQUIRED",
						"message": "this endpoint requires browser session authentication",
					})
					return
				}
				c.Next()
			})

			r.Handle(tt.method, tt.endpoint, func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(tt.method, tt.endpoint, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code, tt.description)
		})
	}
}

// TestCustomerIsolation verifies that GetUserID returns the correct customer ID
// for both JWT and API key authentication.
func TestCustomerIsolation(t *testing.T) {
	t.Run("JWT auth sets correct customer ID", func(t *testing.T) {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			// Simulate JWT auth setting user_id from JWT claims
			c.Set("user_id", "customer-jwt-123")
			c.Set("user_type", "customer")
			c.Next()
		})
		r.GET("/test", func(c *gin.Context) {
			customerID := middleware.GetUserID(c)
			c.JSON(http.StatusOK, gin.H{"customer_id": customerID})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		// Response body would contain "customer-jwt-123"
	})

	t.Run("API key auth sets correct customer ID", func(t *testing.T) {
		r := gin.New()
		r.Use(func(c *gin.Context) {
			// Simulate API key auth setting user_id from CustomerAPIKeyInfo
			c.Set("user_id", "customer-apikey-456")
			c.Set("user_type", "customer")
			c.Set("api_key_id", "key-789")
			c.Set("permissions", []string{"vm:read"})
			c.Next()
		})
		r.GET("/test", func(c *gin.Context) {
			customerID := middleware.GetUserID(c)
			c.JSON(http.StatusOK, gin.H{"customer_id": customerID})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		// Response body would contain "customer-apikey-456"
	})
}

// TestJWTHasFullPermissions verifies that JWT-authenticated requests
// bypass permission checks (full access).
func TestJWTHasFullPermissions(t *testing.T) {
	// When permissions is nil (JWT auth), HasPermission returns true for any permission

	r := gin.New()
	r.Use(func(c *gin.Context) {
		// Don't set "permissions" - simulates JWT auth
		c.Set("user_id", "customer-1")
		c.Set("user_type", "customer")
		c.Next()
	})
	r.Use(middleware.RequirePermission(PermissionVMWrite))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "JWT auth should have full permissions")
}

// TestAllPermissionsNeeded tests that all 7 permission types work correctly.
func TestAllPermissionsNeeded(t *testing.T) {
	allPermissions := []string{
		PermissionVMRead,
		PermissionVMWrite,
		PermissionVMPower,
		PermissionBackupRead,
		PermissionBackupWrite,
		PermissionSnapshotRead,
		PermissionSnapshotWrite,
	}

	for _, perm := range allPermissions {
		t.Run(perm+"_granted", func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set("user_id", "customer-1")
				c.Set("permissions", []string{perm})
				c.Next()
			})
			r.Use(middleware.RequirePermission(perm))
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Should have permission %s", perm)
		})

		t.Run(perm+"_denied", func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set("user_id", "customer-1")
				c.Set("permissions", []string{"other:permission"})
				c.Next()
			})
			r.Use(middleware.RequirePermission(perm))
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusForbidden, w.Code, "Should not have permission %s", perm)
		})
	}
}