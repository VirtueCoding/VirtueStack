package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

// ─── ExtractResourceFromPath ─────────────────────────────────────────────────

func TestExtractResourceFromPath(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantType     string
		wantID       string
	}{
		{
			name:     "VM with UUID and sub-action",
			path:     "/api/v1/admin/vms/550e8400-e29b-41d4-a716-446655440000/start",
			wantType: "vm",
			wantID:   "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:     "customer with UUID",
			path:     "/api/v1/customers/550e8400-e29b-41d4-a716-446655440000",
			wantType: "customer",
			wantID:   "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:     "node collection without ID",
			path:     "/api/v1/nodes",
			wantType: "node",
			wantID:   "",
		},
		{
			name:     "template with opaque ID",
			path:     "/api/v1/admin/templates/uuid-here",
			wantType: "template",
			wantID:   "",
		},
		{
			name:     "template with UUID",
			path:     "/api/v1/admin/templates/550e8400-e29b-41d4-a716-446655440000",
			wantType: "template",
			wantID:   "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:     "api-keys collection",
			path:     "/api/v1/admin/api-keys",
			wantType: "api_key",
			wantID:   "",
		},
		{
			name:     "audit-logs collection",
			path:     "/api/v1/admin/audit-logs",
			wantType: "audit_log",
			wantID:   "",
		},
		{
			name:     "empty path",
			path:     "",
			wantType: "",
			wantID:   "",
		},
		{
			name:     "root path",
			path:     "/",
			wantType: "",
			wantID:   "",
		},
		{
			name:     "unknown path",
			path:     "/unknown-path",
			wantType: "",
			wantID:   "",
		},
		{
			name:     "snapshot with sub-action after ID",
			path:     "/api/v1/admin/snapshots/550e8400-e29b-41d4-a716-446655440000/restore",
			wantType: "snapshot",
			wantID:   "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:     "provisioning resource",
			path:     "/api/v1/provisioning",
			wantType: "provisioning",
			wantID:   "",
		},
		{
			name:     "node with numeric-like ID",
			path:     "/api/v1/admin/nodes/node123",
			wantType: "node",
			wantID:   "node123",
		},
		{
			name:     "deeply nested path picks first known resource",
			path:     "/api/v1/admin/vms/550e8400-e29b-41d4-a716-446655440000/snapshots/abc123",
			wantType: "vm",
			wantID:   "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:     "roles collection",
			path:     "/api/v1/admin/roles",
			wantType: "role",
			wantID:   "",
		},
		{
			name:     "permissions collection",
			path:     "/api/v1/admin/permissions",
			wantType: "permission",
			wantID:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotID := ExtractResourceFromPath(tt.path)
			assert.Equal(t, tt.wantType, gotType, "resourceType mismatch")
			assert.Equal(t, tt.wantID, gotID, "resourceID mismatch")
		})
	}
}

// ─── MapMethodToAction ───────────────────────────────────────────────────────

