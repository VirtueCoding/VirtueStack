// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/alexedwards/argon2id"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
)

const (
	// AccessTokenDuration is the lifetime of JWT access tokens.
	AccessTokenDuration = 15 * time.Minute

	// CustomerRefreshTokenDuration is the lifetime of customer refresh tokens.
	CustomerRefreshTokenDuration = 7 * 24 * time.Hour // 7 days

	// AdminRefreshTokenDuration is the lifetime of admin refresh tokens.
	AdminRefreshTokenDuration = 4 * time.Hour

	// TempTokenDuration is the lifetime of temporary 2FA tokens.
	TempTokenDuration = 5 * time.Minute

	// PasswordResetTokenDuration is the lifetime of password reset tokens.
	PasswordResetTokenDuration = 1 * time.Hour

	// MaxAdminSessions is the maximum concurrent sessions for admin users.
	MaxAdminSessions = 3
)

// Argon2idParams holds the parameters for Argon2id password hashing.
// These parameters are tuned for security while maintaining reasonable performance.
var Argon2idParams = &argon2id.Params{
	Memory:      64 * 1024, // 64MB
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

// AuthService provides authentication business logic for VirtueStack.
// It handles login flows, 2FA verification, token refresh, and session management.
type AuthService struct {
	customerRepo  *repository.CustomerRepository
	adminRepo     *repository.AdminRepository
	authConfig    middleware.AuthConfig
	encryptionKey string // hex-encoded AES-256 key for TOTP secret decryption
	logger        *slog.Logger
}

// NewAuthService creates a new AuthService with the given dependencies.
func NewAuthService(
	customerRepo *repository.CustomerRepository,
	adminRepo *repository.AdminRepository,
	jwtSecret string,
	issuer string,
	encryptionKey string,
	logger *slog.Logger,
) *AuthService {
	return &AuthService{
		customerRepo:  customerRepo,
		adminRepo:     adminRepo,
		authConfig:    middleware.AuthConfig{JWTSecret: jwtSecret, Issuer: issuer},
		encryptionKey: encryptionKey,
		logger:        logger,
	}
}

// Login authenticates a customer and returns tokens or a 2FA challenge.
// If the customer has 2FA enabled, returns temp_token with requires_2fa=true.
// Otherwise, returns access_token and refresh_token directly.
func (s *AuthService) Login(ctx context.Context, email, password, ipAddress, userAgent string) (*models.AuthTokens, string, error) {
	customer, err := s.customerRepo.GetByEmail(ctx, email)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			s.logger.Warn("login attempt for non-existent email", "email", email)
			return nil, "", sharederrors.ErrUnauthorized
		}
		return nil, "", fmt.Errorf("getting customer by email: %w", err)
	}

	// Verify password using Argon2id
	match, err := s.verifyPassword(password, customer.PasswordHash)
	if err != nil {
		return nil, "", fmt.Errorf("verifying password: %w", err)
	}
	if !match {
		s.logger.Warn("invalid password attempt", "customer_id", customer.ID)
		return nil, "", sharederrors.ErrUnauthorized
	}

	// Check if customer account is active
	if customer.Status != models.CustomerStatusActive {
		return nil, "", fmt.Errorf("account is %s", customer.Status)
	}

	// If 2FA is enabled, return temp token for 2FA verification
	if customer.TOTPEnabled && customer.TOTPSecretEncrypted != nil {
		tempToken, err := middleware.GenerateTempToken(s.authConfig, customer.ID, "customer")
		if err != nil {
			return nil, "", fmt.Errorf("generating temp token: %w", err)
		}

		s.logger.Info("login requires 2FA", "customer_id", customer.ID)

		return &models.AuthTokens{
			TokenType:   "Bearer",
			Requires2FA: true,
			TempToken:   tempToken,
		}, "", nil
	}

	// No 2FA - generate tokens and create session
	accessToken, err := middleware.GenerateAccessToken(s.authConfig, customer.ID, "customer", "", AccessTokenDuration)
	if err != nil {
		return nil, "", fmt.Errorf("generating access token: %w", err)
	}

	refreshToken, err := middleware.GenerateRefreshToken()
	if err != nil {
		return nil, "", fmt.Errorf("generating refresh token: %w", err)
	}

	// Create session in database
	refreshTokenHash := crypto.HashSHA256(refreshToken)
	session := &models.Session{
		ID:               uuid.New().String(),
		UserID:           customer.ID,
		UserType:         "customer",
		RefreshTokenHash: refreshTokenHash,
		IPAddress:        &ipAddress,
		UserAgent:        &userAgent,
		ExpiresAt:        time.Now().Add(CustomerRefreshTokenDuration),
	}

	if err := s.customerRepo.CreateSession(ctx, session); err != nil {
		return nil, "", fmt.Errorf("creating session: %w", err)
	}

	s.logger.Info("customer logged in", "customer_id", customer.ID, "session_id", session.ID)

	return &models.AuthTokens{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(AccessTokenDuration.Seconds()),
	}, refreshToken, nil
}

