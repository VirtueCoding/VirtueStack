// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
	"github.com/alexedwards/argon2id"
	"github.com/google/uuid"
	"github.com/pquerna/otp"
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
	PasswordResetTokenDuration = 24 * time.Hour

	// MaxAdminSessions is the maximum concurrent sessions for admin users.
	MaxAdminSessions = 3

	// MaxFailedLoginAttempts is the maximum failed login attempts before account lockout.
	MaxFailedLoginAttempts = 5

	// LockoutWindow is the time window for counting failed login attempts.
	LockoutWindow = 15 * time.Minute
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
	customerRepo  repository.CustomerRepo
	adminRepo     *repository.AdminRepository
	auditRepo     *repository.AuditRepository
	authConfig    middleware.AuthConfig
	encryptionKey string
	logger        *slog.Logger
}

// NewAuthService creates a new AuthService with the given dependencies.
func NewAuthService(
	customerRepo repository.CustomerRepo,
	adminRepo *repository.AdminRepository,
	auditRepo *repository.AuditRepository,
	jwtSecret string,
	issuer string,
	encryptionKey string,
	logger *slog.Logger,
) *AuthService {
	return &AuthService{
		customerRepo:  customerRepo,
		adminRepo:     adminRepo,
		auditRepo:     auditRepo,
		authConfig:    middleware.AuthConfig{JWTSecret: jwtSecret, Issuer: issuer},
		encryptionKey: encryptionKey,
		logger:        logger,
	}
}

// checkLoginLockout returns ErrAccountLocked when the email has exceeded the
// failed-login threshold within LockoutWindow.
func (s *AuthService) checkLoginLockout(ctx context.Context, email string) error {
	failedCount, err := s.customerRepo.GetFailedLoginCount(ctx, email, LockoutWindow)
	if err != nil {
		return fmt.Errorf("checking login attempts: %w", err)
	}
	if failedCount >= MaxFailedLoginAttempts {
		s.logger.Warn("login attempt on locked account", "email", util.MaskEmail(email), "attempts", failedCount)
		return sharederrors.ErrAccountLocked
	}
	return nil
}

// verifyLoginCredentials fetches the customer by email and verifies the password.
// On wrong password it records the failure and returns ErrUnauthorized.
func (s *AuthService) verifyLoginCredentials(ctx context.Context, email, password string) (*models.Customer, error) {
	customer, err := s.customerRepo.GetByEmail(ctx, email)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			s.logger.Warn("login attempt for non-existent email", "email", util.MaskEmail(email))
			return nil, sharederrors.ErrUnauthorized
		}
		return nil, fmt.Errorf("getting customer by email: %w", err)
	}

	match, err := s.verifyPassword(password, customer.PasswordHash)
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

// createLoginSession mints access + refresh tokens and persists the session row.
// Returns the AuthTokens response and the plain-text refresh token.
func (s *AuthService) createLoginSession(ctx context.Context, userID, userType, role, ipAddress, userAgent string, refreshDuration time.Duration) (*models.AuthTokens, string, error) {
	accessToken, err := middleware.GenerateAccessToken(s.authConfig, userID, userType, role, AccessTokenDuration)
	if err != nil {
		return nil, "", fmt.Errorf("generating access token: %w", err)
	}
	refreshToken, err := middleware.GenerateRefreshToken()
	if err != nil {
		return nil, "", fmt.Errorf("generating refresh token: %w", err)
	}
	session := &models.Session{
		ID: uuid.New().String(), UserID: userID, UserType: userType,
		RefreshTokenHash: crypto.HashSHA256(refreshToken),
		IPAddress:        &ipAddress, UserAgent: &userAgent,
		ExpiresAt: time.Now().Add(refreshDuration),
	}
	if err := s.customerRepo.CreateSession(ctx, session); err != nil {
		return nil, "", fmt.Errorf("creating session: %w", err)
	}
	return &models.AuthTokens{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(AccessTokenDuration.Seconds()),
	}, refreshToken, nil
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

	tokens, refreshToken, err := s.createLoginSession(ctx, customer.ID, "customer", "", ipAddress, userAgent, CustomerRefreshTokenDuration)
	if err != nil {
		return nil, "", err
	}
	s.logger.Info("customer logged in", "customer_id", customer.ID)
	return tokens, refreshToken, nil
}

