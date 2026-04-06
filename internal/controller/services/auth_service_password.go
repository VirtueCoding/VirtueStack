// Package services provides business logic services for VirtueStack Controller.
//
// This file contains password management operations including password changes and resets.
package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// ChangePassword changes a user's password after verifying the old password.
func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword, userType string) error {
	var currentHash string

	// Get current password hash based on user type
	switch userType {
	case "customer":
		customer, err := s.customerRepo.GetByID(ctx, userID)
		if err != nil {
			return fmt.Errorf("getting customer: %w", err)
		}
		if customer.PasswordHash == nil {
			return sharederrors.NewValidationError("password", "no password set — use account settings to create one")
		}
		currentHash = *customer.PasswordHash
	case "admin":
		admin, err := s.adminRepo.GetByID(ctx, userID)
		if err != nil {
			return fmt.Errorf("getting admin: %w", err)
		}
		currentHash = admin.PasswordHash
	default:
		return fmt.Errorf("invalid user type: %s", userType)
	}

	// Verify old password
	match, err := s.verifyPassword(oldPassword, currentHash)
	if err != nil {
		return fmt.Errorf("verifying old password: %w", err)
	}
	if !match {
		return sharederrors.ErrUnauthorized
	}

	// Hash new password
	newHash, err := s.hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing new password: %w", err)
	}

	// Update password hash in database
	switch userType {
	case "customer":
		if err := s.customerRepo.UpdateCustomerPasswordHash(ctx, userID, newHash); err != nil {
			return fmt.Errorf("updating customer password: %w", err)
		}
	case "admin":
		if err := s.adminRepo.UpdatePasswordHash(ctx, userID, newHash); err != nil {
			return fmt.Errorf("updating admin password: %w", err)
		}
	}

	// Invalidate all sessions for this user (force re-login)
	if err := s.LogoutAll(ctx, userID, userType); err != nil {
		s.logger.Warn("failed to logout all sessions after password change", "user_id", userID, "error", err)
	}

	s.logger.Info("password changed", "user_id", userID, "user_type", userType)
	return nil
}

// RequestPasswordReset generates a password reset token for a user.
// Returns the reset token (caller is responsible for sending email).
func (s *AuthService) RequestPasswordReset(ctx context.Context, email string) (string, error) {
	customer, err := s.customerRepo.GetByEmail(ctx, email)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return "", nil
		}
		return "", fmt.Errorf("getting customer by email: %w", err)
	}

	resetToken, err := crypto.GenerateRandomToken(32)
	if err != nil {
		return "", fmt.Errorf("generating reset token: %w", err)
	}

	tokenHash := crypto.HashSHA256(resetToken)

	reset := &models.PasswordReset{
		UserID:    customer.ID,
		UserType:  "customer",
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(PasswordResetTokenDuration),
	}

	if err := s.customerRepo.CreatePasswordReset(ctx, reset); err != nil {
		return "", fmt.Errorf("creating password reset: %w", err)
	}

	s.logger.Info("password reset requested", "customer_id", customer.ID)

	return resetToken, nil
}

// ResetPassword resets a user's password using a reset token.
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	tokenHash := crypto.HashSHA256(token)

	newHash, err := s.hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing new password: %w", err)
	}

	reset, err := s.customerRepo.ResetPasswordWithToken(ctx, tokenHash, newHash)
	if err != nil {
		if errors.Is(err, repository.ErrNoRowsAffected) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			return sharederrors.ErrUnauthorized
		}
		return fmt.Errorf("resetting password with token: %w", err)
	}

	if err := s.LogoutAll(ctx, reset.UserID, reset.UserType); err != nil {
		s.logger.Warn("failed to logout all sessions after password reset", "user_id", reset.UserID, "error", err)
	}

	if s.auditRepo != nil {
		audit := &models.AuditLog{
			ActorID:      &reset.UserID,
			ActorType:    reset.UserType,
			Action:       "password.reset",
			ResourceType: "user",
			ResourceID:   &reset.UserID,
			Success:      true,
		}
		if err := s.auditRepo.Append(ctx, audit); err != nil {
			s.logger.Warn("failed to write audit log for password reset", "user_id", reset.UserID, "error", err)
		}
	}

	s.logger.Info("password reset completed", "user_id", reset.UserID, "user_type", reset.UserType)
	return nil
}
