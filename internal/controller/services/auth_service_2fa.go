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
	"github.com/pquerna/otp/totp"
)

// TOTPSetupResult contains the result of initiating 2FA setup.
type TOTPSetupResult struct {
	Secret      string   // Base32 encoded TOTP secret
	QRURL       string   // otpauth:// URL for QR code generation
	BackupCodes []string // Plain text backup codes (only shown once)
}

// ValidateTOTP validates a TOTP code against an encrypted secret (utility method).
// This is useful for backup code verification or re-auth scenarios.
// Delegates to validateTOTPCode to avoid duplication.
func (s *AuthService) ValidateTOTP(totpCode, encryptedSecret string) (bool, error) {
	return s.validateTOTPCode(encryptedSecret, totpCode)
}

// Initiate2FA generates a new TOTP secret and backup codes for a customer.
// Returns the secret, QR URL, and backup codes. The secret is NOT yet enabled.
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
		backupCodesHash[i] = crypto.HashSHA256(code)
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

// GetBackupCodes returns the plain text backup codes for a customer.
// Returns the codes, a flag indicating if they've already been shown, and any error.
// Codes are only returned once - subsequent calls will return alreadyShown=true.
func (s *AuthService) GetBackupCodes(ctx context.Context, customerID string) ([]string, bool, error) {
	customer, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		return nil, false, fmt.Errorf("getting customer: %w", err)
	}

	if !customer.TOTPEnabled {
		return nil, false, fmt.Errorf("2FA is not enabled")
	}

	if customer.TOTPBackupCodesHash == nil || len(customer.TOTPBackupCodesHash) == 0 {
		return nil, false, fmt.Errorf("no backup codes available")
	}

	if customer.TOTPBackupCodesShown {
		return nil, true, nil
	}

	codes := make([]string, 10)
	codesHash := make([]string, 10)
	for i := range codes {
		code, err := crypto.GenerateRandomDigits(8)
		if err != nil {
			return nil, false, fmt.Errorf("generating backup code: %w", err)
		}
		codes[i] = code
		codesHash[i] = crypto.HashSHA256(code)
	}

	if err := s.customerRepo.UpdateBackupCodesWithShown(ctx, customerID, codesHash); err != nil {
		return nil, false, fmt.Errorf("storing backup codes: %w", err)
	}

	s.logger.Info("backup codes retrieved and stored", "customer_id", customerID)

	return codes, false, nil
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
		codesHash[i] = crypto.HashSHA256(code)
	}

	if err := s.customerRepo.UpdateBackupCodesWithShown(ctx, customerID, codesHash); err != nil {
		return nil, fmt.Errorf("regenerating backup codes: %w", err)
	}

	s.logger.Info("backup codes regenerated", "customer_id", customerID)

	return codes, nil
}