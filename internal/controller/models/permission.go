// Package models provides data model types for VirtueStack Controller.
package models

// Permission is a typed string representing a fine-grained access control permission.
type Permission string

// Permission constants for plans resource.
const (
	PermissionPlansRead   Permission = "plans:read"
	PermissionPlansWrite  Permission = "plans:write"
	PermissionPlansDelete Permission = "plans:delete"
)

// Permission constants for nodes resource.
const (
	PermissionNodesRead   Permission = "nodes:read"
	PermissionNodesWrite  Permission = "nodes:write"
	PermissionNodesDelete Permission = "nodes:delete"
)

// Permission constants for customers resource.
const (
	PermissionCustomersRead   Permission = "customers:read"
	PermissionCustomersWrite  Permission = "customers:write"
	PermissionCustomersDelete Permission = "customers:delete"
)

// Permission constants for VMs resource.
const (
	PermissionVMsRead   Permission = "vms:read"
	PermissionVMsWrite  Permission = "vms:write"
	PermissionVMsDelete Permission = "vms:delete"
)

// Permission constants for settings resource.
const (
	PermissionSettingsRead  Permission = "settings:read"
	PermissionSettingsWrite Permission = "settings:write"
)

// Permission constants for backups resource.
const (
	PermissionBackupsRead  Permission = "backups:read"
	PermissionBackupsWrite Permission = "backups:write"
)

// Permission constants for IP sets resource.
const (
	PermissionIPSetsRead   Permission = "ipsets:read"
	PermissionIPSetsWrite  Permission = "ipsets:write"
	PermissionIPSetsDelete Permission = "ipsets:delete"
)

// Permission constants for templates resource.
const (
	PermissionTemplatesRead  Permission = "templates:read"
	PermissionTemplatesWrite Permission = "templates:write"
)

// Permission constants for RDNS resource.
const (
	PermissionRDNSRead  Permission = "rdns:read"
	PermissionRDNSWrite Permission = "rdns:write"
)

// Permission constants for audit logs resource.
const (
	PermissionAuditLogsRead Permission = "audit_logs:read"
)

// Permission constants for storage backends resource.
const (
	PermissionStorageBackendsRead   Permission = "storage_backends:read"
	PermissionStorageBackendsWrite  Permission = "storage_backends:write"
	PermissionStorageBackendsDelete Permission = "storage_backends:delete"
)

// Permission constants for billing resource.
const (
	PermissionBillingRead  Permission = "billing:read"
	PermissionBillingWrite Permission = "billing:write"
)

// Role constants for default permission sets.
const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleViewer     = "viewer"
)

// allPermissions contains all available permissions.
var allPermissions = []Permission{
	PermissionPlansRead, PermissionPlansWrite, PermissionPlansDelete,
	PermissionNodesRead, PermissionNodesWrite, PermissionNodesDelete,
	PermissionCustomersRead, PermissionCustomersWrite, PermissionCustomersDelete,
	PermissionVMsRead, PermissionVMsWrite, PermissionVMsDelete,
	PermissionSettingsRead, PermissionSettingsWrite,
	PermissionBackupsRead, PermissionBackupsWrite,
	PermissionIPSetsRead, PermissionIPSetsWrite, PermissionIPSetsDelete,
	PermissionTemplatesRead, PermissionTemplatesWrite,
	PermissionRDNSRead, PermissionRDNSWrite,
	PermissionAuditLogsRead,
	PermissionStorageBackendsRead, PermissionStorageBackendsWrite, PermissionStorageBackendsDelete,
	PermissionBillingRead, PermissionBillingWrite,
}

// defaultPermissions maps roles to their default permission sets.
var defaultPermissions = map[string][]Permission{
	RoleSuperAdmin: allPermissions,
	RoleAdmin: {
		PermissionPlansRead, PermissionPlansWrite,
		PermissionNodesRead, PermissionNodesWrite,
		PermissionCustomersRead, PermissionCustomersWrite,
		PermissionVMsRead, PermissionVMsWrite, PermissionVMsDelete,
		PermissionSettingsRead,
		PermissionBackupsRead, PermissionBackupsWrite,
		PermissionIPSetsRead, PermissionIPSetsWrite,
		PermissionTemplatesRead, PermissionTemplatesWrite,
		PermissionRDNSRead, PermissionRDNSWrite,
		PermissionAuditLogsRead,
		PermissionStorageBackendsRead, PermissionStorageBackendsWrite,
		PermissionBillingRead, PermissionBillingWrite,
	},
	RoleViewer: {
		PermissionPlansRead,
		PermissionNodesRead,
		PermissionCustomersRead,
		PermissionVMsRead,
		PermissionSettingsRead,
		PermissionBackupsRead,
		PermissionIPSetsRead,
		PermissionTemplatesRead,
		PermissionRDNSRead,
		PermissionAuditLogsRead,
		PermissionStorageBackendsRead,
		PermissionBillingRead,
	},
}

// GetAllPermissions returns all available permissions.
func GetAllPermissions() []Permission {
	result := make([]Permission, len(allPermissions))
	copy(result, allPermissions)
	return result
}

// GetDefaultPermissions returns the default permissions for a given role.
// Returns nil if the role is not recognized.
func GetDefaultPermissions(role string) []Permission {
	perms, ok := defaultPermissions[role]
	if !ok {
		return nil
	}
	result := make([]Permission, len(perms))
	copy(result, perms)
	return result
}

// HasPermission checks if the required permission is present in the user's permissions.
func HasPermission(userPermissions []Permission, required Permission) bool {
	for _, p := range userPermissions {
		if p == required {
			return true
		}
	}
	return false
}

// HasAnyPermission checks if any of the required permissions are present.
func HasAnyPermission(userPermissions []Permission, required []Permission) bool {
	for _, r := range required {
		if HasPermission(userPermissions, r) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks if all of the required permissions are present.
func HasAllPermissions(userPermissions []Permission, required []Permission) bool {
	for _, r := range required {
		if !HasPermission(userPermissions, r) {
			return false
		}
	}
	return true
}

// PermissionsToStrings converts a slice of Permission to a slice of strings.
func PermissionsToStrings(permissions []Permission) []string {
	result := make([]string, len(permissions))
	for i, p := range permissions {
		result[i] = string(p)
	}
	return result
}
