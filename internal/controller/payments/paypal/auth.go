// Package paypal implements the PaymentProvider interface using PayPal Orders API v2.
package paypal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	sandboxBaseURL    = "https://api-m.sandbox.paypal.com"
	productionBaseURL = "https://api-m.paypal.com"
)

// TokenClient manages OAuth2 access tokens for the PayPal API.
// Tokens are cached and refreshed automatically when expired.
// Thread-safe via sync.Mutex.
type TokenClient struct {
	clientID     string
	clientSecret string
	baseURL      string
	httpClient   *http.Client
	logger       *slog.Logger

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// NewTokenClient creates a TokenClient for PayPal API authentication.
func NewTokenClient(
	clientID, clientSecret, mode string,
	httpClient *http.Client,
	logger *slog.Logger,
) *TokenClient {
	base := sandboxBaseURL
	if mode == "production" {
		base = productionBaseURL
	}
	return &TokenClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      base,
		httpClient:   httpClient,
		logger:       logger.With("component", "paypal-token"),
	}
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// GetAccessToken returns a valid OAuth2 access token, refreshing if expired.
func (tc *TokenClient) GetAccessToken(ctx context.Context) (string, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.accessToken != "" && time.Now().Before(tc.expiresAt) {
		return tc.accessToken, nil
	}
	return tc.refreshTokenLocked(ctx)
}

// refreshTokenLocked fetches a new token. Caller must hold tc.mu.
func (tc *TokenClient) refreshTokenLocked(ctx context.Context) (string, error) {
	req, err := tc.buildTokenRequest(ctx)
	if err != nil {
		return "", err
	}

	resp, err := tc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("paypal token request: %w", err)
	}
	defer resp.Body.Close()

	return tc.parseTokenResponse(resp)
}

func (tc *TokenClient) buildTokenRequest(ctx context.Context) (*http.Request, error) {
	endpoint := tc.baseURL + "/v1/oauth2/token"
	body := url.Values{"grant_type": {"client_credentials"}}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint,
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.SetBasicAuth(tc.clientID, tc.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (tc *TokenClient) parseTokenResponse(resp *http.Response) (string, error) {
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"paypal token request failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var tok tokenResponse
	if err := json.Unmarshal(respBody, &tok); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}

	const expiryBuffer = 60 * time.Second
	tc.accessToken = tok.AccessToken
	tc.expiresAt = time.Now().Add(
		time.Duration(tok.ExpiresIn)*time.Second - expiryBuffer,
	)
	tc.logger.Debug("paypal token refreshed", "expires_in", tok.ExpiresIn)
	return tc.accessToken, nil
}

// BaseURL returns the PayPal API base URL for this client.
func (tc *TokenClient) BaseURL() string {
	return tc.baseURL
}
