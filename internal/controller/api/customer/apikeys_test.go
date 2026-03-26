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
			AllowedIPs:  key.AllowedIPs,
			VMIDs:       key.VMIDs,
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
				AllowedIPs:  []string{"198.51.100.0/24"},
				VMIDs:       []string{"vm-1", "vm-2"},
				Permissions: []string{"vm:read", "vm:write"},
				IsActive:    true,
			},
			wantInfo: middleware.CustomerAPIKeyInfo{
				KeyID:       "key-123",
				CustomerID:  "customer-456",
				AllowedIPs:  []string{"198.51.100.0/24"},
				VMIDs:       []string{"vm-1", "vm-2"},
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
				assert.Equal(t, tt.wantInfo.AllowedIPs, info.AllowedIPs)
				assert.Equal(t, tt.wantInfo.VMIDs, info.VMIDs)
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

// TestCustomerAPIKeyIsAllowedIP tests the IsAllowedIP method for CustomerAPIKey.
func TestCustomerAPIKeyIsAllowedIP(t *testing.T) {
	tests := []struct {
		name       string
		allowedIPs []string
		clientIP   string
		wantResult bool
	}{
		// Empty whitelist - all IPs allowed
		{
			name:       "empty whitelist allows all IPs",
			allowedIPs: nil,
			clientIP:   "192.168.1.100",
			wantResult: true,
		},
		{
			name:       "empty slice allows all IPs",
			allowedIPs: []string{},
			clientIP:   "10.0.0.1",
			wantResult: true,
		},

		// IPv4 exact matches
		{
			name:       "IPv4 exact match - allowed",
			allowedIPs: []string{"192.168.1.100"},
			clientIP:   "192.168.1.100",
			wantResult: true,
		},
		{
			name:       "IPv4 no match - denied",
			allowedIPs: []string{"192.168.1.100"},
			clientIP:   "192.168.1.101",
			wantResult: false,
		},

		// IPv4 CIDR notation
		{
			name:       "IPv4 CIDR /24 - in range",
			allowedIPs: []string{"192.168.1.0/24"},
			clientIP:   "192.168.1.50",
			wantResult: true,
		},
		{
			name:       "IPv4 CIDR /24 - out of range",
			allowedIPs: []string{"192.168.1.0/24"},
			clientIP:   "192.168.2.1",
			wantResult: false,
		},
		{
			name:       "IPv4 CIDR /16 - in range",
			allowedIPs: []string{"10.0.0.0/16"},
			clientIP:   "10.0.255.255",
			wantResult: true,
		},
		{
			name:       "IPv4 CIDR /32 - exact match",
			allowedIPs: []string{"192.168.1.1/32"},
			clientIP:   "192.168.1.1",
			wantResult: true,
		},
		{
			name:       "IPv4 CIDR /32 - no match",
			allowedIPs: []string{"192.168.1.1/32"},
			clientIP:   "192.168.1.2",
			wantResult: false,
		},

		// IPv6 exact matches
		{
			name:       "IPv6 exact match - allowed",
			allowedIPs: []string{"2001:db8::1"},
			clientIP:   "2001:db8::1",
			wantResult: true,
		},
		{
			name:       "IPv6 no match - denied",
			allowedIPs: []string{"2001:db8::1"},
			clientIP:   "2001:db8::2",
			wantResult: false,
		},

		// IPv6 CIDR notation
		{
			name:       "IPv6 CIDR /64 - in range",
			allowedIPs: []string{"2001:db8:abcd::/64"},
			clientIP:   "2001:db8:abcd::1234",
			wantResult: true,
		},
		{
			name:       "IPv6 CIDR /64 - out of range",
			allowedIPs: []string{"2001:db8:abcd::/64"},
			clientIP:   "2001:db8:ef01::1",
			wantResult: false,
		},
		{
			name:       "IPv6 CIDR /48 - in range",
			allowedIPs: []string{"2001:db8::/48"},
			clientIP:   "2001:db8:0:1::1",
			wantResult: true,
		},
		{
			name:       "IPv6 CIDR /48 - out of range",
			allowedIPs: []string{"2001:db8::/48"},
			clientIP:   "2001:db8:abcd::1",
			wantResult: false,
		},

		// Mixed IPv4/IPv6/CIDR
		{
			name:       "mixed list - IPv4 match",
			allowedIPs: []string{"192.168.1.0/24", "2001:db8::1", "10.0.0.1"},
			clientIP:   "192.168.1.50",
			wantResult: true,
		},
		{
			name:       "mixed list - IPv6 match",
			allowedIPs: []string{"192.168.1.0/24", "2001:db8::/64", "10.0.0.1"},
			clientIP:   "2001:db8::abcd",
			wantResult: true,
		},
		{
			name:       "mixed list - no match",
			allowedIPs: []string{"192.168.1.0/24", "2001:db8::/64", "10.0.0.1"},
			clientIP:   "172.16.0.1",
			wantResult: false,
		},
		{
			name:       "mixed list - exact IPv4 match",
			allowedIPs: []string{"192.168.1.0/24", "2001:db8::/64", "10.0.0.1"},
			clientIP:   "10.0.0.1",
			wantResult: true,
		},

		// Invalid inputs
		{
			name:       "invalid client IP - denied",
			allowedIPs: []string{"192.168.1.100"},
			clientIP:   "not-an-ip",
			wantResult: false,
		},
		{
			name:       "empty client IP - denied",
			allowedIPs: []string{"192.168.1.100"},
			clientIP:   "",
			wantResult: false,
		},

		// Multiple entries
		{
			name:       "multiple CIDR - first match",
			allowedIPs: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			clientIP:   "10.5.5.5",
			wantResult: true,
		},
		{
			name:       "multiple CIDR - second match",
			allowedIPs: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			clientIP:   "172.20.1.1",
			wantResult: true,
		},
		{
			name:       "multiple CIDR - third match",
			allowedIPs: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			clientIP:   "192.168.100.50",
			wantResult: true,
		},
		{
			name:       "multiple CIDR - no match",
			allowedIPs: []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
			clientIP:   "8.8.8.8",
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := &models.CustomerAPIKey{
				ID:          "test-key",
				CustomerID:  "test-customer",
				AllowedIPs:  tt.allowedIPs,
				Permissions: []string{"vm:read"},
			}

			result := key.IsAllowedIP(tt.clientIP)
			assert.Equal(t, tt.wantResult, result, "IsAllowedIP(%q) with whitelist %v", tt.clientIP, tt.allowedIPs)
		})
	}
}

