package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidOAuthProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     bool
	}{
		{"google is valid", "google", true},
		{"github is valid", "github", true},
		{"facebook is invalid", "facebook", false},
		{"empty is invalid", "", false},
		{"random is invalid", "random", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsValidOAuthProvider(tt.provider))
		})
	}
}

func TestOAuthLinkJSONSerialization(t *testing.T) {
	link := OAuthLink{
		ID:                    "link-1",
		CustomerID:            "cust-1",
		Provider:              "google",
		ProviderUserID:        "google-user-123",
		Email:                 "user@example.com",
		DisplayName:           "Test User",
		AvatarURL:             "https://example.com/avatar.jpg",
		AccessTokenEncrypted:  []byte("encrypted-access"),
		RefreshTokenEncrypted: []byte("encrypted-refresh"),
	}

	data, err := json.Marshal(link)
	require.NoError(t, err)

	jsonStr := string(data)
	assert.Contains(t, jsonStr, `"provider":"google"`)
	assert.Contains(t, jsonStr, `"email":"user@example.com"`)
	assert.Contains(t, jsonStr, `"display_name":"Test User"`)

	// Sensitive fields must not be serialized
	assert.NotContains(t, jsonStr, "provider_user_id")
	assert.NotContains(t, jsonStr, "access_token")
	assert.NotContains(t, jsonStr, "refresh_token")
	assert.NotContains(t, jsonStr, "encrypted-access")
	assert.NotContains(t, jsonStr, "encrypted-refresh")
}

func TestOAuthAuthorizeRequestValidation(t *testing.T) {
	req := OAuthAuthorizeRequest{
		CodeChallenge: "aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789ABCDEF-_",
		State:         "randomstate12345678",
		RedirectURI:   "https://example.com/callback",
	}
	assert.NotEmpty(t, req.CodeChallenge)
	assert.NotEmpty(t, req.State)
	assert.NotEmpty(t, req.RedirectURI)
}
