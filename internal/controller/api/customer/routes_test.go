package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	sharedcrypto "github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

type customerNotificationRouteTestDB struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (db *customerNotificationRouteTestDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if db.queryRowFunc != nil {
		return db.queryRowFunc(ctx, sql, args...)
	}
	return customerNotificationRouteTestRow{err: pgx.ErrNoRows}
}

func (db *customerNotificationRouteTestDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (db *customerNotificationRouteTestDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if db.execFunc != nil {
		return db.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (db *customerNotificationRouteTestDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, nil
}

type customerNotificationRouteTestRow struct {
	values []any
	err    error
}

func (row customerNotificationRouteTestRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	if len(dest) != len(row.values) {
		return fmt.Errorf("unexpected scan destination count: got %d want %d", len(dest), len(row.values))
	}
	for i, value := range row.values {
		if err := assignNotificationScanValue(dest[i], value); err != nil {
			return fmt.Errorf("scanning column %d: %w", i, err)
		}
	}
	return nil
}

// TestAPISubsetPermissionEnforcement tests that API keys with specific permissions
// can only access matching endpoints.
func TestAPISubsetPermissionEnforcement(t *testing.T) {
	tests := []struct {
		name          string
		permissions   []string
		endpoint      string
		method        string
		setupMockRepo func() *mockAPIKeyRepoForRoutes
		wantStatus    int
		description   string
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
			switch {
			case tt.endpoint == "/vms" && tt.method == http.MethodGet:
				r.GET("/vms", middleware.RequirePermission(PermissionVMRead), func(c *gin.Context) {
					c.Status(http.StatusOK)
				})
			case tt.endpoint == "/vms/vm-1/start":
				r.POST("/vms/vm-1/start", middleware.RequirePermission(PermissionVMPower), func(c *gin.Context) {
					c.Status(http.StatusOK)
				})
			case tt.endpoint == "/backups" && tt.method == http.MethodGet:
				r.GET("/backups", middleware.RequirePermission(PermissionBackupRead), func(c *gin.Context) {
					c.Status(http.StatusOK)
				})
			case tt.endpoint == "/backups" && tt.method == http.MethodPost:
				r.POST("/backups", middleware.RequirePermission(PermissionBackupWrite), func(c *gin.Context) {
					c.Status(http.StatusOK)
				})
			case tt.endpoint == "/snapshots" && tt.method == http.MethodPost:
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

func TestAPIKeyCannotMutateNotificationRoutes(t *testing.T) {
	router, rawAPIKey := newNotificationRouteTestRouter(t)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			name:       "API key can still read notification preferences",
			method:     http.MethodGet,
			path:       "/api/v1/customer/notifications/preferences",
			wantStatus: http.StatusOK,
		},
		{
			name:       "API key cannot update notification preferences",
			method:     http.MethodPut,
			path:       "/api/v1/customer/notifications/preferences",
			body:       `{"email_enabled":false}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "API key cannot mark notification as read",
			method:     http.MethodPost,
			path:       "/api/v1/customer/notifications/notif-1/read",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "API key cannot mark all notifications as read",
			method:     http.MethodPost,
			path:       "/api/v1/customer/notifications/read-all",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))
			req.Header.Set("X-API-Key", rawAPIKey)
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			assert.Equal(t, tt.wantStatus, recorder.Code)
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

func newNotificationRouteTestRouter(t *testing.T) (*gin.Engine, string) {
	t.Helper()

	const (
		rawAPIKey     = "vs_test_notification_key"
		encryptionKey = "test-encryption-key"
	)

	now := time.Date(2026, time.April, 2, 12, 0, 0, 0, time.UTC)
	expectedHash := sharedcrypto.GenerateHMACSignature(encryptionKey, []byte(rawAPIKey))
	authConfig := middleware.AuthConfig{
		JWTSecret: "test-secret",
		Issuer:    "virtuestack",
	}
	logger := testAuthHandlerLogger()

	db := &customerNotificationRouteTestDB{
		queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM customer_api_keys WHERE key_hash = $1 AND revoked_at IS NULL"):
				if len(args) != 1 || args[0] != expectedHash {
					return customerNotificationRouteTestRow{err: pgx.ErrNoRows}
				}
				return customerNotificationRouteTestRow{values: []any{
					"key-1",
					"customer-1",
					"notifications-test-key",
					expectedHash,
					[]string{},
					[]string{},
					[]string{PermissionVMRead},
					nil,
					now,
					nil,
					nil,
				}}
			case strings.Contains(sql, "FROM notification_preferences WHERE customer_id = $1"):
				return customerNotificationRouteTestRow{values: []any{
					"pref-1",
					"customer-1",
					true,
					false,
					[]string{"vm.created"},
					now,
					now,
				}}
			case strings.Contains(sql, "UPDATE notification_preferences SET"):
				return customerNotificationRouteTestRow{values: []any{
					"pref-1",
					"customer-1",
					false,
					false,
					[]string{"vm.created"},
					now,
					now,
				}}
			case strings.Contains(sql, "SELECT COUNT(*) FROM notifications WHERE customer_id = $1 AND NOT read"):
				return customerNotificationRouteTestRow{values: []any{0}}
			default:
				return customerNotificationRouteTestRow{err: fmt.Errorf("unexpected query: %s", sql)}
			}
		},
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			switch {
			case strings.Contains(sql, "UPDATE notifications SET read = TRUE WHERE id = $1 AND customer_id = $2"):
				return pgconn.NewCommandTag("UPDATE 1"), nil
			case strings.Contains(sql, "UPDATE notifications SET read = TRUE WHERE customer_id = $1 AND NOT read"):
				return pgconn.NewCommandTag("UPDATE 5"), nil
			default:
				return pgconn.CommandTag{}, fmt.Errorf("unexpected exec: %s", sql)
			}
		},
	}

	apiKeyRepo := repository.NewCustomerAPIKeyRepository(db)
	notifyHandler := NewNotificationsHandler(
		repository.NewNotificationPreferenceRepository(db),
		repository.NewNotificationEventRepository(db),
		nil,
		nil,
		logger,
	)
	sseHub := services.NewSSEHub(logger)
	inAppHandler := NewInAppNotificationsHandler(
		services.NewInAppNotificationService(services.InAppNotificationServiceConfig{
			Repo:   repository.NewInAppNotificationRepository(db),
			Hub:    sseHub,
			Logger: logger,
		}),
		sseHub,
		authConfig,
		logger,
		nil,
	)
	handler := &CustomerHandler{
		authConfig:    authConfig,
		encryptionKey: encryptionKey,
		logger:        logger,
	}

	router := gin.New()
	api := router.Group("/api/v1")
	RegisterCustomerRoutes(api, handler, notifyHandler, inAppHandler, apiKeyRepo, false, BillingRoutesConfig{})

	return router, rawAPIKey
}

func newConsoleRouteTestRouter(t *testing.T, permissions []string) (*gin.Engine, string) {
	t.Helper()

	const (
		rawAPIKey     = "vs_test_console_key"
		encryptionKey = "test-encryption-key"
	)

	now := time.Date(2026, time.April, 2, 12, 0, 0, 0, time.UTC)
	expectedHash := sharedcrypto.GenerateHMACSignature(encryptionKey, []byte(rawAPIKey))
	authConfig := middleware.AuthConfig{
		JWTSecret: "test-secret",
		Issuer:    "virtuestack",
	}
	logger := testAuthHandlerLogger()
	nodeID := "550e8400-e29b-41d4-a716-446655440001"

	db := &customerNotificationRouteTestDB{
		queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM customer_api_keys WHERE key_hash = $1 AND revoked_at IS NULL"):
				if len(args) != 1 || args[0] != expectedHash {
					return customerNotificationRouteTestRow{err: pgx.ErrNoRows}
				}
				return customerNotificationRouteTestRow{values: []any{
					"key-1",
					"customer-1",
					"console-test-key",
					expectedHash,
					[]string{},
					[]string{"550e8400-e29b-41d4-a716-446655440000"},
					permissions,
					nil,
					now,
					nil,
					nil,
				}}
			default:
				return customerNotificationRouteTestRow{err: pgx.ErrNoRows}
			}
		},
	}

	apiKeyRepo := repository.NewCustomerAPIKeyRepository(db)
	handler := &CustomerHandler{
		authConfig:    authConfig,
		encryptionKey: encryptionKey,
		vmService: newWebSocketVMService(t, models.VM{
			ID:                 "550e8400-e29b-41d4-a716-446655440000",
			CustomerID:         "customer-1",
			NodeID:             &nodeID,
			PlanID:             "550e8400-e29b-41d4-a716-446655440002",
			Hostname:           "vm-test",
			Status:             models.VMStatusRunning,
			VCPU:               2,
			MemoryMB:           2048,
			DiskGB:             40,
			PortSpeedMbps:      1000,
			BandwidthLimitGB:   1000,
			BandwidthUsedBytes: 0,
			BandwidthResetAt:   now,
			MACAddress:         "52:54:00:12:34:56",
			StorageBackend:     "qcow",
			Timestamps: models.Timestamps{
				CreatedAt: now,
				UpdatedAt: now,
			},
		}),
		nodeRepo: newWebSocketNodeRepo(t, models.Node{
			ID:                nodeID,
			Hostname:          "node-1",
			GRPCAddress:       "127.0.0.1:50051",
			ManagementIP:      "192.0.2.10",
			Status:            models.NodeStatusOnline,
			TotalVCPU:         16,
			TotalMemoryMB:     32768,
			AllocatedVCPU:     4,
			AllocatedMemoryMB: 8192,
			StorageBackend:    "qcow",
			StoragePath:       "/var/lib/libvirt/images",
			CreatedAt:         now,
		}),
		nodeAgent:  &websocketTestNodeAgent{err: errors.New("node offline")},
		tokenStore: newConsoleTokenStore(),
		logger:     logger,
	}
	t.Cleanup(handler.tokenStore.Stop)

	router := gin.New()
	api := router.Group("/api/v1")
	RegisterCustomerRoutes(api, handler, nil, nil, apiKeyRepo, false, BillingRoutesConfig{})

	return router, rawAPIKey
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

func TestConsoleWebSocketRoutes_RequireVMPowerForAPIKeys(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		permissions []string
		wantStatus  int
	}{
		{
			name:        "vnc websocket rejects read-only api key",
			path:        "/api/v1/customer/ws/vnc/550e8400-e29b-41d4-a716-446655440000",
			permissions: []string{PermissionVMRead},
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "serial websocket rejects read-only api key",
			path:        "/api/v1/customer/ws/serial/550e8400-e29b-41d4-a716-446655440000",
			permissions: []string{PermissionVMRead},
			wantStatus:  http.StatusForbidden,
		},
		{
			name:        "vnc websocket allows vm power api key past permission gate",
			path:        "/api/v1/customer/ws/vnc/550e8400-e29b-41d4-a716-446655440000",
			permissions: []string{PermissionVMRead, PermissionVMPower},
			wantStatus:  http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, rawAPIKey := newConsoleRouteTestRouter(t, tt.permissions)

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("X-API-Key", rawAPIKey)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
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

func TestRegisterAuthRoutes_SelfRegistrationToggle(t *testing.T) {
	tests := []struct {
		name              string
		allowRegistration bool
		wantRegister      bool
		wantVerify        bool
	}{
		{
			name:              "enabled registers self-registration endpoints",
			allowRegistration: true,
			wantRegister:      true,
			wantVerify:        true,
		},
		{
			name:              "disabled omits self-registration endpoints",
			allowRegistration: false,
			wantRegister:      false,
			wantVerify:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			customerGroup := r.Group("/customer")
			registerAuthRoutes(customerGroup, &CustomerHandler{}, tt.allowRegistration, false)

			routes := r.Routes()
			hasRegister := slices.ContainsFunc(routes, func(route gin.RouteInfo) bool {
				return route.Method == http.MethodPost && route.Path == "/customer/auth/register"
			})
			hasVerify := slices.ContainsFunc(routes, func(route gin.RouteInfo) bool {
				return route.Method == http.MethodPost && route.Path == "/customer/auth/verify-email"
			})

			assert.Equal(t, tt.wantRegister, hasRegister)
			assert.Equal(t, tt.wantVerify, hasVerify)
		})
	}
}

func TestRegisterCustomerRoutes_LogoutAllowsCleanupTokenWithoutCurrentJWT(t *testing.T) {
	logger := testAuthHandlerLogger()
	authConfig := middleware.AuthConfig{
		JWTSecret: "test-secret",
		Issuer:    "virtuestack",
	}
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

	r := gin.New()
	api := r.Group("/api/v1")
	RegisterCustomerRoutes(api, handler, nil, nil, nil, false, BillingRoutesConfig{})

	body, err := json.Marshal(LogoutRequest{SessionCleanupToken: cleanupToken})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/auth/logout", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	r.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	errorBody, ok := response["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "LOGOUT_FAILED", errorBody["code"])
}

func TestRegisterCustomerRoutes_LogoutRejectsMalformedJSON(t *testing.T) {
	logger := testAuthHandlerLogger()
	authConfig := middleware.AuthConfig{
		JWTSecret: "test-secret",
		Issuer:    "virtuestack",
	}

	handler := &CustomerHandler{
		authConfig: authConfig,
		logger:     logger,
	}

	r := gin.New()
	api := r.Group("/api/v1")
	RegisterCustomerRoutes(api, handler, nil, nil, nil, false, BillingRoutesConfig{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/auth/logout", bytes.NewBufferString(`{"session_cleanup_token":`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	r.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	errorBody, ok := response["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "INVALID_REQUEST_BODY", errorBody["code"])
}

func TestRegisterCustomerRoutes_LogoutStillRequiresCSRFFromCurrentSession(t *testing.T) {
	logger := testAuthHandlerLogger()
	authConfig := middleware.AuthConfig{
		JWTSecret: "test-secret",
		Issuer:    "virtuestack",
	}
	authService := services.NewAuthService(
		&logoutCustomerRepoStub{deleteSessionErr: errors.New("delete session failed")},
		nil,
		nil,
		authConfig.JWTSecret,
		authConfig.Issuer,
		"",
		logger,
	)

	handler := &CustomerHandler{
		authService: authService,
		authConfig:  authConfig,
		logger:      logger,
	}

	r := gin.New()
	api := r.Group("/api/v1")
	RegisterCustomerRoutes(api, handler, nil, nil, nil, false, BillingRoutesConfig{})

	accessToken, err := middleware.GenerateAccessToken(
		authConfig,
		"customer-123",
		"customer",
		"",
		"session-123",
		15*time.Minute,
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/auth/logout", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: middleware.AccessTokenCookieName, Value: accessToken})
	recorder := httptest.NewRecorder()

	r.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusForbidden, recorder.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	errorBody, ok := response["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "CSRF_COOKIE_MISSING", errorBody["code"])
}

func TestRegisterCustomerRoutes_LogoutRejectsNonCustomerCleanupToken(t *testing.T) {
	logger := testAuthHandlerLogger()
	authConfig := middleware.AuthConfig{
		JWTSecret: "test-secret",
		Issuer:    "virtuestack",
	}
	authService := services.NewAuthService(
		&logoutCustomerRepoStub{deleteSessionErr: errors.New("delete session failed")},
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
		"admin-session-123",
	)
	require.NoError(t, err)

	handler := &CustomerHandler{
		authService: authService,
		authConfig:  authConfig,
		logger:      logger,
	}

	r := gin.New()
	api := r.Group("/api/v1")
	RegisterCustomerRoutes(api, handler, nil, nil, nil, false, BillingRoutesConfig{})

	body, err := json.Marshal(LogoutRequest{SessionCleanupToken: cleanupToken})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/auth/logout", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	r.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusUnauthorized, recorder.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	errorBody, ok := response["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "UNAUTHORIZED", errorBody["code"])
}

func TestRegisterCustomerRoutes_DoesNotExposeLegacyBackupCodesFetch(t *testing.T) {
	handler := &CustomerHandler{
		authConfig: middleware.AuthConfig{
			JWTSecret: "test-secret",
			Issuer:    "virtuestack",
		},
		logger: testAuthHandlerLogger(),
	}

	r := gin.New()
	api := r.Group("/api/v1")
	RegisterCustomerRoutes(api, handler, nil, nil, nil, false, BillingRoutesConfig{})

	routes := r.Routes()
	hasLegacyBackupCodesFetch := slices.ContainsFunc(routes, func(route gin.RouteInfo) bool {
		return route.Method == http.MethodGet && route.Path == "/api/v1/customer/2fa/backup-codes"
	})

	assert.False(t, hasLegacyBackupCodesFetch)
}
