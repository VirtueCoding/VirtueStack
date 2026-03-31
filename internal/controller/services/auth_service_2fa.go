// Package services provides business logic services for VirtueStack Controller.
//
// This file contains 2FA setup and management operations.
package services

import (
	"context"
	"fmt"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/alexedwards/argon2id"
	"github.com/pquerna/otp/totp"
)

// TOTPSetupResult contains the result of initiating 2FA setup.
type TOTPSetupResult struct {
	Secret      string   // Base32 encoded TOTP secret
	QRURL       string   // otpauth:// URL for QR code generation
	BackupCodes []string // Plain text backup codes (only shown once)
}

// hashBackupCode hashes a backup code using Argon2id with the same parameters
// used for passwords. This replaces the previous SHA-256 hashing (F-011).
func hashBackupCode(code string) (string, error) {
	return argon2id.CreateHash(code, Argon2idParams)
}

// checkBackupCode verifies a backup code against an Argon2id hash.
func checkBackupCode(code, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(code, hash)
}

// ValidateTOTP validates a TOTP code against an encrypted secret (utility method).
// This is useful for backup code verification or re-auth scenarios.
// Delegates to validateTOTPCode to avoid duplication.
func (s *AuthService) ValidateTOTP(totpCode, encryptedSecret string) (bool, error) {
	return s.validateTOTPCode(encryptedSecret, totpCode)
}

// TOTPSetupTTL is the maximum age of an unconfirmed (totp_enabled=false) TOTP
// secret before it should be purged by the periodic cleanup job (F-159).
const TOTPSetupTTL = 24 * time.Hour

// Initiate2FA generates a new TOTP secret and backup codes for a customer.
// Returns the secret, QR URL, and backup codes. The secret is NOT yet enabled.
//
// NOTE (F-159): The TOTP secret is persisted with totp_enabled=false before the
// user completes setup. A background goroutine should periodically purge unconfirmed
// TOTP secrets older than TOTPSetupTTL to limit database accumulation of abandoned
// setup attempts.
func (s *AuthService) Initiate2FA(ctx context.Context, customerID, email string) (*TOTPSetupResult, error) {
	customer, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("getting customer: %w", err)
	}

	if customer.TOTPEnabled {
		return nil, fmt.Errorf("2FA is already enabled")
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "VirtueStack",
		AccountName: email,
		SecretSize:  20,
	})
	if err != nil {
		return nil, fmt.Errorf("generating TOTP key: %w", err)
	}

	backupCodes := make([]string, 10)
	backupCodesHash := make([]string, 10)
	for i := range backupCodes {
		code, err := crypto.GenerateRandomDigits(8)
		if err != nil {
			return nil, fmt.Errorf("generating backup code: %w", err)
		}
		backupCodes[i] = code
		h, err := hashBackupCode(code)
		if err != nil {
			return nil, fmt.Errorf("hashing backup code: %w", err)
		}
		backupCodesHash[i] = h
	}

	encryptedSecret, err := crypto.Encrypt(key.Secret(), s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("encrypting TOTP secret: %w", err)
	}

	if err := s.customerRepo.UpdateTOTPEnabled(ctx, customerID, false, &encryptedSecret, backupCodesHash); err != nil {
		return nil, fmt.Errorf("storing TOTP secret: %w", err)
	}

	s.logger.Info("2FA setup initiated", "customer_id", customerID)

	return &TOTPSetupResult{
		Secret:      key.Secret(),
		QRURL:       key.URL(),
		BackupCodes: backupCodes,
	}, nil
}

