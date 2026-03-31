# Billing Phase 6: PayPal Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add PayPal as a payment gateway option, using PayPal Orders API v2 for one-time top-up payments.

**Architecture:** New PayPal provider implements the PaymentProvider interface from Phase 4. Uses PayPal REST API directly (no SDK). OAuth2 client credentials for API auth, webhook signature verification via PayPal API. Plugs into existing PaymentRegistry.

**Tech Stack:** Go 1.26, PayPal Orders API v2, net/http

**Depends on:** Phase 4 (PaymentProvider interface + PaymentRegistry)
**Depended on by:** None

---

## Task 1: Add PayPal OAuth2 Token Client

- [ ] Create `internal/controller/payments/paypal/auth.go` with OAuth2 client credentials flow

**File:** `internal/controller/payments/paypal/auth.go`

### 1a. Create the `paypal` package directory

```bash
mkdir -p internal/controller/payments/paypal
```

### 1b. Implement `TokenClient` struct

```go
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
// mode must be "sandbox" or "production".
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
// Safe for concurrent use.
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
	endpoint := tc.baseURL + "/v1/oauth2/token"
	body := url.Values{"grant_type": {"client_credentials"}}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint,
		strings.NewReader(body.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.SetBasicAuth(tc.clientID, tc.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := tc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("paypal token request: %w", err)
	}
	defer resp.Body.Close()

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

	// Shave 60 seconds to avoid using a token at the boundary.
	tc.accessToken = tok.AccessToken
	tc.expiresAt = time.Now().Add(
		time.Duration(tok.ExpiresIn)*time.Second - 60*time.Second,
	)
	tc.logger.Debug("paypal token refreshed",
		"expires_in", tok.ExpiresIn)
	return tc.accessToken, nil
}

// BaseURL returns the PayPal API base URL for this client.
func (tc *TokenClient) BaseURL() string {
	return tc.baseURL
}
```

**Test:**

```bash
go build ./internal/controller/payments/paypal/...
# Expected: BUILD OK (no tests yet)
```

**Commit:**

```
feat(payments): add PayPal OAuth2 token client

Implement TokenClient for PayPal API authentication using OAuth2 client
credentials flow. Tokens are cached with expiry tracking and refreshed
automatically. Thread-safe via sync.Mutex. Supports sandbox and
production modes.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 2: Add PayPal OAuth2 Token Client Tests

- [ ] Create `internal/controller/payments/paypal/auth_test.go` with table-driven tests

**File:** `internal/controller/payments/paypal/auth_test.go`

### 2a. Implement test file

```go
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
		resp := tokenResponse{
			AccessToken: "test-access-token",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		}
		json.NewEncoder(w).Encode(resp)
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
```

**Test:**

```bash
go test -race -run TestGetAccessToken ./internal/controller/payments/paypal/...
go test -race -run TestNewTokenClient ./internal/controller/payments/paypal/...
# Expected: PASS
```

**Commit:**

```
test(payments): add PayPal token client tests

Table-driven tests for NewTokenClient mode selection, token caching,
concurrent access safety, and API error handling using httptest server.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 3: Implement PayPal Payment Provider

- [ ] Create `internal/controller/payments/paypal/provider.go` implementing `PaymentProvider`

**File:** `internal/controller/payments/paypal/provider.go`

### 3a. Define PayPal API request/response types

```go
package paypal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ProviderConfig holds the configuration for the PayPal payment provider.
type ProviderConfig struct {
	ClientID     string
	ClientSecret string
	Mode         string // "sandbox" or "production"
	WebhookID    string // PayPal webhook ID for signature verification
	ReturnURL    string // URL PayPal redirects to after approval
	CancelURL    string // URL PayPal redirects to on cancellation
	HTTPClient   *http.Client
	Logger       *slog.Logger
}

// Provider implements PaymentProvider using the PayPal Orders API v2.
type Provider struct {
	tokenClient *TokenClient
	webhookID   string
	returnURL   string
	cancelURL   string
	httpClient  *http.Client
	logger      *slog.Logger
}

// NewProvider creates a PayPal PaymentProvider.
func NewProvider(cfg ProviderConfig) *Provider {
	tc := NewTokenClient(
		cfg.ClientID, cfg.ClientSecret, cfg.Mode,
		cfg.HTTPClient, cfg.Logger,
	)
	return &Provider{
		tokenClient: tc,
		webhookID:   cfg.WebhookID,
		returnURL:   cfg.ReturnURL,
		cancelURL:   cfg.CancelURL,
		httpClient:  cfg.HTTPClient,
		logger:      cfg.Logger.With("component", "paypal-provider"),
	}
}
```

### 3b. Define PayPal API types

Add to the same file:

```go
// --- PayPal API types (internal, not exported) ---

type orderRequest struct {
	Intent        string         `json:"intent"`
	PurchaseUnits []purchaseUnit `json:"purchase_units"`
	PaymentSource *paymentSource `json:"payment_source,omitempty"`
}

type purchaseUnit struct {
	ReferenceID string  `json:"reference_id,omitempty"`
	Description string  `json:"description,omitempty"`
	CustomID    string  `json:"custom_id,omitempty"`
	Amount      *amount `json:"amount"`
}

type amount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type paymentSource struct {
	PayPal *paypalSource `json:"paypal"`
}

type paypalSource struct {
	ExperienceContext *experienceContext `json:"experience_context"`
}

type experienceContext struct {
	ReturnURL string `json:"return_url"`
	CancelURL string `json:"cancel_url"`
}

type orderResponse struct {
	ID     string      `json:"id"`
	Status string      `json:"status"`
	Links  []orderLink `json:"links"`
}

type orderLink struct {
	Href   string `json:"href"`
	Rel    string `json:"rel"`
	Method string `json:"method"`
}

type captureResponse struct {
	ID            string         `json:"id"`
	Status        string         `json:"status"`
	PurchaseUnits []capturedUnit `json:"purchase_units"`
}

type capturedUnit struct {
	Payments *capturedPayments `json:"payments"`
}

type capturedPayments struct {
	Captures []captureDetail `json:"captures"`
}

type captureDetail struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	Amount *amount `json:"amount"`
}
```

### 3c. Implement `CreatePaymentSession`

Add to the same file:

