// Package services provides business logic services for VirtueStack Controller.
//
// This file contains customer login operations including Login and 2FA verification.
package services

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// verifyLoginCredentials fetches the customer by email and verifies the password.
// On wrong password it records the failure and returns ErrUnauthorized.
// Timing attack mitigation: always performs password verification even for non-existent emails.
func (s *AuthService) verifyLoginCredentials(ctx context.Context, email, password string) (*models.Customer, error) {
	customer, err := s.customerRepo.GetByEmail(ctx, email)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			s.logger.Warn("login attempt for non-existent email", "email", util.MaskEmail(email))
			// Timing attack mitigation: perform a dummy password verification
			// to ensure consistent response time regardless of email existence.
			// The dummy hash uses standard Argon2id parameters.
			// Use the pre-computed dummy hash with a random salt to prevent
			// timing attacks (F-149). dummyArgon2idHash is initialised in auth_service.go.
			_, _ = s.verifyPassword(password, dummyArgon2idHash)
			return nil, sharederrors.ErrUnauthorized
		}
		return nil, fmt.Errorf("getting customer by email: %w", err)
	}

	// If customer.PasswordHash is nil, the customer is OAuth-only and
	// cannot log in with a password.
	if customer.PasswordHash == nil {
		s.logger.Warn("password login attempt on OAuth-only account", "customer_id", customer.ID, "email", util.MaskEmail(email))
		_ = s.customerRepo.RecordFailedLogin(ctx, email)
		return nil, sharederrors.ErrUnauthorized
	}

	match, err := s.verifyPassword(password, *customer.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("verifying password: %w", err)
	}
	if !match {
		s.logger.Warn("invalid password attempt", "customer_id", customer.ID, "email", util.MaskEmail(email))
		// Audit logging error intentionally ignored - login failure already returned to user
		_ = s.customerRepo.RecordFailedLogin(ctx, email)
		return nil, sharederrors.ErrUnauthorized
	}
	return customer, nil
}

// build2FAChallenge generates a short-lived temp token for the 2FA verification step.
func (s *AuthService) build2FAChallenge(userID, userType string) (*models.AuthTokens, error) {
	tempToken, err := middleware.GenerateTempToken(s.authConfig, userID, userType)
	if err != nil {
		return nil, fmt.Errorf("generating temp token: %w", err)
	}
	return &models.AuthTokens{TokenType: "Bearer", Requires2FA: true, TempToken: tempToken}, nil
}

// Login authenticates a customer and returns tokens or a 2FA challenge.
// If the customer has 2FA enabled, returns temp_token with requires_2fa=true.
// Otherwise, returns access_token and refresh_token directly.
func (s *AuthService) Login(ctx context.Context, email, password, ipAddress, userAgent string) (*models.AuthTokens, string, error) {
	if err := s.checkLoginLockout(ctx, email); err != nil {
		return nil, "", err
	}

	customer, err := s.verifyLoginCredentials(ctx, email, password)
	if err != nil {
		return nil, "", err
	}

	// Audit logging error intentionally ignored - not critical for login success
	_ = s.customerRepo.ClearFailedLogins(ctx, email)

	if customer.Status != models.CustomerStatusActive {
		return nil, "", fmt.Errorf("account is %s", customer.Status)
	}

	if customer.TOTPEnabled && customer.TOTPSecretEncrypted != nil {
		tokens, err := s.build2FAChallenge(customer.ID, "customer")
		if err != nil {
			return nil, "", err
		}
		s.logger.Info("login requires 2FA", "customer_id", customer.ID)
		return tokens, "", nil
	}

	s.enforceCustomerSessionLimit(ctx, customer.ID)
	tokens, refreshToken, err := s.createLoginSession(ctx, customer.ID, "customer", "", ipAddress, userAgent, CustomerRefreshTokenDuration)
	if err != nil {
		return nil, "", err
	}
	s.logger.Info("customer logged in", "customer_id", customer.ID)
	return tokens, refreshToken, nil
}

