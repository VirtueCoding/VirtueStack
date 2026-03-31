package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubOAuth_AuthorizationURL(t *testing.T) {
	provider := NewGitHubOAuthProvider("test-client-id", "test-secret")
	url := provider.AuthorizationURL("test-challenge", "test-state", "https://example.com/callback")

	assert.Contains(t, url, "github.com/login/oauth/authorize")
	assert.Contains(t, url, "client_id=test-client-id")
	assert.Contains(t, url, "code_challenge=test-challenge")
	assert.Contains(t, url, "code_challenge_method=S256")
	assert.Contains(t, url, "state=test-state")
	assert.Contains(t, url, "scope=read%3Auser+user%3Aemail")
}

func TestGitHubOAuth_ExchangeCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))

		resp := map[string]any{
			"access_token": "github-access-token",
			"token_type":   "bearer",
			"scope":        "read:user,user:email",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &GitHubOAuthProvider{
		clientID:     "test-id",
		clientSecret: "test-secret",
		httpClient:   server.Client(),
	}

	ctx := t.Context()
	tokens, err := provider.exchangeCodeWithURL(ctx, server.URL, "test-code", "test-verifier", "https://example.com/cb")
	require.NoError(t, err)
	assert.Equal(t, "github-access-token", tokens.AccessToken)
	assert.Equal(t, "bearer", tokens.TokenType)
}

func TestGitHubOAuth_ExchangeCode_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"error":             "bad_verification_code",
			"error_description": "The code passed is incorrect or expired.",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &GitHubOAuthProvider{
		clientID:     "test-id",
		clientSecret: "test-secret",
		httpClient:   server.Client(),
	}

	ctx := t.Context()
	_, err := provider.exchangeCodeWithURL(ctx, server.URL, "bad-code", "test-verifier", "https://example.com/cb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad_verification_code")
}

func TestGitHubOAuth_GetUserInfo_WithPublicEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"id":         12345,
			"login":      "testuser",
			"name":       "Test User",
			"email":      "user@example.com",
			"avatar_url": "https://avatars.githubusercontent.com/u/12345",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &GitHubOAuthProvider{
		clientID:   "test-id",
		httpClient: server.Client(),
	}

	ctx := t.Context()
	info, err := provider.getUserInfoWithURL(ctx, server.URL, "", "test-token")
	require.NoError(t, err)
	assert.Equal(t, "12345", info.ProviderUserID)
	assert.Equal(t, "user@example.com", info.Email)
	assert.Equal(t, "Test User", info.Name)
}

func TestGitHubOAuth_GetUserInfo_PrivateEmail(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// /user response with no email
			resp := map[string]any{
				"id":    12345,
				"login": "testuser",
				"name":  "Test User",
				"email": "",
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			// /user/emails response
			resp := []map[string]any{
				{"email": "secondary@example.com", "primary": false, "verified": true},
				{"email": "primary@example.com", "primary": true, "verified": true},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	provider := &GitHubOAuthProvider{
		clientID:   "test-id",
		httpClient: server.Client(),
	}

	ctx := t.Context()
	info, err := provider.getUserInfoWithURL(ctx, server.URL, server.URL, "test-token")
	require.NoError(t, err)
	assert.Equal(t, "primary@example.com", info.Email)
}

func TestGitHubOAuth_GetUserInfo_NoVerifiedPrimaryEmail(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			resp := map[string]any{
				"id":    12345,
				"login": "testuser",
				"email": "",
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			resp := []map[string]any{
				{"email": "unverified@example.com", "primary": true, "verified": false},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	provider := &GitHubOAuthProvider{
		clientID:   "test-id",
		httpClient: server.Client(),
	}

	ctx := t.Context()
	_, err := provider.getUserInfoWithURL(ctx, server.URL, server.URL, "test-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no verified primary email")
}