```go
// CreatePaymentRequest mirrors the Phase 4 PaymentProvider interface input.
type CreatePaymentRequest struct {
	AccountID   string
	PaymentID   string
	AmountCents int64
	Currency    string
	Description string
}

// PaymentSession is returned after creating a payment session.
type PaymentSession struct {
	GatewayPaymentID string
	ApprovalURL      string
	ExpiresAt        time.Time
}

// CreatePaymentSession creates a PayPal order and returns the approval URL.
func (p *Provider) CreatePaymentSession(
	ctx context.Context,
	req *CreatePaymentRequest,
) (*PaymentSession, error) {
	token, err := p.tokenClient.GetAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get paypal token: %w", err)
	}

	orderReq := orderRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []purchaseUnit{{
			ReferenceID: req.PaymentID,
			Description: req.Description,
			CustomID:    req.AccountID,
			Amount: &amount{
				CurrencyCode: strings.ToUpper(req.Currency),
				Value:        centsToDecimal(req.AmountCents),
			},
		}},
		PaymentSource: &paymentSource{
			PayPal: &paypalSource{
				ExperienceContext: &experienceContext{
					ReturnURL: p.returnURL,
					CancelURL: p.cancelURL,
				},
			},
		},
	}

	body, err := json.Marshal(orderReq)
	if err != nil {
		return nil, fmt.Errorf("marshal order request: %w", err)
	}

	endpoint := p.tokenClient.BaseURL() + "/v2/checkout/orders"
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint,
		strings.NewReader(string(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("build create order request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Prefer", "return=representation")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("paypal create order: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("read create order response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf(
			"paypal create order failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var order orderResponse
	if err := json.Unmarshal(respBody, &order); err != nil {
		return nil, fmt.Errorf("decode order response: %w", err)
	}

	approvalURL := ""
	for _, link := range order.Links {
		if link.Rel == "payer-action" || link.Rel == "approve" {
			approvalURL = link.Href
			break
		}
	}
	if approvalURL == "" {
		return nil, fmt.Errorf("paypal order missing approval link")
	}

	p.logger.Info("paypal order created",
		"order_id", order.ID,
		"payment_id", req.PaymentID,
	)

	return &PaymentSession{
		GatewayPaymentID: order.ID,
		ApprovalURL:      approvalURL,
		ExpiresAt:        time.Now().Add(3 * time.Hour),
	}, nil
}
```

### 3d. Implement `CaptureOrder`

Add to the same file:

```go
// CaptureResult holds the result of capturing a PayPal order.
type CaptureResult struct {
	CaptureID   string
	Status      string
	AmountCents int64
	Currency    string
}

// CaptureOrder finalizes payment for an approved PayPal order.
func (p *Provider) CaptureOrder(
	ctx context.Context, orderID string,
) (*CaptureResult, error) {
	token, err := p.tokenClient.GetAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get paypal token: %w", err)
	}

	endpoint := fmt.Sprintf(
		"%s/v2/checkout/orders/%s/capture",
		p.tokenClient.BaseURL(), orderID,
	)
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build capture request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Prefer", "return=representation")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("paypal capture order: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("read capture response: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf(
			"paypal capture failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var capture captureResponse
	if err := json.Unmarshal(respBody, &capture); err != nil {
		return nil, fmt.Errorf("decode capture response: %w", err)
	}

	if len(capture.PurchaseUnits) == 0 ||
		capture.PurchaseUnits[0].Payments == nil ||
		len(capture.PurchaseUnits[0].Payments.Captures) == 0 {
		return nil, fmt.Errorf("paypal capture response missing capture detail")
	}

	cap := capture.PurchaseUnits[0].Payments.Captures[0]
	cents, err := decimalToCents(cap.Amount.Value)
	if err != nil {
		return nil, fmt.Errorf("parse capture amount: %w", err)
	}

	p.logger.Info("paypal order captured",
		"order_id", orderID,
		"capture_id", cap.ID,
		"status", cap.Status,
	)

	return &CaptureResult{
		CaptureID:   cap.ID,
		Status:      cap.Status,
		AmountCents: cents,
		Currency:    cap.Amount.CurrencyCode,
	}, nil
}
```

### 3e. Implement `GetPaymentStatus`

Add to the same file:

```go
// PaymentStatus holds the current status of a PayPal order.
type PaymentStatus struct {
	Status      string
	AmountCents int64
	Currency    string
}

// GetPaymentStatus retrieves the current status of a PayPal order.
func (p *Provider) GetPaymentStatus(
	ctx context.Context, orderID string,
) (*PaymentStatus, error) {
	token, err := p.tokenClient.GetAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get paypal token: %w", err)
	}

	endpoint := fmt.Sprintf(
		"%s/v2/checkout/orders/%s",
		p.tokenClient.BaseURL(), orderID,
	)
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet, endpoint, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build get order request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("paypal get order: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, fmt.Errorf("read get order response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"paypal get order failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var order orderResponse
	if err := json.Unmarshal(respBody, &order); err != nil {
		return nil, fmt.Errorf("decode order response: %w", err)
	}

	return &PaymentStatus{Status: order.Status}, nil
}
```

### 3f. Add helper functions

Add to the same file:

```go
// centsToDecimal converts an integer cents amount to a decimal string.
// Example: 1050 → "10.50".
func centsToDecimal(cents int64) string {
	whole := cents / 100
	frac := cents % 100
	if frac < 0 {
		frac = -frac
	}
	return fmt.Sprintf("%d.%02d", whole, frac)
}

// decimalToCents converts a decimal string to integer cents.
// Example: "10.50" → 1050.
func decimalToCents(s string) (int64, error) {
	parts := strings.SplitN(s, ".", 2)
	whole := int64(0)
	if _, err := fmt.Sscanf(parts[0], "%d", &whole); err != nil {
		return 0, fmt.Errorf("parse whole part %q: %w", parts[0], err)
	}
	frac := int64(0)
	if len(parts) == 2 {
		fracStr := parts[1]
		// Pad or truncate to 2 decimal places.
		switch len(fracStr) {
		case 0:
			frac = 0
		case 1:
			if _, err := fmt.Sscanf(fracStr, "%d", &frac); err != nil {
				return 0, fmt.Errorf("parse frac part %q: %w", fracStr, err)
			}
			frac *= 10
		default:
			if _, err := fmt.Sscanf(fracStr[:2], "%d", &frac); err != nil {
				return 0, fmt.Errorf("parse frac part %q: %w", fracStr[:2], err)
			}
		}
	}
	return whole*100 + frac, nil
}
```