// Verify2FA verifies a TOTP code and completes the 2FA login flow.
// The tempToken is the token returned from Login when 2FA is required.
func (s *AuthService) Verify2FA(ctx context.Context, tempToken, totpCode, ipAddress, userAgent string) (*models.AuthTokens, string, error) {
	// Validate the temp token
	claims, err := middleware.ValidateTempToken(s.authConfig, tempToken)
	if err != nil {
		return nil, "", fmt.Errorf("invalid temp token: %w", err)
	}

	if claims.UserType != "customer" {
		return nil, "", fmt.Errorf("temp token is not for customer")
	}

	// Get the customer
	customer, err := s.customerRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, "", fmt.Errorf("getting customer: %w", err)
	}

	// Verify TOTP code
	if !customer.TOTPEnabled || customer.TOTPSecretEncrypted == nil {
		return nil, "", fmt.Errorf("2FA not enabled for this customer")
	}

	// Decrypt TOTP secret
	totpSecret, err := crypto.Decrypt(*customer.TOTPSecretEncrypted, s.encryptionKey)
	if err != nil {
		return nil, "", fmt.Errorf("decrypting TOTP secret: %w", err)
	}

	// Verify the TOTP code
	valid, err := totp.ValidateCustom(totpCode, totpSecret, time.Now(), totp.ValidateOpts{
		Skew:      1, // Allow 1 step tolerance (±30 seconds)
		Digits:    6,
		Algorithm: 1, // SHA1
	})
	if err != nil {
		return nil, "", fmt.Errorf("validating TOTP: %w", err)
	}
	if !valid {
		s.logger.Warn("invalid TOTP code", "customer_id", customer.ID)
		return nil, "", sharederrors.ErrUnauthorized
	}

	// Check for backup code usage if TOTP fails (not implemented in this iteration)
	// For now, just generate tokens

	// Generate access and refresh tokens
	accessToken, err := middleware.GenerateAccessToken(s.authConfig, customer.ID, "customer", "", AccessTokenDuration)
	if err != nil {
		return nil, "", fmt.Errorf("generating access token: %w", err)
	}

	refreshToken, err := middleware.GenerateRefreshToken()
	if err != nil {
		return nil, "", fmt.Errorf("generating refresh token: %w", err)
	}

	// Create session
	refreshTokenHash := crypto.HashSHA256(refreshToken)
	session := &models.Session{
		ID:               uuid.New().String(),
		UserID:           customer.ID,
		UserType:         "customer",
		RefreshTokenHash: refreshTokenHash,
		IPAddress:        &ipAddress,
		UserAgent:        &userAgent,
		ExpiresAt:        time.Now().Add(CustomerRefreshTokenDuration),
	}

	if err := s.customerRepo.CreateSession(ctx, session); err != nil {
		return nil, "", fmt.Errorf("creating session: %w", err)
	}

	s.logger.Info("customer 2FA verified", "customer_id", customer.ID, "session_id", session.ID)

	return &models.AuthTokens{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(AccessTokenDuration.Seconds()),
	}, refreshToken, nil
}