func TestMapMethodToAction(t *testing.T) {
	const uuid = "550e8400-e29b-41d4-a716-446655440000"

	tests := []struct {
		name       string
		method     string
		path       string
		wantAction string
	}{
		{
			name:       "POST collection → create",
			method:     http.MethodPost,
			path:       "/api/v1/admin/vms",
			wantAction: "vm.create",
		},
		{
			name:       "DELETE with ID → delete",
			method:     http.MethodDelete,
			path:       "/api/v1/admin/vms/" + uuid,
			wantAction: "vm.delete",
		},
		{
			name:       "PUT with ID → replace",
			method:     http.MethodPut,
			path:       "/api/v1/admin/vms/" + uuid,
			wantAction: "vm.replace",
		},
		{
			name:       "PATCH with ID → update",
			method:     http.MethodPatch,
			path:       "/api/v1/admin/vms/" + uuid,
			wantAction: "vm.update",
		},
		{
			name:       "POST sub-action start",
			method:     http.MethodPost,
			path:       "/api/v1/admin/vms/" + uuid + "/start",
			wantAction: "vm.start",
		},
		{
			name:       "POST sub-action stop",
			method:     http.MethodPost,
			path:       "/api/v1/admin/vms/" + uuid + "/stop",
			wantAction: "vm.stop",
		},
		{
			name:       "POST sub-action migrate",
			method:     http.MethodPost,
			path:       "/api/v1/admin/vms/" + uuid + "/migrate",
			wantAction: "vm.migrate",
		},
		{
			name:       "POST sub-action restore on snapshot",
			method:     http.MethodPost,
			path:       "/api/v1/admin/snapshots/" + uuid + "/restore",
			wantAction: "snapshot.restore",
		},
		{
			name:       "POST unknown resource → post.unknown",
			method:     http.MethodPost,
			path:       "/unknown",
			wantAction: "post.unknown",
		},
		{
			name:       "DELETE unknown resource → delete.unknown",
			method:     http.MethodDelete,
			path:       "/unknown",
			wantAction: "delete.unknown",
		},
		{
			name:       "POST node collection → create",
			method:     http.MethodPost,
			path:       "/api/v1/admin/nodes",
			wantAction: "node.create",
		},
		{
			name:       "PUT node with ID → replace",
			method:     http.MethodPut,
			path:       "/api/v1/admin/nodes/" + uuid,
			wantAction: "node.replace",
		},
		{
			name:       "POST to ID without sub-action → action",
			method:     http.MethodPost,
			path:       "/api/v1/admin/vms/" + uuid,
			wantAction: "vm.action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapMethodToAction(tt.method, tt.path)
			assert.Equal(t, tt.wantAction, got)
		})
	}
}

// ─── Audit middleware ────────────────────────────────────────────────────────

// capturedEntry holds the last entry written by a mock AuditLogger.
type capturedEntry struct {
	mu    sync.Mutex
	entry *AuditEntry
}

func (c *capturedEntry) get() *AuditEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entry
}

func newCapturingLogger(cap *capturedEntry) AuditLogger {
	return func(_ context.Context, entry *AuditEntry) error {
		cap.mu.Lock()
		defer cap.mu.Unlock()
		cap.entry = entry
		return nil
	}
}