**Test:**

```bash
go build ./internal/controller/payments/paypal/...
# Expected: BUILD OK
```

**Commit:**

```
feat(payments): implement PayPal payment provider

Implement Provider with CreatePaymentSession (Orders API v2 create),
CaptureOrder (capture after approval), and GetPaymentStatus. Uses
TokenClient for authenticated API calls. All amounts converted between
cents and decimal strings. SSRF-safe HTTP client passed via config.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 4: Add PayPal Provider Unit Tests

- [ ] Create `internal/controller/payments/paypal/provider_test.go`

**File:** `internal/controller/payments/paypal/provider_test.go`

### 4a. Implement provider tests

```go
package paypal

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProvider(t *testing.T, handler http.Handler) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Provider{
		tokenClient: &TokenClient{
			clientID:     "test-id",
			clientSecret: "test-secret",
			baseURL:      srv.URL,
			httpClient:   srv.Client(),
			logger:       testLogger(),
			accessToken:  "pre-cached-token",
			expiresAt:    timeInFuture(),
		},
		returnURL:  "https://example.com/return",
		cancelURL:  "https://example.com/cancel",
		httpClient: srv.Client(),
		logger:     slog.Default(),
	}
}

func timeInFuture() time.Time {
	return time.Now().Add(1 * time.Hour)
}

