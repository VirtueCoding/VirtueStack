package provisioning

// Test credentials in this file are for testing purposes only.
// DO NOT use these credentials in production environments.

import (
	cryptorand "crypto/rand"
	"errors"
	"io"
	"testing"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingReader struct{}

func (failingReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		wantErr     bool
		errField    string
		errContains string
	}{
		{
			name:     "valid password with all requirements",
			password: "ValidPass123!@#",
			wantErr:  false,
		},
		{
			name:     "valid password exactly 12 chars",
			password: "Valid12!@#$%",
			wantErr:  false,
		},
		{
			name:     "valid password with max length 128",
			password: "ValidPass123!" + string(make([]byte, 113)),
			wantErr:  false,
		},
		{
			name:        "too short - 11 chars",
			password:    "Short1!aA",
			wantErr:     true,
			errField:    "password",
			errContains: "at least 12 characters",
		},
		{
			name:        "too short - empty",
			password:    "",
			wantErr:     true,
			errField:    "password",
			errContains: "at least 12 characters",
		},
		{
			name:        "too long - 129 chars",
			password:    string(make([]byte, 129)),
			wantErr:     true,
			errField:    "password",
			errContains: "not exceed 128 characters",
		},
		{
			name:        "missing uppercase",
			password:    "validpass123!@#",
			wantErr:     true,
			errField:    "password",
			errContains: "uppercase",
		},
		{
			name:        "missing lowercase",
			password:    "VALIDPASS123!@#",
			wantErr:     true,
			errField:    "password",
			errContains: "lowercase",
		},
		{
			name:        "missing digit",
			password:    "ValidPassword!@#",
			wantErr:     true,
			errField:    "password",
			errContains: "digit",
		},
		{
			name:        "missing special character",
			password:    "ValidPassword123",
			wantErr:     true,
			errField:    "password",
			errContains: "special character",
		},
		{
			name:     "valid with various special chars",
			password: "ValidPass123!@#$%^&*",
			wantErr:  false,
		},
		{
			name:     "valid with unicode special chars",
			password: "ValidPass123\xF0\x9F\x98\x80",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePasswordStrength(tt.password)

			if tt.wantErr {
				require.Error(t, err)

				// Check that it returns a ValidationError
				var validationErr *sharederrors.ValidationError
				assert.True(t, errors.As(err, &validationErr), "error should be a ValidationError")

				if validationErr != nil {
					assert.Equal(t, tt.errField, validationErr.Field)
					assert.Contains(t, validationErr.Issue, tt.errContains)
				}

				// Check it can be checked with errors.Is
				assert.True(t, errors.Is(err, sharederrors.ErrValidation))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid password",
			password: "ValidPass123!@#",
			wantErr:  false,
		},
		{
			name:        "empty password",
			password:    "",
			wantErr:     true,
			errContains: "empty",
		},
		{
			name:        "weak password",
			password:    "weak",
			wantErr:     true,
			errContains: "12 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := hashPassword(tt.password)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Empty(t, hash)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, hash)
				// Argon2id hashes start with $argon2id$
				assert.Contains(t, hash, "$argon2id$")
			}
		})
	}
}

func TestValidatePasswordStrength_ReturnsTypedError(t *testing.T) {
	// This test verifies QG-04 compliance - typed errors instead of fmt.Errorf
	err := validatePasswordStrength("short")

	require.Error(t, err)

	// Verify it's a ValidationError
	var validationErr *sharederrors.ValidationError
	require.True(t, errors.As(err, &validationErr), "error should be a ValidationError")

	// Verify the field is set
	assert.Equal(t, "password", validationErr.Field)
	assert.NotEmpty(t, validationErr.Issue)

	// Verify it supports errors.Is
	assert.True(t, errors.Is(err, sharederrors.ErrValidation))
}

func TestGenerateRandomPassword(t *testing.T) {
	password, err := generateRandomPassword()

	require.NoError(t, err)
	assert.Len(t, password, 16)
	assert.NoError(t, validatePasswordStrength(password))
}

func TestGenerateRandomPassword_ReturnsErrorOnEntropyFailure(t *testing.T) {
	originalReader := cryptorand.Reader
	cryptorand.Reader = failingReader{}
	t.Cleanup(func() {
		cryptorand.Reader = originalReader
	})

	var (
		password string
		err      error
	)
	assert.NotPanics(t, func() {
		password, err = generateRandomPassword()
	})

	require.Error(t, err)
	assert.Empty(t, password)
	assert.Contains(t, err.Error(), "random password")
}