func TestAudit_ReadOnlyMethodsSkipped(t *testing.T) {
	readOnlyMethods := []string{http.MethodGet, http.MethodHead, http.MethodOptions}

	for _, method := range readOnlyMethods {
		t.Run(method, func(t *testing.T) {
			called := false
			logger := func(_ context.Context, _ *AuditEntry) error {
				called = true
				return nil
			}

			r := gin.New()
			r.Use(Audit(logger))
			r.Handle(method, "/api/v1/admin/vms", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/api/v1/admin/vms", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.False(t, called, "audit logger should not be called for %s", method)
		})
	}
}

func TestAudit_POST_TriggersAuditLog(t *testing.T) {
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(Audit(newCapturingLogger(cap)))
	r.POST("/api/v1/admin/vms", func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/vms", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	entry := cap.get()
	require.NotNil(t, entry, "audit entry should be captured")
	assert.Equal(t, "vm.create", entry.Action)
	assert.Equal(t, "vm", entry.ResourceType)
	assert.Empty(t, entry.ResourceID)
	assert.True(t, entry.Success)
	assert.Empty(t, entry.ErrorMessage)
}

func TestAudit_DELETE_TriggersAuditLog(t *testing.T) {
	const uuid = "550e8400-e29b-41d4-a716-446655440000"
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(Audit(newCapturingLogger(cap)))
	r.DELETE("/api/v1/admin/vms/:id", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/vms/"+uuid, nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNoContent, w.Code)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.Equal(t, "vm.delete", entry.Action)
	assert.Equal(t, "vm", entry.ResourceType)
	assert.Equal(t, uuid, entry.ResourceID)
	assert.True(t, entry.Success)
}

func TestAudit_FailedResponse_SetsSuccessFalse(t *testing.T) {
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(Audit(newCapturingLogger(cap)))
	r.POST("/api/v1/admin/vms", func(c *gin.Context) {
		c.Status(http.StatusBadRequest)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/vms", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.False(t, entry.Success)
	assert.Equal(t, "HTTP 400", entry.ErrorMessage)
}

func TestAudit_FailedResponse_WithGinError(t *testing.T) {
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(Audit(newCapturingLogger(cap)))
	r.POST("/api/v1/admin/vms", func(c *gin.Context) {
		_ = c.Error(fmt.Errorf("hostname validation failed"))
		c.Status(http.StatusUnprocessableEntity)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/vms", nil)
	r.ServeHTTP(w, req)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.False(t, entry.Success)
	assert.Contains(t, entry.ErrorMessage, "hostname validation failed")
}

func TestAudit_SuccessResponse_Codes(t *testing.T) {
	successCodes := []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusNoContent,
	}

	for _, code := range successCodes {
		t.Run(fmt.Sprintf("HTTP_%d", code), func(t *testing.T) {
			cap := &capturedEntry{}

			r := gin.New()
			r.Use(Audit(newCapturingLogger(cap)))
			r.POST("/api/v1/admin/nodes", func(c *gin.Context) {
				c.Status(code)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/nodes", nil)
			r.ServeHTTP(w, req)

			entry := cap.get()
			require.NotNil(t, entry)
			assert.True(t, entry.Success, "status %d should be considered success", code)
			assert.Empty(t, entry.ErrorMessage)
		})
	}
}

func TestAudit_ActorID_FromUserID(t *testing.T) {
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(userIDContextKey, "admin-user-123")
		c.Set(userTypeContextKey, "admin")
		c.Next()
	})
	r.Use(Audit(newCapturingLogger(cap)))
	r.POST("/api/v1/admin/vms", func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/vms", nil)
	r.ServeHTTP(w, req)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.Equal(t, "admin-user-123", entry.ActorID)
}

func TestAudit_ActorID_FromAPIKey(t *testing.T) {
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(apiKeyIDContextKey, "key-abc-456")
		c.Next()
	})
	r.Use(Audit(newCapturingLogger(cap)))
	r.POST("/api/v1/admin/vms", func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/vms", nil)
	r.ServeHTTP(w, req)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.Equal(t, "key-abc-456", entry.ActorID)
}

func TestAudit_ActorType_FromContext(t *testing.T) {
	tests := []struct {
		name          string
		setContext    func(c *gin.Context)
		wantActorType string
	}{
		{
			name: "explicit actor_type takes precedence",
			setContext: func(c *gin.Context) {
				c.Set(actorTypeContextKey, "provisioning")
				c.Set(userTypeContextKey, "admin")
			},
			wantActorType: "provisioning",
		},
		{
			name: "falls back to user_type",
			setContext: func(c *gin.Context) {
				c.Set(userTypeContextKey, "customer")
			},
			wantActorType: "customer",
		},
		{
			name:          "empty when nothing set",
			setContext:     func(_ *gin.Context) {},
			wantActorType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := &capturedEntry{}

			r := gin.New()
			r.Use(func(c *gin.Context) {
				tt.setContext(c)
				c.Next()
			})
			r.Use(Audit(newCapturingLogger(cap)))
			r.PUT("/api/v1/admin/nodes/:id", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut,
				"/api/v1/admin/nodes/550e8400-e29b-41d4-a716-446655440000", nil)
			r.ServeHTTP(w, req)

			entry := cap.get()
			require.NotNil(t, entry)
			assert.Equal(t, tt.wantActorType, entry.ActorType)
		})
	}
}

func TestAudit_CorrelationID_Propagated(t *testing.T) {
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(CorrelationIDContextKey, "corr-xyz-789")
		c.Next()
	})
	r.Use(Audit(newCapturingLogger(cap)))
	r.POST("/api/v1/admin/templates", func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/templates", nil)
	r.ServeHTTP(w, req)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.Equal(t, "corr-xyz-789", entry.CorrelationID)
}

func TestAudit_LoggerError_DoesNotBreakResponse(t *testing.T) {
	failingLogger := func(_ context.Context, _ *AuditEntry) error {
		return errors.New("database unavailable")
	}

	r := gin.New()
	r.Use(Audit(failingLogger))
	r.POST("/api/v1/admin/vms", func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/vms", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code,
		"response should succeed even when audit logger fails")
}

func TestAudit_ClientIP_Captured(t *testing.T) {
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(Audit(newCapturingLogger(cap)))
	r.POST("/api/v1/admin/vms", func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/vms", nil)
	req.RemoteAddr = "198.51.100.42:12345"
	r.ServeHTTP(w, req)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.NotEmpty(t, entry.ActorIP)
}

func TestAudit_PATCH_TriggersAuditLog(t *testing.T) {
	const uuid = "550e8400-e29b-41d4-a716-446655440000"
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(Audit(newCapturingLogger(cap)))
	r.PATCH("/api/v1/admin/customers/:id", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/customers/"+uuid, nil)
	r.ServeHTTP(w, req)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.Equal(t, "customer.update", entry.Action)
	assert.Equal(t, "customer", entry.ResourceType)
	assert.Equal(t, uuid, entry.ResourceID)
	assert.True(t, entry.Success)
}

func TestAudit_PUT_TriggersAuditLog(t *testing.T) {
	const uuid = "550e8400-e29b-41d4-a716-446655440000"
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(Audit(newCapturingLogger(cap)))
	r.PUT("/api/v1/admin/nodes/:id", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/nodes/"+uuid, nil)
	r.ServeHTTP(w, req)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.Equal(t, "node.replace", entry.Action)
	assert.Equal(t, "node", entry.ResourceType)
	assert.Equal(t, uuid, entry.ResourceID)
	assert.True(t, entry.Success)
}

func TestAudit_ServerError_SetsSuccessFalse(t *testing.T) {
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(Audit(newCapturingLogger(cap)))
	r.POST("/api/v1/admin/vms", func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/vms", nil)
	r.ServeHTTP(w, req)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.False(t, entry.Success)
	assert.Equal(t, "HTTP 500", entry.ErrorMessage)
}

func TestAudit_SubAction_PostOnResource(t *testing.T) {
	const uuid = "550e8400-e29b-41d4-a716-446655440000"
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(Audit(newCapturingLogger(cap)))
	r.POST("/api/v1/admin/vms/:id/migrate", func(c *gin.Context) {
		c.Status(http.StatusAccepted)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/admin/vms/"+uuid+"/migrate", nil)
	r.ServeHTTP(w, req)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.Equal(t, "vm.migrate", entry.Action)
	assert.Equal(t, "vm", entry.ResourceType)
	assert.Equal(t, uuid, entry.ResourceID)
	assert.True(t, entry.Success)
}

func TestAudit_MultipleContextFields(t *testing.T) {
	const uuid = "550e8400-e29b-41d4-a716-446655440000"
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(userIDContextKey, "user-42")
		c.Set(actorTypeContextKey, "admin")
		c.Set(CorrelationIDContextKey, "corr-001")
		c.Next()
	})
	r.Use(Audit(newCapturingLogger(cap)))
	r.POST("/api/v1/admin/vms/:id/start", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/admin/vms/"+uuid+"/start", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	r.ServeHTTP(w, req)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.Equal(t, "user-42", entry.ActorID)
	assert.Equal(t, "admin", entry.ActorType)
	assert.Equal(t, "corr-001", entry.CorrelationID)
	assert.Equal(t, "vm.start", entry.Action)
	assert.Equal(t, "vm", entry.ResourceType)
	assert.Equal(t, uuid, entry.ResourceID)
	assert.True(t, entry.Success)
	assert.NotEmpty(t, entry.ActorIP)
}

func TestAudit_Redirect_SetsSuccessFalse(t *testing.T) {
	cap := &capturedEntry{}

	r := gin.New()
	r.Use(Audit(newCapturingLogger(cap)))
	r.POST("/api/v1/admin/vms", func(c *gin.Context) {
		c.Status(http.StatusMovedPermanently)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/vms", nil)
	r.ServeHTTP(w, req)

	entry := cap.get()
	require.NotNil(t, entry)
	assert.False(t, entry.Success, "3xx should not be treated as success")
}