func TestCreatePaymentSession_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/checkout/orders" {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "Bearer pre-cached-token", r.Header.Get("Authorization"))

			var req orderRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "CAPTURE", req.Intent)
			assert.Equal(t, "10.50", req.PurchaseUnits[0].Amount.Value)
			assert.Equal(t, "USD", req.PurchaseUnits[0].Amount.CurrencyCode)

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(orderResponse{
				ID:     "ORDER-123",
				Status: "CREATED",
				Links: []orderLink{
					{Href: "https://paypal.com/approve/ORDER-123", Rel: "payer-action"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	p := newTestProvider(t, handler)
	session, err := p.CreatePaymentSession(context.Background(), &CreatePaymentRequest{
		AccountID:   "acct-1",
		PaymentID:   "pay-1",
		AmountCents: 1050,
		Currency:    "usd",
		Description: "Credit Top-Up",
	})

	require.NoError(t, err)
	assert.Equal(t, "ORDER-123", session.GatewayPaymentID)
	assert.Equal(t, "https://paypal.com/approve/ORDER-123", session.ApprovalURL)
}

func TestCreatePaymentSession_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"name":"INVALID_REQUEST"}`))
	})

	p := newTestProvider(t, handler)
	_, err := p.CreatePaymentSession(context.Background(), &CreatePaymentRequest{
		AmountCents: 100,
		Currency:    "USD",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
}

func TestCreatePaymentSession_MissingApprovalLink(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(orderResponse{
			ID:     "ORDER-NOLINK",
			Status: "CREATED",
			Links:  []orderLink{},
		})
	})

	p := newTestProvider(t, handler)
	_, err := p.CreatePaymentSession(context.Background(), &CreatePaymentRequest{
		AmountCents: 500,
		Currency:    "USD",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing approval link")
}

func TestCaptureOrder_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/capture")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(captureResponse{
			ID:     "ORDER-456",
			Status: "COMPLETED",
			PurchaseUnits: []capturedUnit{{
				Payments: &capturedPayments{
					Captures: []captureDetail{{
						ID:     "CAP-789",
						Status: "COMPLETED",
						Amount: &amount{CurrencyCode: "USD", Value: "25.00"},
					}},
				},
			}},
		})
	})

	p := newTestProvider(t, handler)
	result, err := p.CaptureOrder(context.Background(), "ORDER-456")

	require.NoError(t, err)
	assert.Equal(t, "CAP-789", result.CaptureID)
	assert.Equal(t, "COMPLETED", result.Status)
	assert.Equal(t, int64(2500), result.AmountCents)
	assert.Equal(t, "USD", result.Currency)
}

func TestCaptureOrder_EmptyCaptures(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(captureResponse{
			ID:            "ORDER-EMPTY",
			Status:        "COMPLETED",
			PurchaseUnits: []capturedUnit{},
		})
	})

	p := newTestProvider(t, handler)
	_, err := p.CaptureOrder(context.Background(), "ORDER-EMPTY")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing capture detail")
}

func TestGetPaymentStatus_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(orderResponse{
			ID:     "ORDER-STATUS",
			Status: "COMPLETED",
		})
	})

	p := newTestProvider(t, handler)
	status, err := p.GetPaymentStatus(context.Background(), "ORDER-STATUS")

	require.NoError(t, err)
	assert.Equal(t, "COMPLETED", status.Status)
}

func TestCentsToDecimal(t *testing.T) {
	tests := []struct {
		name  string
		cents int64
		want  string
	}{
		{"zero", 0, "0.00"},
		{"whole dollars", 1000, "10.00"},
		{"with cents", 1050, "10.50"},
		{"single cent", 1, "0.01"},
		{"large amount", 999999, "9999.99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, centsToDecimal(tt.cents))
		})
	}
}

func TestDecimalToCents(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"whole number", "10", 1000, false},
		{"with cents", "10.50", 1050, false},
		{"single decimal", "10.5", 1050, false},
		{"zero", "0.00", 0, false},
		{"one cent", "0.01", 1, false},
		{"large amount", "9999.99", 999999, false},
		{"invalid", "abc", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decimalToCents(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
```

Note: add `"time"` to the import list in the test file (needed for `timeInFuture`).

**Test:**

```bash
go test -race ./internal/controller/payments/paypal/...
# Expected: PASS
```

**Commit:**

```
test(payments): add PayPal provider unit tests

Table-driven tests for CreatePaymentSession, CaptureOrder,
GetPaymentStatus, centsToDecimal, and decimalToCents. Uses httptest
servers to mock PayPal API responses including success, error, and
edge cases.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 5: Implement PayPal Webhook Signature Verification

- [ ] Create `internal/controller/payments/paypal/webhook.go` with PayPal webhook verification

**File:** `internal/controller/payments/paypal/webhook.go`

### 5a. Implement webhook verification and event parsing

```go
package paypal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WebhookEvent represents a parsed PayPal webhook event.
type WebhookEvent struct {
	ID           string          `json:"id"`
	EventType    string          `json:"event_type"`
	ResourceType string          `json:"resource_type"`
	Resource     json.RawMessage `json:"resource"`
	Summary      string          `json:"summary"`
}

// WebhookResource represents the resource in a webhook event payload.
type WebhookResource struct {
	ID            string  `json:"id"`
	Status        string  `json:"status"`
	CustomID      string  `json:"custom_id"`
	Amount        *amount `json:"amount"`
	SupplementaryData *supplementaryData `json:"supplementary_data,omitempty"`
}

type supplementaryData struct {
	RelatedIDs *relatedIDs `json:"related_ids,omitempty"`
}

type relatedIDs struct {
	OrderID string `json:"order_id"`
}

// verifyWebhookRequest holds the PayPal signature verification request.
type verifyWebhookRequest struct {
	AuthAlgo         string          `json:"auth_algo"`
	CertURL          string          `json:"cert_url"`
	TransmissionID   string          `json:"transmission_id"`
	TransmissionSig  string          `json:"transmission_sig"`
	TransmissionTime string          `json:"transmission_time"`
	WebhookID        string          `json:"webhook_id"`
	WebhookEvent     json.RawMessage `json:"webhook_event"`
}

type verifyWebhookResponse struct {
	VerificationStatus string `json:"verification_status"`
}

// VerifyWebhookSignature verifies a PayPal webhook using the
// POST /v1/notifications/verify-webhook-signature API.
func (p *Provider) VerifyWebhookSignature(
	ctx context.Context,
	headers http.Header,
	body []byte,
) error {
	token, err := p.tokenClient.GetAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("get paypal token: %w", err)
	}

	verifyReq := verifyWebhookRequest{
		AuthAlgo:         headers.Get("Paypal-Auth-Algo"),
		CertURL:          headers.Get("Paypal-Cert-Url"),
		TransmissionID:   headers.Get("Paypal-Transmission-Id"),
		TransmissionSig:  headers.Get("Paypal-Transmission-Sig"),
		TransmissionTime: headers.Get("Paypal-Transmission-Time"),
		WebhookID:        p.webhookID,
		WebhookEvent:     body,
	}

	reqBody, err := json.Marshal(verifyReq)
	if err != nil {
		return fmt.Errorf("marshal verify request: %w", err)
	}

	endpoint := p.tokenClient.BaseURL() +
		"/v1/notifications/verify-webhook-signature"
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint,
		strings.NewReader(string(reqBody)),
	)
	if err != nil {
		return fmt.Errorf("build verify request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("paypal verify webhook: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return fmt.Errorf("read verify response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"paypal verify webhook failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var verifyResp verifyWebhookResponse
	if err := json.Unmarshal(respBody, &verifyResp); err != nil {
		return fmt.Errorf("decode verify response: %w", err)
	}
	if verifyResp.VerificationStatus != "SUCCESS" {
		return fmt.Errorf(
			"paypal webhook verification failed: %s",
			verifyResp.VerificationStatus,
		)
	}

	return nil
}

// ParseWebhookEvent parses a raw webhook body into a WebhookEvent.
func ParseWebhookEvent(body []byte) (*WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("parse webhook event: %w", err)
	}
	return &event, nil
}

// ParseWebhookResource extracts the resource from a webhook event.
func ParseWebhookResource(raw json.RawMessage) (*WebhookResource, error) {
	var res WebhookResource
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("parse webhook resource: %w", err)
	}
	return &res, nil
}
```

**Test:**

```bash
go build ./internal/controller/payments/paypal/...
# Expected: BUILD OK
```

**Commit:**

```
feat(payments): add PayPal webhook signature verification

Implement VerifyWebhookSignature using PayPal's verify-webhook-signature
API. Extract PayPal signature headers and forward them for server-side
verification. Add ParseWebhookEvent and ParseWebhookResource helpers
for processing webhook payloads.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 6: Add PayPal Webhook Verification Tests

- [ ] Create `internal/controller/payments/paypal/webhook_test.go`

**File:** `internal/controller/payments/paypal/webhook_test.go`

### 6a. Implement webhook tests

```go
package paypal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyWebhookSignature_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/notifications/verify-webhook-signature" {
			var req verifyWebhookRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "SHA256withRSA", req.AuthAlgo)
			assert.Equal(t, "webhook-123", req.WebhookID)
			assert.NotEmpty(t, req.TransmissionID)

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(verifyWebhookResponse{
				VerificationStatus: "SUCCESS",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	p := &Provider{
		tokenClient: &TokenClient{
			baseURL:     srv.URL,
			httpClient:  srv.Client(),
			logger:      testLogger(),
			accessToken: "test-token",
			expiresAt:   timeInFuture(),
		},
		webhookID:  "webhook-123",
		httpClient: srv.Client(),
		logger:     testLogger(),
	}

	headers := http.Header{}
	headers.Set("Paypal-Auth-Algo", "SHA256withRSA")
	headers.Set("Paypal-Cert-Url", "https://paypal.com/cert")
	headers.Set("Paypal-Transmission-Id", "txn-001")
	headers.Set("Paypal-Transmission-Sig", "sig-abc")
	headers.Set("Paypal-Transmission-Time", "2026-01-01T00:00:00Z")

	err := p.VerifyWebhookSignature(
		context.Background(),
		headers,
		[]byte(`{"event_type":"CHECKOUT.ORDER.APPROVED"}`),
	)
	require.NoError(t, err)
}

func TestVerifyWebhookSignature_Failure(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(verifyWebhookResponse{
			VerificationStatus: "FAILURE",
		})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	p := &Provider{
		tokenClient: &TokenClient{
			baseURL:     srv.URL,
			httpClient:  srv.Client(),
			logger:      testLogger(),
			accessToken: "test-token",
			expiresAt:   timeInFuture(),
		},
		webhookID:  "webhook-123",
		httpClient: srv.Client(),
		logger:     testLogger(),
	}

	err := p.VerifyWebhookSignature(
		context.Background(),
		http.Header{},
		[]byte(`{}`),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
}

func TestVerifyWebhookSignature_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	p := &Provider{
		tokenClient: &TokenClient{
			baseURL:     srv.URL,
			httpClient:  srv.Client(),
			logger:      testLogger(),
			accessToken: "test-token",
			expiresAt:   timeInFuture(),
		},
		webhookID:  "webhook-123",
		httpClient: srv.Client(),
		logger:     testLogger(),
	}

	err := p.VerifyWebhookSignature(
		context.Background(),
		http.Header{},
		[]byte(`{}`),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestParseWebhookEvent(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantType  string
		wantErr   bool
	}{
		{
			"valid event",
			`{"id":"EVT-1","event_type":"CHECKOUT.ORDER.APPROVED","resource_type":"checkout-order","resource":{}}`,
			"CHECKOUT.ORDER.APPROVED",
			false,
		},
		{
			"capture completed",
			`{"id":"EVT-2","event_type":"PAYMENT.CAPTURE.COMPLETED","resource_type":"capture","resource":{"id":"CAP-1"}}`,
			"PAYMENT.CAPTURE.COMPLETED",
			false,
		},
		{"invalid json", `{bad`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ParseWebhookEvent([]byte(tt.body))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, event.EventType)
		})
	}
}

func TestParseWebhookResource(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantID  string
		wantErr bool
	}{
		{
			"valid resource",
			`{"id":"CAP-123","status":"COMPLETED","custom_id":"acct-1","amount":{"currency_code":"USD","value":"10.00"}}`,
			"CAP-123",
			false,
		},
		{"invalid json", `{bad`, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ParseWebhookResource(json.RawMessage(tt.raw))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, res.ID)
		})
	}
}
```

**Test:**

```bash
go test -race -run TestVerifyWebhook ./internal/controller/payments/paypal/...
go test -race -run TestParseWebhook ./internal/controller/payments/paypal/...
# Expected: PASS
```

**Commit:**

```
test(payments): add PayPal webhook verification tests

Tests for VerifyWebhookSignature (success, failure, API error) and
ParseWebhookEvent/ParseWebhookResource with table-driven cases
including valid events, capture completed, and invalid JSON.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 7: Implement PayPal HandleWebhook Method

- [ ] Add `HandleWebhook` to `Provider` in `internal/controller/payments/paypal/provider.go`

**File:** `internal/controller/payments/paypal/provider.go`

### 7a. Add `WebhookResult` type and `HandleWebhook` method

Append to the end of `provider.go`:

```go
// WebhookResult holds the outcome of processing a PayPal webhook.
type WebhookResult struct {
	EventType      string
	OrderID        string
	CaptureID      string
	AccountID      string
	AmountCents    int64
	Currency       string
	Status         string
	IdempotencyKey string
}

// HandleWebhook verifies and processes an inbound PayPal webhook.
// Returns a WebhookResult for the billing service to apply idempotently.
// Only processes PAYMENT.CAPTURE.COMPLETED events; all others are
// acknowledged but return nil (no action needed).
func (p *Provider) HandleWebhook(
	ctx context.Context,
	headers http.Header,
	body []byte,
) (*WebhookResult, error) {
	if err := p.VerifyWebhookSignature(ctx, headers, body); err != nil {
		return nil, fmt.Errorf("verify paypal webhook: %w", err)
	}

	event, err := ParseWebhookEvent(body)
	if err != nil {
		return nil, fmt.Errorf("parse paypal webhook event: %w", err)
	}

	p.logger.Info("paypal webhook received",
		"event_type", event.EventType,
		"event_id", event.ID,
	)

	if event.EventType != "PAYMENT.CAPTURE.COMPLETED" {
		p.logger.Debug("ignoring non-capture event",
			"event_type", event.EventType)
		return nil, nil
	}

	resource, err := ParseWebhookResource(event.Resource)
	if err != nil {
		return nil, fmt.Errorf("parse capture resource: %w", err)
	}

	if resource.Status != "COMPLETED" {
		p.logger.Warn("capture not completed",
			"capture_id", resource.ID,
			"status", resource.Status)
		return nil, nil
	}

	cents := int64(0)
	currency := ""
	if resource.Amount != nil {
		cents, err = decimalToCents(resource.Amount.Value)
		if err != nil {
			return nil, fmt.Errorf("parse capture amount: %w", err)
		}
		currency = resource.Amount.CurrencyCode
	}

	orderID := ""
	if resource.SupplementaryData != nil &&
		resource.SupplementaryData.RelatedIDs != nil {
		orderID = resource.SupplementaryData.RelatedIDs.OrderID
	}

	return &WebhookResult{
		EventType:      event.EventType,
		OrderID:        orderID,
		CaptureID:      resource.ID,
		AccountID:      resource.CustomID,
		AmountCents:    cents,
		Currency:       currency,
		Status:         resource.Status,
		IdempotencyKey: "paypal:capture:" + resource.ID,
	}, nil
}
```

**Test:**

```bash
go build ./internal/controller/payments/paypal/...
# Expected: BUILD OK
```

**Commit:**

```
feat(payments): add PayPal HandleWebhook method

HandleWebhook verifies signature via PayPal API, parses the event,
and processes PAYMENT.CAPTURE.COMPLETED events. Returns WebhookResult
with idempotency key (paypal:capture:{capture_id}) for the billing
service to credit the account. Non-capture events are acknowledged
but return nil.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 8: Add HandleWebhook Tests

- [ ] Add `HandleWebhook` tests to `internal/controller/payments/paypal/webhook_test.go`

**File:** `internal/controller/payments/paypal/webhook_test.go`

### 8a. Add HandleWebhook tests

Append to the existing `webhook_test.go`:

```go
func TestHandleWebhook_CaptureCompleted(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/notifications/verify-webhook-signature" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(verifyWebhookResponse{
				VerificationStatus: "SUCCESS",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	p := &Provider{
		tokenClient: &TokenClient{
			baseURL:     srv.URL,
			httpClient:  srv.Client(),
			logger:      testLogger(),
			accessToken: "test-token",
			expiresAt:   timeInFuture(),
		},
		webhookID:  "webhook-123",
		httpClient: srv.Client(),
		logger:     testLogger(),
	}

	body := `{
		"id": "EVT-001",
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource_type": "capture",
		"resource": {
			"id": "CAP-ABC",
			"status": "COMPLETED",
			"custom_id": "acct-42",
			"amount": {"currency_code": "USD", "value": "50.00"},
			"supplementary_data": {"related_ids": {"order_id": "ORDER-XYZ"}}
		}
	}`

	result, err := p.HandleWebhook(context.Background(), http.Header{}, []byte(body))
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "PAYMENT.CAPTURE.COMPLETED", result.EventType)
	assert.Equal(t, "ORDER-XYZ", result.OrderID)
	assert.Equal(t, "CAP-ABC", result.CaptureID)
	assert.Equal(t, "acct-42", result.AccountID)
	assert.Equal(t, int64(5000), result.AmountCents)
	assert.Equal(t, "USD", result.Currency)
	assert.Equal(t, "paypal:capture:CAP-ABC", result.IdempotencyKey)
}

func TestHandleWebhook_NonCaptureEvent(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(verifyWebhookResponse{
			VerificationStatus: "SUCCESS",
		})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	p := &Provider{
		tokenClient: &TokenClient{
			baseURL:     srv.URL,
			httpClient:  srv.Client(),
			logger:      testLogger(),
			accessToken: "test-token",
			expiresAt:   timeInFuture(),
		},
		webhookID:  "webhook-123",
		httpClient: srv.Client(),
		logger:     testLogger(),
	}

	body := `{
		"id": "EVT-002",
		"event_type": "CHECKOUT.ORDER.APPROVED",
		"resource_type": "checkout-order",
		"resource": {"id": "ORDER-123"}
	}`

	result, err := p.HandleWebhook(context.Background(), http.Header{}, []byte(body))
	require.NoError(t, err)
	assert.Nil(t, result, "non-capture events should return nil result")
}

func TestHandleWebhook_VerificationFailure(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(verifyWebhookResponse{
			VerificationStatus: "FAILURE",
		})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	p := &Provider{
		tokenClient: &TokenClient{
			baseURL:     srv.URL,
			httpClient:  srv.Client(),
			logger:      testLogger(),
			accessToken: "test-token",
			expiresAt:   timeInFuture(),
		},
		webhookID:  "webhook-123",
		httpClient: srv.Client(),
		logger:     testLogger(),
	}

	_, err := p.HandleWebhook(context.Background(), http.Header{}, []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification failed")
}

func TestHandleWebhook_CapturePending(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(verifyWebhookResponse{
			VerificationStatus: "SUCCESS",
		})
	})

	srv := httptest.NewServer(handler)
	defer srv.Close()

	p := &Provider{
		tokenClient: &TokenClient{
			baseURL:     srv.URL,
			httpClient:  srv.Client(),
			logger:      testLogger(),
			accessToken: "test-token",
			expiresAt:   timeInFuture(),
		},
		webhookID:  "webhook-123",
		httpClient: srv.Client(),
		logger:     testLogger(),
	}

	body := `{
		"id": "EVT-003",
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource_type": "capture",
		"resource": {
			"id": "CAP-PEND",
			"status": "PENDING",
			"amount": {"currency_code": "USD", "value": "10.00"}
		}
	}`

	result, err := p.HandleWebhook(context.Background(), http.Header{}, []byte(body))
	require.NoError(t, err)
	assert.Nil(t, result, "pending captures should return nil")
}
```

**Test:**

```bash
go test -race -run TestHandleWebhook ./internal/controller/payments/paypal/...
# Expected: PASS
```

**Commit:**

```
test(payments): add PayPal HandleWebhook tests

Tests for capture completed (with full assertion on idempotency key,
amounts, and IDs), non-capture events returning nil, verification
failure, and pending capture status ignored.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 9: Register PayPal Provider in PaymentRegistry

- [ ] Wire PayPal provider into the payment registry in `internal/controller/dependencies.go`

**File:** `internal/controller/dependencies.go`

### 9a. Add PayPal provider registration

In `InitializeServices`, after the existing payment gateway registrations (Stripe), add conditional PayPal registration:

```go
	// Register PayPal payment provider if configured
	if s.config.PayPal.ClientID.Value() != "" && s.config.PayPal.ClientSecret.Value() != "" {
		paypalProvider := paypal.NewProvider(paypal.ProviderConfig{
			ClientID:     s.config.PayPal.ClientID.Value(),
			ClientSecret: s.config.PayPal.ClientSecret.Value(),
			Mode:         s.config.PayPal.Mode,
			WebhookID:    s.config.PayPal.WebhookID,
			ReturnURL:    s.config.PayPal.ReturnURL,
			CancelURL:    s.config.PayPal.CancelURL,
			HTTPClient:   tasks.DefaultHTTPClient(),
			Logger:       s.logger,
		})
		paymentRegistry.Register("paypal", paypalProvider)
		s.paypalProvider = paypalProvider
		s.logger.Info("PayPal payment provider registered",
			"mode", s.config.PayPal.Mode)
	}
```

### 9b. Add `paypalProvider` field to `Server` struct

In `internal/controller/server.go`, add the field to the `Server` struct:

```go
	paypalProvider *paypal.Provider
```

### 9c. Add PayPal config fields

In `internal/shared/config/config.go`, add the following fields to the existing `PayPalConfig` struct (these were defined in Phase 1 but may need `WebhookID`, `ReturnURL`, and `CancelURL` fields):

```go
	WebhookID string `yaml:"webhook_id"` // PayPal webhook ID for signature verification
	ReturnURL string `yaml:"return_url"` // redirect URL after payment approval
	CancelURL string `yaml:"cancel_url"` // redirect URL on payment cancellation
```

Add corresponding env var overrides in `applyEnvOverridesPayments`:

```go
	if v := os.Getenv("PAYPAL_WEBHOOK_ID"); v != "" {
		cfg.PayPal.WebhookID = v
	}
	if v := os.Getenv("PAYPAL_RETURN_URL"); v != "" {
		cfg.PayPal.ReturnURL = v
	}
	if v := os.Getenv("PAYPAL_CANCEL_URL"); v != "" {
		cfg.PayPal.CancelURL = v
	}
```

### 9d. Add import for the paypal package

Add to the imports in `dependencies.go`:

```go
	"github.com/AbuGosok/VirtueStack/internal/controller/payments/paypal"
```

**Test:**

```bash
make build-controller
# Expected: BUILD OK
```

**Commit:**

```
feat(payments): wire PayPal provider into payment registry

Register PayPal provider conditionally when PAYPAL_CLIENT_ID and
PAYPAL_CLIENT_SECRET are configured. Add PAYPAL_WEBHOOK_ID,
PAYPAL_RETURN_URL, and PAYPAL_CANCEL_URL config fields. Uses
SSRF-safe DefaultHTTPClient for all PayPal API calls.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 10: Add PayPal Webhook HTTP Handler

- [ ] Create webhook HTTP handler at `POST /api/v1/webhooks/paypal`

**File:** `internal/controller/api/webhooks/paypal_handler.go`

### 10a. Create the webhooks package and PayPal handler

```bash
mkdir -p internal/controller/api/webhooks
```

```go
// Package webhooks provides HTTP handlers for third-party payment webhooks.
package webhooks

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments/paypal"
	"github.com/gin-gonic/gin"
)

// BillingCreditor applies a payment credit to a billing account.
// Implemented by the billing service.
type BillingCreditor interface {
	CreditFromPayment(
		ctx context.Context,
		accountID string,
		amountCents int64,
		currency string,
		gateway string,
		gatewayPaymentID string,
		idempotencyKey string,
	) error
}

// PayPalWebhookHandler handles PayPal webhook events.
type PayPalWebhookHandler struct {
	provider *paypal.Provider
	creditor BillingCreditor
	logger   *slog.Logger
}

// NewPayPalWebhookHandler creates a new PayPal webhook handler.
func NewPayPalWebhookHandler(
	provider *paypal.Provider,
	creditor BillingCreditor,
	logger *slog.Logger,
) *PayPalWebhookHandler {
	return &PayPalWebhookHandler{
		provider: provider,
		creditor: creditor,
		logger:   logger.With("component", "paypal-webhook-handler"),
	}
}

// HandleWebhook processes inbound PayPal webhook POST requests.
// This endpoint is unauthenticated — signature verification is
// performed via the PayPal API.
func (h *PayPalWebhookHandler) HandleWebhook(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		h.logger.Error("failed to read webhook body", "error", err)
		c.Status(http.StatusBadRequest)
		return
	}

	result, err := h.provider.HandleWebhook(
		c.Request.Context(),
		c.Request.Header,
		body,
	)
	if err != nil {
		h.logger.Error("paypal webhook processing failed", "error", err)
		c.Status(http.StatusBadRequest)
		return
	}

	// Non-actionable event (e.g., ORDER.APPROVED) — acknowledge it.
	if result == nil {
		c.Status(http.StatusOK)
		return
	}

	// Credit the billing account idempotently.
	if err := h.creditor.CreditFromPayment(
		c.Request.Context(),
		result.AccountID,
		result.AmountCents,
		result.Currency,
		"paypal",
		result.CaptureID,
		result.IdempotencyKey,
	); err != nil {
		h.logger.Error("failed to credit account from paypal payment",
			"error", err,
			"capture_id", result.CaptureID,
			"account_id", result.AccountID,
		)
		c.Status(http.StatusInternalServerError)
		return
	}

	h.logger.Info("paypal payment credited",
		"capture_id", result.CaptureID,
		"account_id", result.AccountID,
		"amount_cents", result.AmountCents,
	)
	c.Status(http.StatusOK)
}
```

Note: Add `"context"` to the import list.

### 10b. Register the webhook route in `server.go`

In `RegisterAPIRoutes`, add the PayPal webhook route (unauthenticated):

```go
	// PayPal webhook — unauthenticated, verified via PayPal API
	if s.paypalProvider != nil {
		paypalWebhookHandler := webhooks.NewPayPalWebhookHandler(
			s.paypalProvider,
			s.billingService, // BillingCreditor interface
			s.logger,
		)
		webhookGroup := api.Group("/webhooks")
		webhookGroup.POST("/paypal", paypalWebhookHandler.HandleWebhook)
	}
```

**Test:**

```bash
make build-controller
# Expected: BUILD OK
```

**Commit:**

```
feat(payments): add PayPal webhook HTTP handler

Register POST /api/v1/webhooks/paypal as unauthenticated endpoint.
Reads raw body, delegates to PayPal provider for signature verification
and event parsing, then credits billing account via BillingCreditor
interface. Body limited to 1MB. Non-actionable events return 200 OK.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 11: Add PayPal Return URL Handler (Customer API)

- [ ] Add customer-facing endpoint for PayPal redirect return

**File:** `internal/controller/api/customer/billing_paypal.go`

### 11a. Implement PayPal return handler

When PayPal redirects back after payment approval, the customer portal calls this endpoint to trigger capture:

```go
package customer

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/gin-gonic/gin"
)

// CapturePayPalPaymentRequest holds the order ID from PayPal redirect.
type CapturePayPalPaymentRequest struct {
	OrderID string `json:"order_id" validate:"required"`
}

// CapturePayPalPayment captures an approved PayPal order and credits
// the customer's billing account.
func (h *CustomerHandler) CapturePayPalPayment(c *gin.Context) {
	var req CapturePayPalPaymentRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		return
	}

	customerID := middleware.GetUserID(c)

	result, err := h.billingService.CapturePayPalOrder(
		c.Request.Context(), customerID, req.OrderID,
	)
	if err != nil {
		h.logger.Error("failed to capture paypal order",
			"error", err,
			"customer_id", customerID,
			"order_id", req.OrderID,
			"correlation_id", middleware.GetCorrelationID(c),
		)
		middleware.RespondWithError(c, http.StatusBadRequest,
			"CAPTURE_FAILED", "Failed to capture PayPal payment")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: result})
}
```

Note: Add `"github.com/AbuGosok/VirtueStack/internal/controller/models"` to the import list.

### 11b. Register the route

In `internal/controller/api/customer/routes.go`, inside the billing routes section (which should exist from Phase 4):

```go
	billingGroup.POST("/payments/paypal/capture", handler.CapturePayPalPayment)
