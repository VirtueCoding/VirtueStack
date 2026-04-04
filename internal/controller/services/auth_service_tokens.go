// Package services provides business logic services for VirtueStack Controller.
//
// This file contains token refresh and logout operations.
package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/google/uuid"
)

type refreshSessionRotator interface {
	RotateSession(
		ctx context.Context,
		currentRefreshTokenHash string,
		newSession *models.Session,
	) (*models.Session, error)
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
		if deleteErr := s.customerRepo.DeleteSession(ctx, session.ID); deleteErr != nil {
			s.logger.Warn("failed to delete expired customer session",
				"session_id", session.ID,
				"customer_id", session.UserID,
				"error", deleteErr)
		}
		return nil, "", sharederrors.ErrUnauthorized
	}

	// Determine refresh token duration and get role based on user type
	refreshDuration := CustomerRefreshTokenDuration
	var role string

	if session.UserType == "admin" {
		refreshDuration = AdminRefreshTokenDuration
		// Fetch admin to get their role
		admin, adminErr := s.adminRepo.GetByID(ctx, session.UserID)
		if adminErr != nil {
			return nil, "", fmt.Errorf("getting admin for refresh: %w", adminErr)
		}
		role = admin.Role
	} else {
		customer, customerErr := s.customerRepo.GetByID(ctx, session.UserID)
		if customerErr != nil {
			if sharederrors.Is(customerErr, sharederrors.ErrNotFound) {
				return nil, "", sharederrors.ErrUnauthorized
			}
			return nil, "", fmt.Errorf("getting customer for refresh: %w", customerErr)
		}
		if customer.Status != models.CustomerStatusActive {
			return nil, "", sharederrors.ErrUnauthorized
		}
	}

	// Generate new access token
	// Generate new refresh token (rotation)
	newRefreshToken, err := middleware.GenerateRefreshToken()
	if err != nil {
		return nil, "", fmt.Errorf("generating refresh token: %w", err)
	}

	// Create new session BEFORE deleting the old one to prevent the user from
	// being permanently logged out if CreateSession fails (F-057).
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

	if rotator, ok := s.customerRepo.(refreshSessionRotator); ok {
		rotatedSession, rotateErr := rotator.RotateSession(ctx, refreshTokenHash, newSession)
		if rotateErr != nil {
			if errors.Is(rotateErr, repository.ErrNoRowsAffected) {
				return nil, "", sharederrors.ErrUnauthorized
			}
			return nil, "", fmt.Errorf("rotating session: %w", rotateErr)
		}
		session = rotatedSession
	} else {
		// Create new session BEFORE deleting the old one to prevent the user from
		// being permanently logged out if CreateSession fails (legacy/test path).
		if createErr := s.customerRepo.CreateSession(ctx, newSession); createErr != nil {
			return nil, "", fmt.Errorf("creating new session: %w", createErr)
		}

		// Delete old session only after the new one is confirmed created.
		if deleteErr := s.customerRepo.DeleteSession(ctx, session.ID); deleteErr != nil {
			s.logger.Warn("failed to delete old session", "session_id", session.ID, "error", deleteErr)
		}
	}

	accessToken, err := middleware.GenerateAccessToken(s.authConfig, session.UserID, session.UserType, role, newSession.ID, AccessTokenDuration)
	if err != nil {
		return nil, "", fmt.Errorf("generating access token: %w", err)
	}

	sessionCleanupToken, err := middleware.GenerateSessionCleanupToken(s.authConfig, session.UserID, session.UserType, role, newSession.ID)
	if err != nil {
		return nil, "", fmt.Errorf("generating session cleanup token: %w", err)
	}

	s.logger.Info("token refreshed", "user_id", session.UserID, "new_session_id", newSession.ID)

	return &models.AuthTokens{
		AccessToken:         accessToken,
		TokenType:           "Bearer",
		ExpiresIn:           int(AccessTokenDuration.Seconds()),
		SessionID:           newSession.ID,
		SessionCleanupToken: sessionCleanupToken,
	}, newRefreshToken, nil
}

// Logout invalidates a single session.
func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	if err := s.customerRepo.DeleteSession(ctx, sessionID); err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) || sharederrors.Is(err, sharederrors.ErrNoRowsAffected) {
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

// ValidateAccessSession confirms that the session backing an access token still exists
// and has not expired. Missing or expired sessions are treated as unauthorized.
func (s *AuthService) ValidateAccessSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return sharederrors.ErrUnauthorized
	}

	session, err := s.customerRepo.GetSession(ctx, sessionID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return sharederrors.ErrUnauthorized
		}
		return fmt.Errorf("getting session: %w", err)
	}

	if time.Now().After(session.ExpiresAt) {
		return sharederrors.ErrUnauthorized
	}

	return nil
}
