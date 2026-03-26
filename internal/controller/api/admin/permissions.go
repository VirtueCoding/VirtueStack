// Package admin provides HTTP handlers for the Admin API.
package admin

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PermissionInfo represents a permission with its description.
type PermissionInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// permissionDescriptions maps permission names to their human-readable descriptions.
var permissionDescriptions = map[models.Permission]string{
	models.PermissionPlansRead:       "View plans",
	models.PermissionPlansWrite:      "Create and update plans",
	models.PermissionPlansDelete:     "Delete plans",
	models.PermissionNodesRead:       "View nodes",
	models.PermissionNodesWrite:      "Create and update nodes",
	models.PermissionNodesDelete:     "Delete nodes",
	models.PermissionCustomersRead:   "View customers",
	models.PermissionCustomersWrite:  "Update customer accounts",
	models.PermissionCustomersDelete: "Delete customers",
	models.PermissionVMsRead:         "View VMs",
	models.PermissionVMsWrite:        "Create and modify VMs",
	models.PermissionVMsDelete:       "Delete VMs",
	models.PermissionSettingsRead:    "View settings",
	models.PermissionSettingsWrite:   "Modify settings",
	models.PermissionBackupsRead:     "View backups",
	models.PermissionBackupsWrite:    "Manage backups",
	models.PermissionIPSetsRead:      "View IP sets",
	models.PermissionIPSetsWrite:     "Create and update IP sets",
	models.PermissionIPSetsDelete:    "Delete IP sets",
	models.PermissionTemplatesRead:   "View templates",
	models.PermissionTemplatesWrite:  "Manage templates",
	models.PermissionRDNSRead:        "View RDNS records",
	models.PermissionRDNSWrite:       "Manage RDNS records",
	models.PermissionAuditLogsRead:   "View audit logs",
}

// UpdatePermissionsRequest holds the request body for updating admin permissions.
type UpdatePermissionsRequest struct {
	Permissions []string `json:"permissions" validate:"required,dive,required"`
}

// ListPermissions returns all available permissions with their descriptions.
// Any authenticated admin can call this endpoint.
func (h *AdminHandler) ListPermissions(c *gin.Context) {
	allPerms := models.GetAllPermissions()
	permissions := make([]PermissionInfo, 0, len(allPerms))

	for _, perm := range allPerms {
		description, ok := permissionDescriptions[perm]
		if !ok {
			description = string(perm)
		}
		permissions = append(permissions, PermissionInfo{
			Name:        string(perm),
			Description: description,
		})
	}

	c.JSON(http.StatusOK, models.Response{
		Data: map[string]any{"permissions": permissions},
	})
}

// listAdminsMaxPerPage is the maximum number of admins returned per page.
const listAdminsMaxPerPage = 100

// ListAdmins returns a paginated list of all admins with their permissions.
// Only super_admin can call this endpoint.
func (h *AdminHandler) ListAdmins(c *gin.Context) {
	pagination := models.ParsePagination(c)

	// Enforce a sensible maximum per-page for this endpoint
	if pagination.PerPage > listAdminsMaxPerPage {
		pagination.PerPage = listAdminsMaxPerPage
	}

	actorID := middleware.GetUserID(c)
	h.logAuditEvent(c, "admin.list", "admin", "", map[string]any{
		"actor_id": actorID,
	}, true)

	admins, _, err := h.adminRepo.List(c.Request.Context(), repository.AdminListFilter{
		PaginationParams: pagination,
	})
	if err != nil {
		h.logger.Error("failed to list admins",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list admins")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: admins})
}

// UpdateAdminPermissions updates an admin's permissions.
// Only super_admin can call this endpoint.
func (h *AdminHandler) UpdateAdminPermissions(c *gin.Context) {
	adminID := c.Param("admin_id")
	if adminID == "" {
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "admin_id is required")
		return
	}

	// Validate admin_id as a valid UUID (F-173)
	if _, err := uuid.Parse(adminID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_ADMIN_ID", "admin_id must be a valid UUID")
		return
	}

	// Prevent an admin from modifying their own permissions (F-065)
	requestingAdminID := middleware.GetUserID(c)
	if adminID == requestingAdminID {
		middleware.RespondWithError(c, http.StatusBadRequest, "SELF_PERMISSION_CHANGE_FORBIDDEN",
			"admins cannot modify their own permissions")
		return
	}

	var req UpdatePermissionsRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Guard against excessively large permission slices (F-087).
	// The maximum number of valid permissions is the size of the known permission universe.
	if len(req.Permissions) > len(models.GetAllPermissions()) {
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "too many permissions specified")
		return
	}

	// Validate that all permissions are valid
	validPerms := make([]models.Permission, 0, len(req.Permissions))
	allValidPerms := models.GetAllPermissions()
	validPermSet := make(map[models.Permission]struct{}, len(allValidPerms))
	for _, p := range allValidPerms {
		validPermSet[p] = struct{}{}
	}

	for _, permStr := range req.Permissions {
		perm := models.Permission(permStr)
		if _, ok := validPermSet[perm]; !ok {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_PERMISSION",
				"invalid permission: "+permStr)
			return
		}
		validPerms = append(validPerms, perm)
	}

	// Update admin permissions via auth service
	admin, err := h.authService.UpdateAdminPermissions(c.Request.Context(), adminID, validPerms)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "NOT_FOUND", "admin not found")
			return
		}
		h.logger.Error("failed to update admin permissions",
			"admin_id", adminID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update permissions")
		return
	}

	// Log audit event
	h.logAuditEvent(c, "update_permissions", "admin", adminID, map[string]any{
		"permissions": validPerms,
	}, true)

	c.JSON(http.StatusOK, models.Response{Data: map[string]any{"admin": admin}})
}