```

**Test:**

```bash
make build-controller
# Expected: BUILD OK
```

**Commit:**

```
feat(billing): add PayPal capture endpoint for customer API

Add POST /api/v1/customer/billing/payments/paypal/capture for handling
PayPal redirect return. Customer submits the order_id after PayPal
approval; backend captures the order and credits the billing account.
JWT-authenticated, customer-scoped.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 12: Add PayPal Option to Customer Billing UI

- [ ] Update the customer billing top-up form to include PayPal as a payment gateway option

**File:** `webui/customer/app/(dashboard)/billing/components/top-up-form.tsx`

### 12a. Add PayPal gateway selection

In the top-up form component (which should exist from Phase 4 Stripe UI), add PayPal as a selectable payment method:

```tsx
// Add to the gateway options list (alongside existing Stripe option)
const GATEWAY_OPTIONS = [
  { value: "stripe", label: "Credit/Debit Card", icon: CreditCardIcon },
  { value: "paypal", label: "PayPal", icon: PayPalIcon },
] as const;
```

### 12b. Handle PayPal redirect flow

When the user selects PayPal and submits:

```tsx
async function handlePayPalTopUp(amountCents: number, currency: string) {
  const response = await apiClient.post<{
    data: { gateway_payment_id: string; approval_url: string };
  }>("/customer/billing/payments/topup", {
    amount_cents: amountCents,
    currency,
    gateway: "paypal",
  });

  // Redirect to PayPal approval page
  window.location.href = response.data.data.approval_url;
}
```

