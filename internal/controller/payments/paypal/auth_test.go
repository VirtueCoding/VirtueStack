package paypal

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.Default()
}

func TestNewTokenClient(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		wantBaseURL string
	}{
		{"sandbox mode", "sandbox", sandboxBaseURL},
		{"production mode", "production", productionBaseURL},
		{"unknown defaults to sandbox", "invalid", sandboxBaseURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := NewTokenClient("id", "secret", tt.mode, http.DefaultClient, testLogger())
			assert.Equal(t, tt.wantBaseURL, tc.BaseURL())
		})
	}
}

func TestGetAccessToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/oauth2/token", r.URL.Path)

		user, pass, ok := r.BasicAuth()
		require.True(t, ok, "expected basic auth")
		assert.Equal(t, "test-id", user)
		assert.Equal(t, "test-secret", pass)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "test-access-token",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer srv.Close()

	tc := &TokenClient{
		clientID:     "test-id",
		clientSecret: "test-secret",
		baseURL:      srv.URL,
		httpClient:   srv.Client(),
		logger:       testLogger(),
	}

	token, err := tc.GetAccessToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-access-token", token)
}

func TestGetAccessToken_CachesToken(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "cached-token",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer srv.Close()

	tc := &TokenClient{
		clientID:     "id",
		clientSecret: "secret",
		baseURL:      srv.URL,
		httpClient:   srv.Client(),
		logger:       testLogger(),
	}

	ctx := context.Background()
	tok1, err := tc.GetAccessToken(ctx)
	require.NoError(t, err)
	tok2, err := tc.GetAccessToken(ctx)
	require.NoError(t, err)

	assert.Equal(t, tok1, tok2)
	assert.Equal(t, int32(1), callCount.Load(), "token should be cached")
}

func TestGetAccessToken_ConcurrentAccess(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "concurrent-token",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer srv.Close()

	tc := &TokenClient{
		clientID:     "id",
		clientSecret: "secret",
		baseURL:      srv.URL,
		httpClient:   srv.Client(),
		logger:       testLogger(),
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tok, err := tc.GetAccessToken(context.Background())
			require.NoError(t, err)
			assert.Equal(t, "concurrent-token", tok)
		}()
	}
	wg.Wait()
}

func TestGetAccessToken_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer srv.Close()

	tc := &TokenClient{
		clientID:     "bad-id",
		clientSecret: "bad-secret",
		baseURL:      srv.URL,
		httpClient:   srv.Client(),
		logger:       testLogger(),
	}

	_, err := tc.GetAccessToken(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 401")
}
