// Package services provides business logic services for VirtueStack Controller.
//
// This file contains token refresh and logout operations.
package services

import (
	"context"
	"fmt"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/google/uuid"
)

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