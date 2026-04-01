// Package middleware provides HTTP middleware for the VirtueStack Controller.
package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

const (
	// adminContextKey is the gin context key for the admin object.
	adminContextKey = "admin"
)

// AdminFetcher is a function type that fetches an admin by ID from the database.
// Returns an error if the admin is not found or on database error.
type AdminFetcher func(ctx context.Context, adminID string) (*models.Admin, error)

// SetAdmin stores the admin in the gin context.
// This should be called by middleware that loads the admin after authentication.
func SetAdmin(c *gin.Context, admin *models.Admin) {
	c.Set(adminContextKey, admin)
}

// GetAdmin retrieves the admin from the gin context.
// Returns nil if not present.
func GetAdmin(c *gin.Context) *models.Admin {
	v, exists := c.Get(adminContextKey)
	if !exists {
		return nil
	}
	admin, ok := v.(*models.Admin)
	if !ok {
		return nil
	}
	return admin
}

// AdminLoader returns a middleware that loads the full admin object from the database.
// Must be used after JWTAuth middleware, which sets user_id in the context.
// On success, it calls SetAdmin to store the admin in context for downstream handlers.
// Returns 401 if user_id is not found in context (auth missing or invalid).
// Returns 500 if the admin fetch fails (database error or admin not found).
func AdminLoader(fetcher AdminFetcher) gin.HandlerFunc {
	return func(c *gin.Context) {
		adminID := GetUserID(c)
		if adminID == "" {
			RespondWithError(c, http.StatusUnauthorized, "UNAUTHORIZED",
				"user id not found in context")
			return
		}

		admin, err := fetcher(c.Request.Context(), adminID)
		if err != nil {
			slog.Error("failed to fetch admin",
				"admin_id", adminID,
				"error", err,
				"correlation_id", GetCorrelationID(c),
			)
			RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR",
				"failed to load admin account")
			return
		}

		SetAdmin(c, admin)
		c.Next()
	}
}

// RequireAdminPermission returns a middleware that checks if the authenticated admin
// has the required permission. Must be used after authentication middleware that
// sets the admin in context (e.g., after AdminLoader middleware).
// Returns 403 Forbidden if the permission is denied.
func RequireAdminPermission(required models.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin := GetAdmin(c)
		if admin == nil {
			RespondWithError(c, http.StatusForbidden, "FORBIDDEN",
				"You do not have permission to perform this action")
			return
		}

		permissions := admin.GetEffectivePermissions()
		if !models.HasPermission(permissions, required) {
			RespondWithError(c, http.StatusForbidden, "FORBIDDEN",
				"You do not have permission to perform this action")
			return
		}

		c.Next()
	}
}

// RequireAnyAdminPermission returns a middleware that checks if the authenticated admin
// has at least one of the required permissions (OR logic).
// Must be used after authentication middleware that sets the admin in context.
// Returns 403 Forbidden if none of the permissions are granted.
func RequireAnyAdminPermission(required []models.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin := GetAdmin(c)
		if admin == nil {
			RespondWithError(c, http.StatusForbidden, "FORBIDDEN",
				"You do not have permission to perform this action")
			return
		}

		permissions := admin.GetEffectivePermissions()
		if !models.HasAnyPermission(permissions, required) {
			RespondWithError(c, http.StatusForbidden, "FORBIDDEN",
				"You do not have permission to perform this action")
			return
		}

		c.Next()
	}
}

// RequireAllAdminPermissions returns a middleware that checks if the authenticated admin
// has all of the required permissions (AND logic).
// Must be used after authentication middleware that sets the admin in context.
// Returns 403 Forbidden if any permission is missing.
func RequireAllAdminPermissions(required []models.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		admin := GetAdmin(c)
		if admin == nil {
			RespondWithError(c, http.StatusForbidden, "FORBIDDEN",
				"You do not have permission to perform this action")
			return
		}

		permissions := admin.GetEffectivePermissions()
		if !models.HasAllPermissions(permissions, required) {
			RespondWithError(c, http.StatusForbidden, "FORBIDDEN",
				"You do not have permission to perform this action")
			return
		}

		c.Next()
	}
}
