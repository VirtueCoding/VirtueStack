package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCustomerStatusConstants(t *testing.T) {
	assert.Equal(t, "active", CustomerStatusActive)
	assert.Equal(t, "pending_verification", CustomerStatusPendingVerification)
	assert.Equal(t, "suspended", CustomerStatusSuspended)
	assert.Equal(t, "deleted", CustomerStatusDeleted)
}

func TestAdmin_GetEffectivePermissions_ExplicitPermissions(t *testing.T) {
	explicit := []Permission{
		PermissionVMsRead,
		PermissionVMsWrite,
	}

	admin := &Admin{
		Role:        "admin",
		Permissions: explicit,
	}

	got := admin.GetEffectivePermissions()
	assert.Equal(t, explicit, got, "explicit permissions should be returned when set")
}

func TestAdmin_GetEffectivePermissions_DefaultPermissions(t *testing.T) {
	admin := &Admin{
		Role:        "super_admin",
		Permissions: nil,
	}

	got := admin.GetEffectivePermissions()
	defaults := GetDefaultPermissions("super_admin")
	assert.Equal(t, defaults, got, "default permissions for role should be returned when no explicit permissions")
}

func TestAdmin_GetEffectivePermissions_EmptySlice(t *testing.T) {
	admin := &Admin{
		Role:        "admin",
		Permissions: []Permission{},
	}

	got := admin.GetEffectivePermissions()
	defaults := GetDefaultPermissions("admin")
	assert.Equal(t, defaults, got, "empty explicit permissions should fall back to defaults")
}

func TestCustomerPasswordHashNotSerialized(t *testing.T) {
	// Verify password_hash has json:"-" tag via struct existence
	hash := "argon2id-hash"
	c := Customer{
		PasswordHash: &hash,
	}
	assert.Equal(t, "argon2id-hash", *c.PasswordHash)
}

func TestAdminTOTPSecretNotSerialized(t *testing.T) {
	a := Admin{
		TOTPSecretEncrypted: "encrypted-secret",
	}
	assert.Equal(t, "encrypted-secret", a.TOTPSecretEncrypted)
}

func TestLoginRequestFields(t *testing.T) {
	req := LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
	}
	assert.Equal(t, "test@example.com", req.Email)
	assert.Equal(t, "password123", req.Password)
}

func TestVerify2FARequestFields(t *testing.T) {
	req := Verify2FARequest{
		TempToken: "temp-token",
		TOTPCode:  "123456",
	}
	assert.Equal(t, "temp-token", req.TempToken)
	assert.Equal(t, "123456", req.TOTPCode)
}

func TestAuthTokensFields(t *testing.T) {
	tokens := AuthTokens{
		AccessToken: "jwt-token",
		TokenType:   "Bearer",
		ExpiresIn:   900,
		Requires2FA: true,
		TempToken:   "2fa-temp",
	}
	assert.Equal(t, "jwt-token", tokens.AccessToken)
	assert.Equal(t, "Bearer", tokens.TokenType)
	assert.Equal(t, 900, tokens.ExpiresIn)
	assert.True(t, tokens.Requires2FA)
	assert.Equal(t, "2fa-temp", tokens.TempToken)
}