// validateTOTPCode decrypts the stored TOTP secret and validates the code.
// Returns true when the code is cryptographically valid.
// Allows a 1-step clock skew tolerance (±30 seconds) for customer accounts.
func (s *AuthService) validateTOTPCode(encryptedSecret, totpCode string) (bool, error) {
	totpSecret, err := crypto.Decrypt(encryptedSecret, s.encryptionKey)
	if err != nil {
		return false, fmt.Errorf("decrypting TOTP secret: %w", err)
	}
	valid, err := totp.ValidateCustom(totpCode, totpSecret, time.Now(), totp.ValidateOpts{
		Skew:      1, // Allow 1 step tolerance (±30 seconds)
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return false, fmt.Errorf("validating TOTP: %w", err)
	}
	return valid, nil
}

// validateTOTPCodeStrict decrypts the stored TOTP secret and validates the code
// with Skew=0 (no clock tolerance). Used for admin accounts where strict security
// is preferred (F-107).
func (s *AuthService) validateTOTPCodeStrict(encryptedSecret, totpCode string) (bool, error) {
	totpSecret, err := crypto.Decrypt(encryptedSecret, s.encryptionKey)
	if err != nil {
		return false, fmt.Errorf("decrypting TOTP secret: %w", err)
	}
	valid, err := totp.ValidateCustom(totpCode, totpSecret, time.Now(), totp.ValidateOpts{
		Skew:      0, // No clock skew tolerance for admin accounts
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return false, fmt.Errorf("validating TOTP: %w", err)
	}
	return valid, nil
}

// consumeBackupCode checks whether totpCode matches any stored backup code hash.
// On match it removes the code from the slice and calls updateFn to persist the
// change. Returns true if a backup code was consumed.
// Uses Argon2id comparison (via checkBackupCode) since backup codes are now
// hashed with Argon2id instead of SHA-256 (F-011).
func (s *AuthService) consumeBackupCode(ctx context.Context, userID, totpCode string, backupCodesHash []string, updateFn func(context.Context, string, []string) error) (bool, error) {
	if len(backupCodesHash) == 0 {
		return false, nil
	}
	for i, codeHash := range backupCodesHash {
		match, err := checkBackupCode(totpCode, codeHash)
		if err != nil || !match {
			continue
		}
		remaining := slices.Concat(backupCodesHash[:i], backupCodesHash[i+1:])
		if err := updateFn(ctx, userID, remaining); err != nil {
			s.logger.Warn("failed to update backup codes after use", "user_id", userID, "error", err)
		}
		return true, nil
	}
	return false, nil
}

// Verify2FA verifies a TOTP code and completes the 2FA login flow.
// The tempToken is the token returned from Login when 2FA is required.
func (s *AuthService) Verify2FA(ctx context.Context, tempToken, totpCode, ipAddress, userAgent string) (*models.AuthTokens, string, error) {
	claims, err := middleware.ValidateTempToken(s.authConfig, tempToken)
	if err != nil {
		return nil, "", fmt.Errorf("invalid temp token: %w", err)
	}
	if claims.UserType != "customer" {
		return nil, "", fmt.Errorf("temp token is not for customer")
	}

	customer, err := s.customerRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, "", fmt.Errorf("getting customer: %w", err)
	}
	if !customer.TOTPEnabled || customer.TOTPSecretEncrypted == nil {
		return nil, "", fmt.Errorf("2FA not enabled for this customer")
	}

	valid, err := s.validateTOTPCode(*customer.TOTPSecretEncrypted, totpCode)
	if err != nil {
		return nil, "", err
	}
	if !valid {
		valid, err = s.consumeBackupCode(ctx, customer.ID, totpCode, customer.TOTPBackupCodesHash, s.customerRepo.UpdateBackupCodes)
		if err != nil {
			return nil, "", err
		}
		if !valid {
			s.logger.Warn("invalid TOTP code and no matching backup code", "customer_id", customer.ID)
			return nil, "", sharederrors.ErrUnauthorized
		}
		s.logger.Info("backup code used for authentication", "customer_id", customer.ID)
	}

	s.enforceCustomerSessionLimit(ctx, customer.ID)
	tokens, refreshToken, err := s.createLoginSession(ctx, customer.ID, "customer", "", ipAddress, userAgent, CustomerRefreshTokenDuration)
	if err != nil {
		return nil, "", err
	}
	s.logger.Info("customer 2FA verified", "customer_id", customer.ID)
	return tokens, refreshToken, nil
}