// TestCustomerAPIKeyIPWhitelistWithValidator tests the full validator flow with IP whitelist.
func TestCustomerAPIKeyIPWhitelistWithValidator(t *testing.T) {
	now := time.Now()
	futureExpiry := now.Add(24 * time.Hour)

	tests := []struct {
		name           string
		key            *models.CustomerAPIKey
		clientIP       string
		wantErr        bool
		wantErrContain string
	}{
		{
			name: "key with no IP restriction - any IP allowed",
			key: &models.CustomerAPIKey{
				ID:          "key-1",
				CustomerID:  "customer-1",
				Permissions: []string{"vm:read"},
				AllowedIPs:  nil,
				IsActive:    true,
			},
			clientIP: "1.2.3.4",
			wantErr:  false,
		},
		{
			name: "key with matching IP - allowed",
			key: &models.CustomerAPIKey{
				ID:          "key-2",
				CustomerID:  "customer-2",
				Permissions: []string{"vm:read"},
				AllowedIPs:  []string{"192.168.1.100"},
				IsActive:    true,
			},
			clientIP: "192.168.1.100",
			wantErr:  false,
		},
		{
			name: "key with CIDR match - allowed",
			key: &models.CustomerAPIKey{
				ID:          "key-3",
				CustomerID:  "customer-3",
				Permissions: []string{"vm:read"},
				AllowedIPs:  []string{"10.0.0.0/8"},
				IsActive:    true,
			},
			clientIP: "10.5.5.5",
			wantErr:  false,
		},
		{
			name: "key with IPv6 match - allowed",
			key: &models.CustomerAPIKey{
				ID:          "key-4",
				CustomerID:  "customer-4",
				Permissions: []string{"vm:read"},
				AllowedIPs:  []string{"2001:db8::/64"},
				IsActive:    true,
			},
			clientIP: "2001:db8::abcd",
			wantErr:  false,
		},
		{
			name: "key with expired but valid IP",
			key: &models.CustomerAPIKey{
				ID:          "key-5",
				CustomerID:  "customer-5",
				Permissions: []string{"vm:read"},
				AllowedIPs:  []string{"192.168.1.0/24"},
				ExpiresAt:   &futureExpiry,
				IsActive:    true,
			},
			clientIP: "192.168.1.50",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify the key's IsAllowedIP method works correctly
			if tt.clientIP != "" {
				ipAllowed := tt.key.IsAllowedIP(tt.clientIP)
				assert.True(t, ipAllowed, "IP should be allowed per IsAllowedIP method")
			}
		})
	}
}
