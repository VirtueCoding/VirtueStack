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

	// MaxCustomerSessions is the maximum concurrent sessions for customer users.
	MaxCustomerSessions = 10

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

// dummyArgon2idHash is a pre-computed Argon2id hash used for timing-attack
// mitigation in login flows when the email does not exist (F-149).
// It is generated once at startup with a random salt so that the hash is
// realistic (same work factor as real hashes) and cannot be distinguished by
// timing from a real password verification.
var dummyArgon2idHash string

func init() {
	var err error
	// Use a fixed dummy password; the randomness comes from the Argon2id salt.
	dummyArgon2idHash, err = argon2id.CreateHash("__dummy_timing_mitigation__", Argon2idParams)
	if err != nil {
		// Fallback to a static hash if random salt generation fails.
		// This is better than panicking at startup.
		dummyArgon2idHash = "$argon2id$v=19$m=65536,t=3,p=4$c29tZXNhbHQ$RdescudvJCsgt3ub+b+dWRWJTmaaJObG"
	}
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
// userType must be "admin" or "customer" so that admin and customer failed-login
// counters are tracked independently (F-030).
func (s *AuthService) checkLoginLockout(ctx context.Context, email string) error {
	return s.checkLoginLockoutForType(ctx, email, "customer")
}

// checkLoginLockoutForType returns ErrAccountLocked when the email+userType pair
// has exceeded the failed-login threshold within LockoutWindow (F-030).
// The lockout key incorporates the userType so admin and customer counters
// are tracked separately.
func (s *AuthService) checkLoginLockoutForType(ctx context.Context, email, userType string) error {
	lockoutKey := userType + ":" + email
	failedCount, err := s.customerRepo.GetFailedLoginCount(ctx, lockoutKey, LockoutWindow)
	if err != nil {
		return fmt.Errorf("checking login attempts: %w", err)
	}
	if failedCount >= MaxFailedLoginAttempts {
		s.logger.Warn("login attempt on locked account",
			"email", util.MaskEmail(email),
			"user_type", userType,
			"attempts", failedCount)
		return sharederrors.ErrAccountLocked
	}
	return nil
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

// hashPassword hashes a password using Argon2id.
func (s *AuthService) hashPassword(password string) (string, error) {
	hash, err := argon2id.CreateHash(password, Argon2idParams)
	if err != nil {
		return "", fmt.Errorf("creating password hash: %w", err)
	}
	return hash, nil
}

// HashPassword hashes a password using Argon2id.
// This is the public version for use by other services.
func (s *AuthService) HashPassword(password string) (string, error) {
	return s.hashPassword(password)
}

// verifyPassword verifies a password against an Argon2id hash.
func (s *AuthService) verifyPassword(password, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, fmt.Errorf("comparing password: %w", err)
	}
	return match, nil
}

// VerifyPassword verifies a password against an Argon2id hash.
// This is the public version for use by other services.
func (s *AuthService) VerifyPassword(password, hash string) (bool, error) {
	return s.verifyPassword(password, hash)
}

// ConstantTimeCompare performs a constant-time comparison of two strings.
// This is used to prevent timing attacks when comparing tokens.
// The length pre-check is intentionally omitted: subtle.ConstantTimeCompare
// already handles unequal lengths correctly and the pre-check would leak
// length information via timing (F-151).
func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// enforceCustomerSessionLimit evicts the oldest customer session when MaxCustomerSessions
// is already reached. Failures are logged and do not block login.
func (s *AuthService) enforceCustomerSessionLimit(ctx context.Context, customerID string) {
	count, err := s.customerRepo.CountSessionsByUser(ctx, customerID, "customer")
	if err != nil {
		s.logger.Warn("failed to count customer sessions", "customer_id", customerID, "error", err)
		return
	}
	if count >= MaxCustomerSessions {
		if err := s.customerRepo.DeleteOldestSession(ctx, customerID, "customer"); err != nil {
			s.logger.Warn("failed to delete oldest customer session", "customer_id", customerID, "error", err)
		}
	}
}