### 12c. Handle PayPal return

Create a return page at `webui/customer/app/(dashboard)/billing/paypal-return/page.tsx`:

```tsx
"use client";

import { useSearchParams, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { apiClient } from "@/lib/api-client";

export default function PayPalReturnPage() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const [status, setStatus] = useState<"processing" | "success" | "error">(
    "processing"
  );

  useEffect(() => {
    const token = searchParams.get("token");
    if (!token) {
      setStatus("error");
      return;
    }

    apiClient
      .post("/customer/billing/payments/paypal/capture", {
        order_id: token,
      })
      .then(() => {
        setStatus("success");
        setTimeout(() => router.push("/billing"), 2000);
      })
      .catch(() => setStatus("error"));
  }, [searchParams, router]);

  return (
    <div className="flex items-center justify-center min-h-[400px]">
      {status === "processing" && <p>Processing your PayPal payment...</p>}
      {status === "success" && (
        <p className="text-green-600">
          Payment successful! Redirecting to billing...
        </p>
      )}
      {status === "error" && (
        <p className="text-red-600">
          Payment failed. Please contact support.
        </p>
      )}
    </div>
  );
}
```

### 12d. Build and verify

```bash
cd webui/customer && npm run type-check && npm run build
# Expected: BUILD OK
```

