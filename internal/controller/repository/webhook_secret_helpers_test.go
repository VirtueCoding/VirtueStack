package repository

import (
	"testing"

	sharedcrypto "github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptWebhookSecret(t *testing.T) {
	key, err := sharedcrypto.GenerateEncryptionKey()
	require.NoError(t, err)

	encryptedPayload, err := sharedcrypto.Encrypt("already-encrypted", key)
	require.NoError(t, err)

	tests := []struct {
		name     string
		secret   string
		key      string
		expected string
		check    func(t *testing.T, got string)
	}{
		{
			name:     "passes through plaintext without key",
			secret:   "plain-secret",
			key:      "",
			expected: "plain-secret",
		},
		{
			name:     "passes through empty secret",
			secret:   "",
			key:      key,
			expected: "",
		},
		{
			name:   "encrypts plaintext with prefix",
			secret: "plain-secret",
			key:    key,
			check: func(t *testing.T, got string) {
				t.Helper()
				assert.NotEqual(t, "plain-secret", got)
				assert.Contains(t, got, webhookSecretEncryptedPrefix)
			},
		},
		{
			name:     "does not double encrypt prefixed secrets",
			secret:   webhookSecretEncryptedPrefix + encryptedPayload,
			key:      key,
			expected: webhookSecretEncryptedPrefix + encryptedPayload,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encryptWebhookSecret(tt.secret, tt.key)
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, got)
				return
			}
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestDecryptWebhookSecret(t *testing.T) {
	key, err := sharedcrypto.GenerateEncryptionKey()
	require.NoError(t, err)

	encryptedPayload, err := sharedcrypto.Encrypt("plain-secret", key)
	require.NoError(t, err)

	tests := []struct {
		name     string
		secret   string
		key      string
		expected string
	}{
		{
			name:     "passes through plaintext without key",
			secret:   "plain-secret",
			key:      "",
			expected: "plain-secret",
		},
		{
			name:     "passes through unprefixed plaintext with key",
			secret:   "plain-secret",
			key:      key,
			expected: "plain-secret",
		},
		{
			name:     "decrypts prefixed ciphertext",
			secret:   webhookSecretEncryptedPrefix + encryptedPayload,
			key:      key,
			expected: "plain-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decryptWebhookSecret(tt.secret, tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}
