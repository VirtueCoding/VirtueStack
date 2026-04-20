package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoogleOAuth_AuthorizationURL(t *testing.T) {
	provider := NewGoogleOAuthProvider("test-client-id", "test-secret")
	url := provider.AuthorizationURL("test-challenge", "test-state", "https://example.com/callback")

	assert.Contains(t, url, "accounts.google.com")
	assert.Contains(t, url, "client_id=test-client-id")
	assert.Contains(t, url, "code_challenge=test-challenge")
	assert.Contains(t, url, "code_challenge_method=S256")
	assert.Contains(t, url, "state=test-state")
	assert.Contains(t, url, "redirect_uri=")
	assert.Contains(t, url, "scope=openid+email+profile")
}

func TestGoogleOAuth_ExchangeCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		resp := map[string]any{
			"access_token":  "google-access-token",
			"refresh_token": "google-refresh-token",
			"expires_in":    3600,
			"token_type":    "Bearer",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &GoogleOAuthProvider{
		clientID:     "test-id",
		clientSecret: "test-secret",
		httpClient:   server.Client(),
	}

	// Override the token URL for testing
	origTokenURL := googleTokenURL
	defer func() { _ = origTokenURL }()

	ctx := t.Context()
	tokens, err := provider.exchangeCodeWithURL(ctx, server.URL, "test-code", "test-verifier", "https://example.com/cb")
	require.NoError(t, err)
	assert.Equal(t, "google-access-token", tokens.AccessToken)
	assert.Equal(t, "google-refresh-token", tokens.RefreshToken)
	assert.Equal(t, "Bearer", tokens.TokenType)
}

func TestGoogleOAuth_GetUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer test-token")

		resp := map[string]any{
			"sub":     "google-sub-123",
			"email":   "user@example.com",
			"name":    "Test User",
			"picture": "https://example.com/photo.jpg",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &GoogleOAuthProvider{
		clientID:   "test-id",
		httpClient: server.Client(),
	}

	ctx := t.Context()
	info, err := provider.getUserInfoWithURL(ctx, server.URL, "test-token")
	require.NoError(t, err)
	assert.Equal(t, "google-sub-123", info.ProviderUserID)
	assert.Equal(t, "user@example.com", info.Email)
	assert.Equal(t, "Test User", info.Name)
	assert.Equal(t, "https://example.com/photo.jpg", info.AvatarURL)
}