// validateTOTPCode decrypts the stored TOTP secret and validates the code.
// Returns true when the code is cryptographically valid.
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

// consumeBackupCode checks whether totpCode matches any stored backup code hash.
// On match it removes the code from the slice and calls updateFn to persist the
// change. Returns true if a backup code was consumed.
func (s *AuthService) consumeBackupCode(ctx context.Context, userID, totpCode string, backupCodesHash []string, updateFn func(context.Context, string, []string) error) (bool, error) {
	if len(backupCodesHash) == 0 {
		return false, nil
	}
	providedHash := crypto.HashSHA256(totpCode)
	for i, codeHash := range backupCodesHash {
		if subtle.ConstantTimeCompare([]byte(providedHash), []byte(codeHash)) != 1 {
			continue
		}
		remaining := append(backupCodesHash[:i:i], backupCodesHash[i+1:]...)
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

	tokens, refreshToken, err := s.createLoginSession(ctx, customer.ID, "customer", "", ipAddress, userAgent, CustomerRefreshTokenDuration)
	if err != nil {
		return nil, "", err
	}
	s.logger.Info("customer 2FA verified", "customer_id", customer.ID)
	return tokens, refreshToken, nil
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
		// Cleanup of expired session is best-effort; auth failure is already returned
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

	reset, err := s.customerRepo.GetPasswordResetByTokenHash(ctx, tokenHash)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return sharederrors.ErrUnauthorized
		}
		return fmt.Errorf("getting password reset: %w", err)
	}

	if time.Now().After(reset.ExpiresAt) {
		return fmt.Errorf("reset token has expired")
	}

	if reset.UsedAt != nil {
		return fmt.Errorf("reset token has already been used")
	}

	newHash, err := s.hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hashing new password: %w", err)
	}

	switch reset.UserType {
	case "customer":
		if err := s.customerRepo.UpdateCustomerPasswordHash(ctx, reset.UserID, newHash); err != nil {
			return fmt.Errorf("updating customer password: %w", err)
		}
	case "admin":
		if err := s.adminRepo.UpdatePasswordHash(ctx, reset.UserID, newHash); err != nil {
			return fmt.Errorf("updating admin password: %w", err)
		}
	default:
		return fmt.Errorf("invalid user type: %s", reset.UserType)
	}

	if err := s.customerRepo.MarkPasswordResetUsed(ctx, reset.ID); err != nil {
		s.logger.Warn("failed to mark password reset as used", "reset_id", reset.ID, "error", err)
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

// verifyAdminCredentials fetches the admin by email and verifies the password.
// On wrong password or missing email it records a failed login attempt and returns
// ErrUnauthorized. Returns the admin record on success.
func (s *AuthService) verifyAdminCredentials(ctx context.Context, email, password string) (*models.Admin, error) {
	admin, err := s.adminRepo.GetByEmail(ctx, email)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			s.logger.Warn("admin login attempt for non-existent email", "email", util.MaskEmail(email))
			// Record a failed attempt to prevent user enumeration via timing.
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

// ValidateTOTP validates a TOTP code against an encrypted secret (utility method).
// This is useful for backup code verification or re-auth scenarios.
// Delegates to validateTOTPCode to avoid duplication.
func (s *AuthService) ValidateTOTP(totpCode, encryptedSecret string) (bool, error) {
	return s.validateTOTPCode(encryptedSecret, totpCode)
}

// ConstantTimeCompare performs a constant-time comparison of two strings.
// This is used to prevent timing attacks when comparing tokens.
func ConstantTimeCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// TOTPSetupResult contains the result of initiating 2FA setup.
type TOTPSetupResult struct {
	Secret      string   // Base32 encoded TOTP secret
	QRURL       string   // otpauth:// URL for QR code generation
	BackupCodes []string // Plain text backup codes (only shown once)
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