**Commit:**

```
feat(customer-ui): add PayPal as payment gateway option

Add PayPal selection to billing top-up form alongside Stripe.
Implements PayPal redirect flow with return page that captures
the order and credits the account. Shows processing/success/error
states during capture.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 13: Update `.env.example` with PayPal Config

- [ ] Add PayPal-specific environment variables to `.env.example`

**File:** `.env.example`

### 13a. Add PayPal section

Add the following to the Payment Gateways section of `.env.example`:

```bash
# --- PayPal ---
# PayPal Orders API v2 credentials.
# Get from https://developer.paypal.com/dashboard/applications
# PAYPAL_CLIENT_ID=           # Required when using PayPal
# PAYPAL_CLIENT_SECRET=       # Required when using PayPal
# PAYPAL_MODE=sandbox         # "sandbox" or "production"
# PAYPAL_WEBHOOK_ID=          # PayPal webhook ID for signature verification
# PAYPAL_RETURN_URL=https://your-domain.com/billing/paypal-return
# PAYPAL_CANCEL_URL=https://your-domain.com/billing
```

**Test:**

```bash
grep -c "PAYPAL" .env.example
# Expected: 6 (the six PayPal variables)
```

**Commit:**

```
docs: add PayPal environment variables to .env.example

Document PAYPAL_CLIENT_ID, PAYPAL_CLIENT_SECRET, PAYPAL_MODE,
PAYPAL_WEBHOOK_ID, PAYPAL_RETURN_URL, and PAYPAL_CANCEL_URL with
descriptions and links to PayPal developer dashboard.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 14: Final Build and Test Verification

