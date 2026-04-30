package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenUsesAtomicSessionRotation(t *testing.T) {
	refreshToken := "old-refresh-token"
	repo := newMockLoginCustomerRepo(nil)
	repo.sessionByRefreshToken = &models.Session{
		ID:               "old-session",
		UserID:           "customer-1",
		UserType:         "customer",
		RefreshTokenHash: crypto.HashSHA256(refreshToken),
		ExpiresAt:        time.Now().Add(time.Hour),
	}
	repo.rotateSessionErr = errors.New("rotation failed")

	service := NewAuthService(
		repo,
		nil,
		nil,
		"test-secret-key-that-is-32-bytes-long!!",
		"virtuestack",
		"",
		slog.Default(),
	)

	tokens, newRefreshToken, err := service.RefreshToken(context.Background(), refreshToken, "127.0.0.1", "test-agent")

	require.Error(t, err)
	require.Nil(t, tokens)
	require.Empty(t, newRefreshToken)
	require.Equal(t, "old-session", repo.rotatedOldSessionID)
	require.NotNil(t, repo.rotatedNewSession)
	require.Empty(t, repo.createdSessionID)
	require.Empty(t, repo.deletedSessionIDs)
}

func TestRefreshTokenTreatsConsumedSessionAsUnauthorized(t *testing.T) {
	tests := []struct {
		name              string
		rotationErr       error
		wantErr           error
		wantEmptyNewToken bool
	}{
		{
			name:              "repository no rows affected",
			rotationErr:       fmt.Errorf("delete old session: %w", repository.ErrNoRowsAffected),
			wantErr:           sharederrors.ErrUnauthorized,
			wantEmptyNewToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refreshToken := "old-refresh-token"
			repo := newMockLoginCustomerRepo(nil)
			repo.sessionByRefreshToken = &models.Session{
				ID:               "old-session",
				UserID:           "customer-1",
				UserType:         "customer",
				RefreshTokenHash: crypto.HashSHA256(refreshToken),
				ExpiresAt:        time.Now().Add(time.Hour),
			}
			repo.rotateSessionErr = tt.rotationErr

			service := NewAuthService(
				repo,
				nil,
				nil,
				"test-secret-key-that-is-32-bytes-long!!",
				"virtuestack",
				"",
				slog.Default(),
			)

			tokens, newRefreshToken, err := service.RefreshToken(context.Background(), refreshToken, "127.0.0.1", "test-agent")

			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, tokens)
			if tt.wantEmptyNewToken {
				require.Empty(t, newRefreshToken)
			}
		})
	}
}
