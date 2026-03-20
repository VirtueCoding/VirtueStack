package customer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// customerAPIKeyGetter is the minimal interface needed by CustomerAPIKeyValidator.
// This allows testing without the full repository.
type customerAPIKeyGetter interface {
	GetByHash(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error)
}

// mockCustomerAPIKeyGetter is a mock implementation for testing.
type mockCustomerAPIKeyGetter struct {
	getByHashFunc func(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error)
}

func (m *mockCustomerAPIKeyGetter) GetByHash(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error) {
	if m.getByHashFunc != nil {
		return m.getByHashFunc(ctx, keyHash)
	}
	return nil, errors.New("not found")
}

// testCustomerAPIKeyValidator creates a validator using the minimal interface.
// This mirrors CustomerAPIKeyValidator but accepts the test interface.
func testCustomerAPIKeyValidator(repo customerAPIKeyGetter) middleware.CustomerAPIKeyValidator {
	return func(ctx context.Context, keyHash string) (middleware.CustomerAPIKeyInfo, error) {
		key, err := repo.GetByHash(ctx, keyHash)
		if err != nil {
			return middleware.CustomerAPIKeyInfo{}, err
		}

		if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
			return middleware.CustomerAPIKeyInfo{}, errors.New("API key has expired")
		}

		return middleware.CustomerAPIKeyInfo{
			KeyID:       key.ID,
			CustomerID:  key.CustomerID,
			Permissions: key.Permissions,
		}, nil
	}
}

func TestCustomerAPIKeyValidator(t *testing.T) {
	now := time.Now()
	futureExpiry := now.Add(24 * time.Hour)
	pastExpiry := now.Add(-24 * time.Hour)

	tests := []struct {
		name           string
		key            *models.CustomerAPIKey
		repoErr        error
		keyHash        string
		wantErr        bool
		wantInfo       middleware.CustomerAPIKeyInfo
		wantErrContain string
	}{
		{
			name:    "valid key returns info",
			keyHash: "valid-hash",
			key: &models.CustomerAPIKey{
				ID:          "key-123",
				CustomerID:  "customer-456",
				Permissions: []string{"vm:read", "vm:write"},
				IsActive:    true,
			},
			wantInfo: middleware.CustomerAPIKeyInfo{
				KeyID:       "key-123",
				CustomerID:  "customer-456",
				Permissions: []string{"vm:read", "vm:write"},
			},
		},
		{
			name:    "key not found returns error",
			keyHash: "unknown-hash",
			repoErr: errors.New("not found"),
			wantErr: true,
		},
		{
			name:    "expired key returns error",
			keyHash: "expired-hash",
			key: &models.CustomerAPIKey{
				ID:          "key-expired",
				CustomerID:  "customer-789",
				Permissions: []string{"vm:read"},
				ExpiresAt:   &pastExpiry,
				IsActive:    true,
			},
			wantErr:        true,
			wantErrContain: "expired",
		},
		{
			name:    "key with no expiry is valid",
			keyHash: "no-expiry-hash",
			key: &models.CustomerAPIKey{
				ID:          "key-noexpiry",
				CustomerID:  "customer-101",
				Permissions: []string{"vm:read", "vm:write", "vm:power"},
				ExpiresAt:   nil,
				IsActive:    true,
			},
			wantInfo: middleware.CustomerAPIKeyInfo{
				KeyID:       "key-noexpiry",
				CustomerID:  "customer-101",
				Permissions: []string{"vm:read", "vm:write", "vm:power"},
			},
		},
		{
			name:    "key with future expiry is valid",
			keyHash: "future-expiry-hash",
			key: &models.CustomerAPIKey{
				ID:          "key-future",
				CustomerID:  "customer-202",
				Permissions: []string{"backup:read"},
				ExpiresAt:   &futureExpiry,
				IsActive:    true,
			},
			wantInfo: middleware.CustomerAPIKeyInfo{
				KeyID:       "key-future",
				CustomerID:  "customer-202",
				Permissions: []string{"backup:read"},
			},
		},
		{
			name:    "revoked key returns error from repo",
			keyHash: "revoked-hash",
			repoErr: errors.New("not found"),
			wantErr: true,
		},
		{
			name:    "empty permissions still valid",
			keyHash: "empty-perms-hash",
			key: &models.CustomerAPIKey{
				ID:          "key-empty",
				CustomerID:  "customer-303",
				Permissions: []string{},
				IsActive:    true,
			},
			wantInfo: middleware.CustomerAPIKeyInfo{
				KeyID:       "key-empty",
				CustomerID:  "customer-303",
				Permissions: []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockGetter := &mockCustomerAPIKeyGetter{
				getByHashFunc: func(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error) {
					if tt.repoErr != nil {
						return nil, tt.repoErr
					}
					if tt.key != nil {
						return tt.key, nil
					}
					return nil, errors.New("not found")
				},
			}

			validator := testCustomerAPIKeyValidator(mockGetter)
			info, err := validator(context.Background(), tt.keyHash)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContain != "" {
					assert.Contains(t, err.Error(), tt.wantErrContain)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantInfo.KeyID, info.KeyID)
				assert.Equal(t, tt.wantInfo.CustomerID, info.CustomerID)
				assert.Equal(t, tt.wantInfo.Permissions, info.Permissions)
			}
		})
	}
}

func TestCustomerAPIKeyValidator_HashVerification(t *testing.T) {
	expectedHash := "sha256-hash-of-api-key"
	var receivedHash string

	mockGetter := &mockCustomerAPIKeyGetter{
		getByHashFunc: func(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error) {
			receivedHash = keyHash
			return &models.CustomerAPIKey{
				ID:          "key-1",
				CustomerID:  "customer-1",
				Permissions: []string{"vm:read"},
			}, nil
		},
	}

	validator := testCustomerAPIKeyValidator(mockGetter)
	_, err := validator(context.Background(), expectedHash)
	require.NoError(t, err)
	assert.Equal(t, expectedHash, receivedHash, "validator should pass the hash to repository unchanged")
}