- [ ] Build and test all components to verify Phase 6 integration

### 14a. Build controller

```bash
make build-controller
# Expected: BUILD OK
```

### 14b. Run unit tests

```bash
make test
# Expected: PASS
```

### 14c. Run PayPal package tests with race detector

```bash
go test -race ./internal/controller/payments/paypal/...
# Expected: PASS
```

### 14d. Run linter (if golangci-lint is installed)

```bash
make lint
# Expected: PASS (or only pre-existing warnings)
```

### 14e. Build customer frontend

```bash
cd webui/customer && npm run type-check && npm run build
# Expected: BUILD OK
```

**Commit:**

```
chore: verify Phase 6 PayPal integration builds and tests pass

All PayPal provider tests pass with race detector. Controller builds
successfully. Customer frontend type-checks and builds.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Summary of Deliverables

| # | Deliverable | Files Changed |
|---|------------|---------------|
| 1 | PayPal OAuth2 token client | `internal/controller/payments/paypal/auth.go` |
| 2 | Token client tests | `internal/controller/payments/paypal/auth_test.go` |
| 3 | PayPal payment provider | `internal/controller/payments/paypal/provider.go` |
| 4 | Provider tests | `internal/controller/payments/paypal/provider_test.go` |
| 5 | Webhook signature verification | `internal/controller/payments/paypal/webhook.go` |
| 6 | Webhook verification tests | `internal/controller/payments/paypal/webhook_test.go` |
| 7 | HandleWebhook method | `internal/controller/payments/paypal/provider.go` |
| 8 | HandleWebhook tests | `internal/controller/payments/paypal/webhook_test.go` |
| 9 | Registry wiring + config | `internal/controller/dependencies.go`, `internal/controller/server.go`, `internal/shared/config/config.go` |
| 10 | Webhook HTTP handler | `internal/controller/api/webhooks/paypal_handler.go`, `internal/controller/server.go` |
| 11 | Customer capture endpoint | `internal/controller/api/customer/billing_paypal.go`, `internal/controller/api/customer/routes.go` |
| 12 | Customer billing UI | `webui/customer/app/(dashboard)/billing/components/top-up-form.tsx`, `webui/customer/app/(dashboard)/billing/paypal-return/page.tsx` |
| 13 | Env documentation | `.env.example` |
| 14 | Final verification | (no files — build/test confirmation) |

## Environment Variables Introduced

| Variable | Type | Default | Required When |
|----------|------|---------|---------------|
| `PAYPAL_CLIENT_ID` | Secret | — | PayPal in use |
| `PAYPAL_CLIENT_SECRET` | Secret | — | PayPal in use |
| `PAYPAL_MODE` | string | `sandbox` | — |
| `PAYPAL_WEBHOOK_ID` | string | — | PayPal webhooks in use |
| `PAYPAL_RETURN_URL` | string | — | PayPal in use |
| `PAYPAL_CANCEL_URL` | string | — | PayPal in use |

## API Endpoints Introduced

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/v1/webhooks/paypal` | None (PayPal signature) | PayPal webhook receiver |
| POST | `/api/v1/customer/billing/payments/paypal/capture` | JWT | Capture approved PayPal order |