// RefreshToken validates a refresh token and returns new tokens.
// Implements refresh token rotation - a new refresh token is issued each time.
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken, ipAddress, userAgent string) (*models.AuthTokens, string, error) {
	// Hash the refresh token and look up the session
	refreshTokenHash := crypto.HashSHA256(refreshToken)
	session, err := s.customerRepo.GetSessionByRefreshToken(ctx, refreshTokenHash)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, "", sharederrors.ErrUnauthorized
		}
		return nil, "", fmt.Errorf("getting session: %w", err)
	}

	// Check if session has expired
	if time.Now().After(session.ExpiresAt) {
		// Clean up expired session
		_ = s.customerRepo.DeleteSession(ctx, session.ID)
		return nil, "", sharederrors.ErrUnauthorized
	}

	// Determine refresh token duration and get role based on user type
	refreshDuration := CustomerRefreshTokenDuration
	var role string

	if session.UserType == "admin" {
		refreshDuration = AdminRefreshTokenDuration
		// Fetch admin to get their role
		admin, err := s.adminRepo.GetByID(ctx, session.UserID)
		if err != nil {
			return nil, "", fmt.Errorf("getting admin for refresh: %w", err)
		}
		role = admin.Role
	}

	// Generate new access token
	accessToken, err := middleware.GenerateAccessToken(s.authConfig, session.UserID, session.UserType, role, AccessTokenDuration)
	if err != nil {
		return nil, "", fmt.Errorf("generating access token: %w", err)
	}

	// Generate new refresh token (rotation)
	newRefreshToken, err := middleware.GenerateRefreshToken()
	if err != nil {
		return nil, "", fmt.Errorf("generating refresh token: %w", err)
	}

	// Delete old session
	if err := s.customerRepo.DeleteSession(ctx, session.ID); err != nil {
		s.logger.Warn("failed to delete old session", "session_id", session.ID, "error", err)
	}

	// Create new session with new refresh token
	newRefreshTokenHash := crypto.HashSHA256(newRefreshToken)
	newSession := &models.Session{
		ID:               uuid.New().String(),
		UserID:           session.UserID,
		UserType:         session.UserType,
		RefreshTokenHash: newRefreshTokenHash,
		IPAddress:        &ipAddress,
		UserAgent:        &userAgent,
		ExpiresAt:        time.Now().Add(refreshDuration),
	}

	if err := s.customerRepo.CreateSession(ctx, newSession); err != nil {
		return nil, "", fmt.Errorf("creating new session: %w", err)
	}

	s.logger.Info("token refreshed", "user_id", session.UserID, "new_session_id", newSession.ID)

	return &models.AuthTokens{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(AccessTokenDuration.Seconds()),
	}, newRefreshToken, nil
}

// Logout invalidates a single session.
func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	if err := s.customerRepo.DeleteSession(ctx, sessionID); err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil // Session already gone, not an error
		}
		return fmt.Errorf("deleting session: %w", err)
	}

	s.logger.Info("session logged out", "session_id", sessionID)
	return nil
}

// LogoutAll invalidates all sessions for a user.
func (s *AuthService) LogoutAll(ctx context.Context, userID, userType string) error {
	if err := s.customerRepo.DeleteSessionsByUser(ctx, userID, userType); err != nil {
		return fmt.Errorf("deleting all sessions: %w", err)
	}

	s.logger.Info("all sessions logged out", "user_id", userID, "user_type", userType)
	return nil
}

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
		currentHash = customer.PasswordHash
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
		// Note: CustomerRepository needs UpdatePasswordHash method
		// For now, return an error indicating this needs to be implemented
		return fmt.Errorf("customer password update not yet implemented in repository")
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
			// Don't reveal whether email exists - return success anyway
			return "", nil
		}
		return "", fmt.Errorf("getting customer by email: %w", err)
	}

	// Generate a password reset token (using temp token mechanism with different purpose)
	// For simplicity, we use a random token stored in a password_resets table
	// In a production system, you'd have a dedicated table for this
	resetToken, err := middleware.GenerateRefreshToken() // 64-char hex token
	if err != nil {
		return "", fmt.Errorf("generating reset token: %w", err)
	}

	// In a full implementation, you would store this token with an expiry
	// For now, we'll log it and return it
	// TODO: Implement password_resets table and storage

	s.logger.Info("password reset requested", "customer_id", customer.ID)

	// Return the token - the caller should send it via email
	return resetToken, nil
}

// ResetPassword resets a user's password using a reset token.
func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	// In a full implementation, you would:
	// 1. Look up the token in password_resets table
	// 2. Verify it hasn't expired
	// 3. Get the associated user ID
	// 4. Hash and update the password
	// 5. Delete the reset token
	// 6. Invalidate all sessions

	// For now, return an error indicating this needs to be implemented
	return fmt.Errorf("password reset not yet fully implemented - requires password_resets table")
}

// AdminLogin authenticates an admin user.
// 2FA is MANDATORY for admin accounts - always returns temp_token with requires_2fa=true.
func (s *AuthService) AdminLogin(ctx context.Context, email, password string) (*models.AuthTokens, error) {
	admin, err := s.adminRepo.GetByEmail(ctx, email)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			s.logger.Warn("admin login attempt for non-existent email", "email", email)
			return nil, sharederrors.ErrUnauthorized
		}
		return nil, fmt.Errorf("getting admin by email: %w", err)
	}

	// Verify password using Argon2id
	match, err := s.verifyPassword(password, admin.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("verifying password: %w", err)
	}
	if !match {
		s.logger.Warn("invalid admin password attempt", "admin_id", admin.ID)
		return nil, sharederrors.ErrUnauthorized
	}

	// 2FA is MANDATORY for admins - always require verification
	if !admin.TOTPEnabled {
		s.logger.Error("admin account does not have 2FA enabled", "admin_id", admin.ID)
		return nil, fmt.Errorf("admin account must have 2FA enabled")
	}

	// Generate temp token for 2FA verification
	tempToken, err := middleware.GenerateTempToken(s.authConfig, admin.ID, "admin")
	if err != nil {
		return nil, fmt.Errorf("generating temp token: %w", err)
	}

	s.logger.Info("admin login requires 2FA", "admin_id", admin.ID)

	return &models.AuthTokens{
		TokenType:   "Bearer",
		Requires2FA: true,
		TempToken:   tempToken,
	}, nil
}