// Enable2FA enables 2FA for a customer after verifying the TOTP code.
// The customer must have previously called Initiate2FA.
func (s *AuthService) Enable2FA(ctx context.Context, customerID, totpCode string) error {
	customer, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		return fmt.Errorf("getting customer: %w", err)
	}

	if customer.TOTPEnabled {
		return fmt.Errorf("2FA is already enabled")
	}

	if customer.TOTPSecretEncrypted == nil {
		return fmt.Errorf("2FA setup not initiated")
	}

	valid, err := s.validateTOTPCode(*customer.TOTPSecretEncrypted, totpCode)
	if err != nil {
		return err
	}
	if !valid {
		return sharederrors.ErrUnauthorized
	}

	if err := s.customerRepo.UpdateTOTPEnabled(ctx, customerID, true, customer.TOTPSecretEncrypted, customer.TOTPBackupCodesHash); err != nil {
		return fmt.Errorf("enabling 2FA: %w", err)
	}

	if err := s.LogoutAll(ctx, customerID, "customer"); err != nil {
		s.logger.Warn("failed to logout all sessions after 2FA enable", "customer_id", customerID, "error", err)
	}

	s.logger.Info("2FA enabled", "customer_id", customerID)

	return nil
}

// Disable2FA disables 2FA for a customer after verifying their password.
func (s *AuthService) Disable2FA(ctx context.Context, customerID, password string) error {
	customer, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		return fmt.Errorf("getting customer: %w", err)
	}

	if !customer.TOTPEnabled {
		return fmt.Errorf("2FA is not enabled")
	}

	match, err := s.verifyPassword(password, customer.PasswordHash)
	if err != nil {
		return fmt.Errorf("verifying password: %w", err)
	}
	if !match {
		return sharederrors.ErrUnauthorized
	}

	if err := s.customerRepo.UpdateTOTPEnabled(ctx, customerID, false, nil, nil); err != nil {
		return fmt.Errorf("disabling 2FA: %w", err)
	}

	s.logger.Info("2FA disabled", "customer_id", customerID)

	return nil
}

// Get2FAStatus returns the 2FA status for a customer.
func (s *AuthService) Get2FAStatus(ctx context.Context, customerID string) (enabled bool, createdAt *time.Time, err error) {
	customer, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		return false, nil, fmt.Errorf("getting customer: %w", err)
	}

	return customer.TOTPEnabled, &customer.UpdatedAt, nil
}

// GetBackupCodes returns whether backup codes exist and have already been shown.
// Plain-text codes are only available at Initiate2FA time. This method only
// indicates whether codes exist and whether they have already been viewed.
// Call RegenerateBackupCodes to obtain a new set of plaintext codes.
func (s *AuthService) GetBackupCodes(ctx context.Context, customerID string) ([]string, bool, error) {
	customer, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		return nil, false, fmt.Errorf("getting customer: %w", err)
	}

	if !customer.TOTPEnabled {
		return nil, false, fmt.Errorf("2FA is not enabled")
	}

	if len(customer.TOTPBackupCodesHash) == 0 {
		return nil, false, fmt.Errorf("no backup codes available")
	}

	// Backup codes are hashed and cannot be reversed. Always report as already
	// shown so that callers know to call RegenerateBackupCodes for new codes.
	// The only time plaintext codes are available is during Initiate2FA.
	s.logger.Info("backup codes status retrieved", "customer_id", customerID)
	return nil, true, nil
}

// RegenerateBackupCodes generates new backup codes for a customer.
func (s *AuthService) RegenerateBackupCodes(ctx context.Context, customerID string) ([]string, error) {
	customer, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("getting customer: %w", err)
	}

	if !customer.TOTPEnabled {
		return nil, fmt.Errorf("2FA is not enabled")
	}

	codes := make([]string, 10)
	codesHash := make([]string, 10)
	for i := range codes {
		code, err := crypto.GenerateRandomDigits(8)
		if err != nil {
			return nil, fmt.Errorf("generating backup code: %w", err)
		}
		codes[i] = code
		h, err := hashBackupCode(code)
		if err != nil {
			return nil, fmt.Errorf("hashing backup code: %w", err)
		}
		codesHash[i] = h
	}

	if err := s.customerRepo.UpdateBackupCodesWithShown(ctx, customerID, codesHash); err != nil {
		return nil, fmt.Errorf("regenerating backup codes: %w", err)
	}

	s.logger.Info("backup codes regenerated", "customer_id", customerID)

	return codes, nil
}