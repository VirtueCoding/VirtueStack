// Package services provides business logic services for VirtueStack Controller.
//
// This file contains admin login operations including AdminLogin and AdminVerify2FA.
package services

import (
	"context"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
)

// verifyAdminCredentials fetches the admin by email and verifies the password.
// On wrong password or missing email it records a failed login attempt and returns
// ErrUnauthorized. Returns the admin record on success.
func (s *AuthService) verifyAdminCredentials(ctx context.Context, email, password string) (*models.Admin, error) {
	admin, err := s.adminRepo.GetByEmail(ctx, email)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			s.logger.Warn("admin login attempt for non-existent email", "email", util.MaskEmail(email))
			// Timing attack mitigation: perform a dummy password verification
			// to ensure consistent response time regardless of email existence.
			// The dummy hash uses standard Argon2id parameters.
			_, _ = s.verifyPassword(password, "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$RdescudvJCsgt3ub+b+dWRWJTmaaJObG")
			_ = s.customerRepo.RecordFailedLogin(ctx, email)
			return nil, sharederrors.ErrUnauthorized
		}
		return nil, fmt.Errorf("getting admin by email: %w", err)
	}

	match, err := s.verifyPassword(password, admin.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("verifying password: %w", err)
	}
	if !match {
		s.logger.Warn("invalid admin password attempt", "admin_id", admin.ID)
		_ = s.customerRepo.RecordFailedLogin(ctx, email)
		return nil, sharederrors.ErrUnauthorized
	}
	return admin, nil
}

// AdminLogin authenticates an admin user.
// 2FA is MANDATORY for admin accounts - always returns temp_token with requires_2fa=true.
// Account lockout is enforced: after MaxFailedLoginAttempts failures within LockoutWindow
// the account is locked and ErrAccountLocked is returned.
func (s *AuthService) AdminLogin(ctx context.Context, email, password string) (*models.AuthTokens, error) {
	if err := s.checkLoginLockout(ctx, email); err != nil {
		return nil, err
	}

	admin, err := s.verifyAdminCredentials(ctx, email, password)
	if err != nil {
		return nil, err
	}

	// Clear failed login counter on success.
	_ = s.customerRepo.ClearFailedLogins(ctx, email)

	// 2FA is MANDATORY for admins - always require verification.
	if !admin.TOTPEnabled {
		s.logger.Error("admin account does not have 2FA enabled", "admin_id", admin.ID)
		return nil, fmt.Errorf("admin account must have 2FA enabled")
	}

	tempToken, err := middleware.GenerateTempToken(s.authConfig, admin.ID, "admin")
	if err != nil {
		return nil, fmt.Errorf("generating temp token: %w", err)
	}

	s.logger.Info("admin login requires 2FA", "admin_id", admin.ID)
	return &models.AuthTokens{TokenType: "Bearer", Requires2FA: true, TempToken: tempToken}, nil
}

// enforceAdminSessionLimit evicts the oldest admin session when MaxAdminSessions
// is already reached. Failures are logged and do not block login.
func (s *AuthService) enforceAdminSessionLimit(ctx context.Context, adminID string) {
	count, err := s.customerRepo.CountSessionsByUser(ctx, adminID, "admin")
	if err != nil {
		s.logger.Warn("failed to count admin sessions", "admin_id", adminID, "error", err)
		return
	}
	if count >= MaxAdminSessions {
		if err := s.customerRepo.DeleteOldestSession(ctx, adminID, "admin"); err != nil {
			s.logger.Warn("failed to delete oldest admin session", "admin_id", adminID, "error", err)
		}
	}
}

// AdminVerify2FA verifies a TOTP code and completes the admin 2FA login flow.
func (s *AuthService) AdminVerify2FA(ctx context.Context, tempToken, totpCode, ipAddress, userAgent string) (*models.AuthTokens, string, error) {
	claims, err := middleware.ValidateTempToken(s.authConfig, tempToken)
	if err != nil {
		return nil, "", fmt.Errorf("invalid temp token: %w", err)
	}
	if claims.UserType != "admin" {
		return nil, "", fmt.Errorf("temp token is not for admin")
	}
	admin, err := s.adminRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, "", fmt.Errorf("getting admin: %w", err)
	}
	if !admin.TOTPEnabled {
		return nil, "", fmt.Errorf("2FA not enabled for this admin")
	}
	valid, err := s.validateTOTPCode(admin.TOTPSecretEncrypted, totpCode)
	if err != nil {
		return nil, "", err
	}
	if !valid {
		valid, err = s.consumeBackupCode(ctx, admin.ID, totpCode, admin.TOTPBackupCodesHash, s.adminRepo.UpdateBackupCodes)
		if err != nil {
			return nil, "", err
		}
		if !valid {
			s.logger.Warn("invalid admin TOTP code and no matching backup code", "admin_id", admin.ID)
			return nil, "", sharederrors.ErrUnauthorized
		}
		s.logger.Info("backup code used for authentication", "admin_id", admin.ID)
	}
	s.enforceAdminSessionLimit(ctx, admin.ID)
	tokens, refreshToken, err := s.createLoginSession(ctx, admin.ID, "admin", admin.Role, ipAddress, userAgent, AdminRefreshTokenDuration)
	if err != nil {
		return nil, "", err
	}
	s.logger.Info("admin 2FA verified", "admin_id", admin.ID)
	return tokens, refreshToken, nil
}

// GetAdminByID retrieves an admin by their ID.
// This is a lightweight method used by the /me endpoint for session validation.
func (s *AuthService) GetAdminByID(ctx context.Context, adminID string) (*models.Admin, error) {
	admin, err := s.adminRepo.GetByID(ctx, adminID)
	if err != nil {
		return nil, fmt.Errorf("getting admin by id: %w", err)
	}
	return admin, nil
}

// UpdateAdminPermissions updates the permissions for an admin.
// This method is used by super_admin to manage fine-grained permissions for other admins.
// Pass an empty slice to reset the admin to use role-based default permissions.
func (s *AuthService) UpdateAdminPermissions(ctx context.Context, adminID string, permissions []models.Permission) (*models.Admin, error) {
	// Verify the admin exists and update permissions
	_, err := s.adminRepo.GetByID(ctx, adminID)
	if err != nil {
		return nil, fmt.Errorf("getting admin: %w", err)
	}

	// Update permissions
	if err := s.adminRepo.UpdatePermissions(ctx, adminID, permissions); err != nil {
		return nil, fmt.Errorf("updating permissions: %w", err)
	}

	// Fetch updated admin
	updated, err := s.adminRepo.GetByID(ctx, adminID)
	if err != nil {
		return nil, fmt.Errorf("getting updated admin: %w", err)
	}

	s.logger.Info("updated admin permissions", "admin_id", adminID, "permission_count", len(permissions))
	return updated, nil
}