// AdminVerify2FA verifies a TOTP code and completes the admin 2FA login flow.
func (s *AuthService) AdminVerify2FA(ctx context.Context, tempToken, totpCode, ipAddress, userAgent string) (*models.AuthTokens, string, error) {
	// Validate the temp token
	claims, err := middleware.ValidateTempToken(s.authConfig, tempToken)
	if err != nil {
		return nil, "", fmt.Errorf("invalid temp token: %w", err)
	}

	if claims.UserType != "admin" {
		return nil, "", fmt.Errorf("temp token is not for admin")
	}

	// Get the admin
	admin, err := s.adminRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, "", fmt.Errorf("getting admin: %w", err)
	}

	// Verify TOTP code
	if !admin.TOTPEnabled {
		return nil, "", fmt.Errorf("2FA not enabled for this admin")
	}

	// Decrypt TOTP secret
	totpSecret, err := crypto.Decrypt(admin.TOTPSecretEncrypted, s.encryptionKey)
	if err != nil {
		return nil, "", fmt.Errorf("decrypting TOTP secret: %w", err)
	}

	// Verify the TOTP code
	valid, err := totp.ValidateCustom(totpCode, totpSecret, time.Now(), totp.ValidateOpts{
		Skew:      1, // Allow 1 step tolerance (±30 seconds)
		Digits:    6,
		Algorithm: 1, // SHA1
	})
	if err != nil {
		return nil, "", fmt.Errorf("validating TOTP: %w", err)
	}
	if !valid {
		s.logger.Warn("invalid admin TOTP code", "admin_id", admin.ID)
		return nil, "", sharederrors.ErrUnauthorized
	}

	// Check session limit for admin
	// In a full implementation, you'd count active sessions and enforce MaxAdminSessions
	// For now, we proceed with session creation

	// Generate access and refresh tokens
	accessToken, err := middleware.GenerateAccessToken(s.authConfig, admin.ID, "admin", admin.Role, AccessTokenDuration)
	if err != nil {
		return nil, "", fmt.Errorf("generating access token: %w", err)
	}

	refreshToken, err := middleware.GenerateRefreshToken()
	if err != nil {
		return nil, "", fmt.Errorf("generating refresh token: %w", err)
	}

	// Create session
	refreshTokenHash := crypto.HashSHA256(refreshToken)
	session := &models.Session{
		ID:               uuid.New().String(),
		UserID:           admin.ID,
		UserType:         "admin",
		RefreshTokenHash: refreshTokenHash,
		IPAddress:        &ipAddress,
		UserAgent:        &userAgent,
		ExpiresAt:        time.Now().Add(AdminRefreshTokenDuration),
	}

	if err := s.customerRepo.CreateSession(ctx, session); err != nil {
		return nil, "", fmt.Errorf("creating session: %w", err)
	}

	s.logger.Info("admin 2FA verified", "admin_id", admin.ID, "session_id", session.ID)

	return &models.AuthTokens{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(AccessTokenDuration.Seconds()),
	}, refreshToken, nil
}

// hashPassword hashes a password using Argon2id.
func (s *AuthService) hashPassword(password string) (string, error) {
	hash, err := argon2id.CreateHash(password, Argon2idParams)
	if err != nil {
		return "", fmt.Errorf("creating password hash: %w", err)
	}
	return hash, nil
}

// verifyPassword verifies a password against an Argon2id hash.
func (s *AuthService) verifyPassword(password, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, fmt.Errorf("comparing password: %w", err)
	}
	return match, nil
}

// ValidateTOTP validates a TOTP code against a secret (utility method).
// This is useful for backup code verification or re-auth scenarios.
func (s *AuthService) ValidateTOTP(totpCode, encryptedSecret string) (bool, error) {
	secret, err := crypto.Decrypt(encryptedSecret, s.encryptionKey)
	if err != nil {
		return false, fmt.Errorf("decrypting TOTP secret: %w", err)
	}

	valid, err := totp.ValidateCustom(totpCode, secret, time.Now(), totp.ValidateOpts{
		Skew:      1,
		Digits:    6,
		Algorithm: 1, // SHA1
	})
	if err != nil {
		return false, fmt.Errorf("validating TOTP: %w", err)
	}

	return valid, nil
}

// ConstantTimeCompare performs a constant-time comparison of two strings.
// This is used to prevent timing attacks when comparing tokens.
func ConstantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}