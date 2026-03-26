package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetAdmin_GetAdmin(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	admin := &models.Admin{
		ID:    "admin-1",
		Email: "admin@example.com",
		Role:  models.RoleSuperAdmin,
	}

	SetAdmin(c, admin)
	got := GetAdmin(c)

	require.NotNil(t, got)
	assert.Equal(t, "admin-1", got.ID)
	assert.Equal(t, "admin@example.com", got.Email)
}

func TestGetAdmin_NotSet(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	got := GetAdmin(c)
	assert.Nil(t, got)
}

func TestGetAdmin_WrongType(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set(adminContextKey, "not-an-admin-pointer")
	got := GetAdmin(c)
	assert.Nil(t, got)
}

func TestAdminLoader_Success(t *testing.T) {
	admin := &models.Admin{
		ID:    "admin-1",
		Email: "admin@example.com",
		Role:  models.RoleAdmin,
	}
	fetcher := func(_ context.Context, id string) (*models.Admin, error) {
		if id == "admin-1" {
			return admin, nil
		}
		return nil, errors.New("not found")
	}

	r := gin.New()
	r.Use(CorrelationID())
	r.Use(func(c *gin.Context) {
		// Simulate JWTAuth setting user_id
		c.Set("user_id", "admin-1")
		c.Next()
	})
	r.Use(AdminLoader(fetcher))
	r.GET("/test", func(c *gin.Context) {
		a := GetAdmin(c)
		require.NotNil(t, a)
		c.JSON(http.StatusOK, gin.H{"admin_id": a.ID})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAdminLoader_MissingUserID(t *testing.T) {
	fetcher := func(_ context.Context, _ string) (*models.Admin, error) {
		return nil, errors.New("should not be called")
	}

	r := gin.New()
	r.Use(CorrelationID())
	// No user_id set in context
	r.Use(AdminLoader(fetcher))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminLoader_FetchError(t *testing.T) {
	fetcher := func(_ context.Context, _ string) (*models.Admin, error) {
		return nil, errors.New("database error")
	}

	r := gin.New()
	r.Use(CorrelationID())
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "admin-1")
		c.Next()
	})
	r.Use(AdminLoader(fetcher))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRequireAdminPermission(t *testing.T) {
	tests := []struct {
		name       string
		admin      *models.Admin
		required   models.Permission
		wantStatus int
	}{
		{
			name: "super_admin has all permissions",
			admin: &models.Admin{
				ID:   "admin-1",
				Role: models.RoleSuperAdmin,
			},
			required:   models.PermissionVMsDelete,
			wantStatus: http.StatusOK,
		},
		{
			name: "viewer lacks write permission",
			admin: &models.Admin{
				ID:   "admin-2",
				Role: models.RoleViewer,
			},
			required:   models.PermissionVMsWrite,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "nil admin returns forbidden",
			admin:      nil,
			required:   models.PermissionVMsRead,
			wantStatus: http.StatusForbidden,
		},
		{
			name: "admin with explicit perms",
			admin: &models.Admin{
				ID:          "admin-3",
				Role:        models.RoleViewer,
				Permissions: []models.Permission{models.PermissionVMsWrite},
			},
			required:   models.PermissionVMsWrite,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				if tt.admin != nil {
					SetAdmin(c, tt.admin)
				}
				c.Next()
			})
			r.Use(RequireAdminPermission(tt.required))
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestRequireAnyAdminPermission(t *testing.T) {
	tests := []struct {
		name       string
		admin      *models.Admin
		required   []models.Permission
		wantStatus int
	}{
		{
			name: "has one of required",
			admin: &models.Admin{
				ID:   "admin-1",
				Role: models.RoleAdmin,
			},
			required:   []models.Permission{models.PermissionVMsRead, models.PermissionPlansDelete},
			wantStatus: http.StatusOK,
		},
		{
			name: "has none of required",
			admin: &models.Admin{
				ID:   "admin-2",
				Role: models.RoleViewer,
			},
			required:   []models.Permission{models.PermissionVMsWrite, models.PermissionVMsDelete},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				SetAdmin(c, tt.admin)
				c.Next()
			})
			r.Use(RequireAnyAdminPermission(tt.required))
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestRequireAllAdminPermissions(t *testing.T) {
	tests := []struct {
		name       string
		admin      *models.Admin
		required   []models.Permission
		wantStatus int
	}{
		{
			name: "has all required",
			admin: &models.Admin{
				ID:   "admin-1",
				Role: models.RoleSuperAdmin,
			},
			required:   []models.Permission{models.PermissionVMsRead, models.PermissionVMsWrite},
			wantStatus: http.StatusOK,
		},
		{
			name: "missing one required",
			admin: &models.Admin{
				ID:   "admin-2",
				Role: models.RoleViewer,
			},
			required:   []models.Permission{models.PermissionVMsRead, models.PermissionVMsWrite},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				SetAdmin(c, tt.admin)
				c.Next()
			})
			r.Use(RequireAllAdminPermissions(tt.required))
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestRequireAdminPermission_ResponseFormat(t *testing.T) {
	r := gin.New()
	r.Use(CorrelationID())
	r.Use(func(c *gin.Context) {
		SetAdmin(c, &models.Admin{
			ID:   "admin-1",
			Role: models.RoleViewer,
		})
		c.Next()
	})
	r.Use(RequireAdminPermission(models.PermissionVMsDelete))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "FORBIDDEN", resp.Error.Code)
}
