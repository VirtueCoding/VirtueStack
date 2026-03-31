package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasPermission(t *testing.T) {
	tests := []struct {
		name     string
		perms    []Permission
		required Permission
		want     bool
	}{
		{
			name:     "has exact permission",
			perms:    []Permission{PermissionVMsRead, PermissionVMsWrite},
			required: PermissionVMsWrite,
			want:     true,
		},
		{
			name:     "does not have permission",
			perms:    []Permission{PermissionVMsRead},
			required: PermissionVMsWrite,
			want:     false,
		},
		{
			name:     "empty permissions list",
			perms:    []Permission{},
			required: PermissionVMsRead,
			want:     false,
		},
		{
			name:     "nil permissions list",
			perms:    nil,
			required: PermissionVMsRead,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasPermission(tt.perms, tt.required))
		})
	}
}

func TestHasAnyPermission(t *testing.T) {
	tests := []struct {
		name     string
		perms    []Permission
		required []Permission
		want     bool
	}{
		{
			name:     "has one of many",
			perms:    []Permission{PermissionVMsRead, PermissionNodesRead},
			required: []Permission{PermissionVMsRead, PermissionPlansWrite},
			want:     true,
		},
		{
			name:     "has none",
			perms:    []Permission{PermissionVMsRead},
			required: []Permission{PermissionPlansWrite, PermissionNodesWrite},
			want:     false,
		},
		{
			name:     "empty required",
			perms:    []Permission{PermissionVMsRead},
			required: []Permission{},
			want:     false,
		},
		{
			name:     "empty user perms",
			perms:    []Permission{},
			required: []Permission{PermissionVMsRead},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasAnyPermission(tt.perms, tt.required))
		})
	}
}

func TestHasAllPermissions(t *testing.T) {
	tests := []struct {
		name     string
		perms    []Permission
		required []Permission
		want     bool
	}{
		{
			name:     "has all required",
			perms:    []Permission{PermissionVMsRead, PermissionVMsWrite, PermissionNodesRead},
			required: []Permission{PermissionVMsRead, PermissionVMsWrite},
			want:     true,
		},
		{
			name:     "missing one",
			perms:    []Permission{PermissionVMsRead},
			required: []Permission{PermissionVMsRead, PermissionVMsWrite},
			want:     false,
		},
		{
			name:     "empty required is satisfied",
			perms:    []Permission{PermissionVMsRead},
			required: []Permission{},
			want:     true,
		},
		{
			name:     "nil required is satisfied",
			perms:    []Permission{PermissionVMsRead},
			required: nil,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasAllPermissions(tt.perms, tt.required))
		})
	}
}

func TestGetDefaultPermissions(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		wantNil  bool
		contains []Permission
	}{
		{
			name:     "super_admin gets all permissions",
			role:     RoleSuperAdmin,
			contains: []Permission{PermissionPlansDelete, PermissionNodesDelete, PermissionCustomersDelete},
		},
		{
			name:     "admin gets read/write but not all deletes",
			role:     RoleAdmin,
			contains: []Permission{PermissionVMsRead, PermissionVMsWrite, PermissionVMsDelete},
		},
		{
			name:     "viewer gets only read permissions",
			role:     RoleViewer,
			contains: []Permission{PermissionVMsRead, PermissionPlansRead},
		},
		{
			name:    "unknown role returns nil",
			role:    "unknown",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perms := GetDefaultPermissions(tt.role)
			if tt.wantNil {
				assert.Nil(t, perms)
				return
			}

			for _, p := range tt.contains {
				assert.True(t, HasPermission(perms, p), "expected %s to be in %s permissions", p, tt.role)
			}
		})
	}
}

func TestGetDefaultPermissions_ViewerLacksWritePerms(t *testing.T) {
	perms := GetDefaultPermissions(RoleViewer)
	writePerms := []Permission{
		PermissionPlansWrite, PermissionPlansDelete,
		PermissionNodesWrite, PermissionNodesDelete,
		PermissionCustomersWrite, PermissionCustomersDelete,
		PermissionVMsWrite, PermissionVMsDelete,
		PermissionSettingsWrite,
	}
	for _, p := range writePerms {
		assert.False(t, HasPermission(perms, p), "viewer should NOT have %s", p)
	}
}

func TestGetAllPermissions(t *testing.T) {
	perms := GetAllPermissions()
	assert.NotEmpty(t, perms)

	// Verify it returns a copy (modifying doesn't affect original)
	original := GetAllPermissions()
	perms[0] = "modified"
	assert.NotEqual(t, "modified", string(original[0]))
}

func TestGetDefaultPermissions_ReturnsCopy(t *testing.T) {
	perms1 := GetDefaultPermissions(RoleSuperAdmin)
	perms2 := GetDefaultPermissions(RoleSuperAdmin)

	perms1[0] = "modified"
	assert.NotEqual(t, string(perms1[0]), string(perms2[0]),
		"GetDefaultPermissions should return a copy")
}

func TestPermissionsToStrings(t *testing.T) {
	perms := []Permission{PermissionVMsRead, PermissionVMsWrite}
	strs := PermissionsToStrings(perms)

	assert.Equal(t, []string{"vms:read", "vms:write"}, strs)
}

func TestPermissionsToStrings_Empty(t *testing.T) {
	strs := PermissionsToStrings([]Permission{})
	assert.Empty(t, strs)
}

func TestBillingPermissions_InRoleDefaults(t *testing.T) {
	tests := []struct {
		name      string
		role      string
		hasRead   bool
		hasWrite  bool
	}{
		{"super_admin has both", RoleSuperAdmin, true, true},
		{"admin has both", RoleAdmin, true, true},
		{"viewer has read only", RoleViewer, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perms := GetDefaultPermissions(tt.role)
			assert.Equal(t, tt.hasRead, HasPermission(perms, PermissionBillingRead),
				"billing:read for %s", tt.role)
			assert.Equal(t, tt.hasWrite, HasPermission(perms, PermissionBillingWrite),
				"billing:write for %s", tt.role)
		})
	}
}

func TestGetAllPermissions_IncludesBilling(t *testing.T) {
	perms := GetAllPermissions()
	assert.True(t, HasPermission(perms, PermissionBillingRead), "allPermissions should include billing:read")
	assert.True(t, HasPermission(perms, PermissionBillingWrite), "allPermissions should include billing:write")
}

func TestAdminGetEffectivePermissions(t *testing.T) {
	t.Run("explicit permissions override defaults", func(t *testing.T) {
		admin := &Admin{
			Role:        RoleViewer,
			Permissions: []Permission{PermissionVMsRead, PermissionVMsWrite},
		}
		perms := admin.GetEffectivePermissions()
		assert.Len(t, perms, 2)
		assert.True(t, HasPermission(perms, PermissionVMsWrite))
	})

	t.Run("nil permissions fall back to role defaults", func(t *testing.T) {
		admin := &Admin{
			Role:        RoleSuperAdmin,
			Permissions: nil,
		}
		perms := admin.GetEffectivePermissions()
		assert.NotEmpty(t, perms)
		// Super admin should have all permissions
		assert.True(t, HasPermission(perms, PermissionPlansDelete))
	})

	t.Run("empty permissions fall back to role defaults", func(t *testing.T) {
		admin := &Admin{
			Role:        RoleAdmin,
			Permissions: []Permission{},
		}
		perms := admin.GetEffectivePermissions()
		assert.NotEmpty(t, perms)
	})
}
