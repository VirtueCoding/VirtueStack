# Billing Phase 8: Native Registration + Google/GitHub OAuth — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Google and GitHub OAuth login/registration for customers, with PKCE flow, account linking, and graceful handling of OAuth-only users (no password required).

**Architecture:** New `customer_oauth_links` table tracks OAuth connections. PKCE flow prevents code interception. OAuth service handles provider-specific authorization URLs and token exchange. Account linking allows multiple auth methods per customer. Frontend adds OAuth buttons and callback handling.

**Tech Stack:** Go 1.26, OAuth 2.0 + PKCE, Google/GitHub APIs, React 19, Next.js

**Depends on:** Phase 0 (billing_provider field on customer), Phase 1 (OAuth config flags)
**Depended on by:** None

---

## Task 1: Migration 000078 — OAuth Schema Changes

- [ ] Create migration to make `password_hash` nullable, add `auth_provider`, and create `customer_oauth_links` table

**Files to create:**
- `migrations/000078_oauth_customer_links.up.sql`
- `migrations/000078_oauth_customer_links.down.sql`

### 1a. Up migration

**File:** `migrations/000078_oauth_customer_links.up.sql`

```sql
SET lock_timeout = '5s';

-- Make password_hash nullable for OAuth-only users (who authenticate via
-- Google/GitHub and may never set a local password).
ALTER TABLE customers ALTER COLUMN password_hash DROP NOT NULL;

-- Track how the account was originally created. Existing rows default to
-- 'local' (email + password). OAuth-created accounts use 'google'/'github'.
ALTER TABLE customers ADD COLUMN IF NOT EXISTS auth_provider VARCHAR(20) NOT NULL DEFAULT 'local'
    CHECK (auth_provider IN ('local', 'google', 'github'));

-- OAuth provider links. One customer can link multiple providers.
-- A single provider account can only be linked to one VirtueStack customer.
CREATE TABLE customer_oauth_links (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id             UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    provider                VARCHAR(20) NOT NULL CHECK (provider IN ('google', 'github')),
    provider_user_id        VARCHAR(255) NOT NULL,
    email                   VARCHAR(255),
    display_name            VARCHAR(255),
    avatar_url              VARCHAR(500),
    access_token_encrypted  BYTEA,
    refresh_token_encrypted BYTEA,
    token_expires_at        TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_user_id)
);

CREATE INDEX idx_oauth_links_customer ON customer_oauth_links(customer_id);
```

### 1b. Down migration

**File:** `migrations/000078_oauth_customer_links.down.sql`

```sql
SET lock_timeout = '5s';

DROP TABLE IF EXISTS customer_oauth_links;

ALTER TABLE customers DROP COLUMN IF EXISTS auth_provider;

-- Restore NOT NULL. Existing OAuth-only rows will need a dummy hash first.
-- This is a destructive rollback — OAuth-only accounts will be broken.
UPDATE customers SET password_hash = '' WHERE password_hash IS NULL;
ALTER TABLE customers ALTER COLUMN password_hash SET NOT NULL;
```

**Verify:**
```bash
head -5 migrations/000078_oauth_customer_links.up.sql
head -5 migrations/000078_oauth_customer_links.down.sql
```

**Commit:**
```
feat(oauth): add OAuth schema — nullable password_hash, auth_provider, oauth_links (migration 000078)

Makes password_hash nullable for OAuth-only customers. Adds auth_provider
column (local/google/github) with default 'local'. Creates customer_oauth_links
table with encrypted token storage and unique provider+user constraint.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 2: OAuth Link Model

- [ ] Create `OAuthLink` model and constants

**File to create:** `internal/controller/models/oauth_link.go`

```go
package models

import "time"

// OAuth provider constants.
const (
	OAuthProviderGoogle = "google"
	OAuthProviderGitHub = "github"
)

// Auth provider constants for the auth_provider column on Customer.
const (
	AuthProviderLocal  = "local"
	AuthProviderGoogle = "google"
	AuthProviderGitHub = "github"
)

// ValidOAuthProviders lists the supported OAuth provider identifiers.
var ValidOAuthProviders = []string{OAuthProviderGoogle, OAuthProviderGitHub}

// OAuthLink represents a customer's linked OAuth provider account.
type OAuthLink struct {
	ID                    string     `json:"id" db:"id"`
	CustomerID            string     `json:"customer_id" db:"customer_id"`
	Provider              string     `json:"provider" db:"provider"`
	ProviderUserID        string     `json:"-" db:"provider_user_id"`
	Email                 string     `json:"email,omitempty" db:"email"`
	DisplayName           string     `json:"display_name,omitempty" db:"display_name"`
	AvatarURL             string     `json:"avatar_url,omitempty" db:"avatar_url"`
	AccessTokenEncrypted  []byte     `json:"-" db:"access_token_encrypted"`
	RefreshTokenEncrypted []byte     `json:"-" db:"refresh_token_encrypted"`
	TokenExpiresAt        *time.Time `json:"-" db:"token_expires_at"`
	CreatedAt             time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at" db:"updated_at"`
}

// OAuthUserInfo holds the user profile data returned by an OAuth provider.
type OAuthUserInfo struct {
	ProviderUserID string
	Email          string
	Name           string
	AvatarURL      string
}

// OAuthCallbackRequest holds the data sent from the frontend after OAuth redirect.
type OAuthCallbackRequest struct {
	Code         string `json:"code" validate:"required"`
	CodeVerifier string `json:"code_verifier" validate:"required,min=43,max=128"`
	RedirectURI  string `json:"redirect_uri" validate:"required,url"`
	State        string `json:"state" validate:"required"`
}

// OAuthAuthorizeRequest holds the query parameters for the authorize endpoint.
type OAuthAuthorizeRequest struct {
	CodeChallenge string `form:"code_challenge" validate:"required,min=43,max=128"`
	State         string `form:"state" validate:"required,min=16,max=128"`
	RedirectURI   string `form:"redirect_uri" validate:"required,url"`
}

// IsValidOAuthProvider checks whether the given provider string is supported.
func IsValidOAuthProvider(provider string) bool {
	for _, p := range ValidOAuthProviders {
		if p == provider {
			return true
		}
	}
	return false
}
```

**Verify:**
```bash
go build ./internal/controller/models/...
```

**Commit:**
```
feat(models): add OAuthLink model and OAuth constants

Defines OAuthLink struct, OAuthUserInfo, callback/authorize request
types, provider constants, and IsValidOAuthProvider helper.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 3: Update Customer Model for OAuth

- [ ] Make `PasswordHash` nullable and add `AuthProvider` field to Customer

**File:** `internal/controller/models/customer.go`

### 3a. Change `PasswordHash` to pointer type and add `AuthProvider`

Replace the Customer struct fields (lines 15-27):

```go
// Customer represents a customer account as stored in the database.
type Customer struct {
	ID                   string   `json:"id" db:"id"`
	Email                string   `json:"email" db:"email"`
	PasswordHash         *string  `json:"-" db:"password_hash"`
	Name                 string   `json:"name" db:"name"`
	Phone                *string  `json:"phone,omitempty" db:"phone"`
	WHMCSClientID        *int     `json:"whmcs_client_id,omitempty" db:"whmcs_client_id"`
	AuthProvider         string   `json:"auth_provider" db:"auth_provider"`
	TOTPSecretEncrypted  *string  `json:"-" db:"totp_secret_encrypted"`
	TOTPEnabled          bool     `json:"totp_enabled" db:"totp_enabled"`
	TOTPBackupCodesHash  []string `json:"-" db:"totp_backup_codes_hash"`
	TOTPBackupCodesShown bool     `json:"-" db:"totp_backup_codes_shown"`
	Status               string   `json:"status" db:"status"`
	Timestamps
}
```

### 3b. Update all callers that assign `PasswordHash` as a string

In `internal/controller/api/customer/registration.go`, update the `Register` handler to assign via pointer:

Replace:
```go
	customer := &models.Customer{
		Email:        email,
		PasswordHash: passwordHash,
		Name:         name,
		Status:       status,
	}
```

With:
```go
	customer := &models.Customer{
		Email:        email,
		PasswordHash: &passwordHash,
		Name:         name,
		AuthProvider: models.AuthProviderLocal,
		Status:       status,
	}
```

### 3c. Update auth service password verification

In `internal/controller/services/auth_service.go` (and `auth_service_login.go`), update `verifyLoginCredentials` to handle nullable `PasswordHash`:

Where `PasswordHash` is compared, add a nil check:

```go
// In verifyLoginCredentials — if customer.PasswordHash is nil,
// the customer is OAuth-only and cannot log in with password.
if customer.PasswordHash == nil {
	s.incrementFailedLogin(ctx, email)
	return nil, sharederrors.ErrUnauthorized
}
```

### 3d. Grep and fix all remaining `PasswordHash` references

Search the entire codebase for `.PasswordHash` usages and update them to handle the `*string` type. Key locations:
- `internal/controller/repository/customer_repo.go` — `Create`, `UpdatePassword` queries
- `internal/controller/api/provisioning/customers.go` — sets PasswordHash on WHMCS-created customers
- `internal/controller/api/provisioning/password.go` — password set/reset
- `internal/controller/api/customer/auth.go` — `ChangePassword` handler

For each, change direct string assignment to pointer assignment (`&hash`) and dereference reads (`*customer.PasswordHash`) with nil guards.

**Verify:**
```bash
go build ./internal/controller/...
go test -race ./internal/controller/models/...
go test -race ./internal/controller/services/...
```

**Commit:**
```
feat(models): make Customer.PasswordHash nullable for OAuth-only accounts

Changes PasswordHash from string to *string. Adds AuthProvider field
(local/google/github). Updates registration, auth service, provisioning
handlers, and repository to handle nullable password hash. OAuth-only
customers (nil password) cannot use password login.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 4: OAuth Link Repository

- [ ] Create repository for `customer_oauth_links` CRUD

**File to create:** `internal/controller/repository/oauth_link_repo.go`

```go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const oauthLinkColumns = `id, customer_id, provider, provider_user_id,
	email, display_name, avatar_url, access_token_encrypted,
	refresh_token_encrypted, token_expires_at, created_at, updated_at`

// OAuthLinkRepository handles database operations for customer_oauth_links.
type OAuthLinkRepository struct {
	db *pgxpool.Pool
}

// NewOAuthLinkRepository creates a new OAuthLinkRepository.
func NewOAuthLinkRepository(db *pgxpool.Pool) *OAuthLinkRepository {
	return &OAuthLinkRepository{db: db}
}

// Create inserts a new OAuth link.
func (r *OAuthLinkRepository) Create(
	ctx context.Context, link *models.OAuthLink,
) (*models.OAuthLink, error) {
	query := `INSERT INTO customer_oauth_links (
		customer_id, provider, provider_user_id, email,
		display_name, avatar_url, access_token_encrypted,
		refresh_token_encrypted, token_expires_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	RETURNING ` + oauthLinkColumns

	row := r.db.QueryRow(ctx, query,
		link.CustomerID, link.Provider, link.ProviderUserID,
		link.Email, link.DisplayName, link.AvatarURL,
		link.AccessTokenEncrypted, link.RefreshTokenEncrypted,
		link.TokenExpiresAt,
	)
	return scanOAuthLink(row)
}

// GetByProviderUserID looks up an OAuth link by provider and provider user ID.
func (r *OAuthLinkRepository) GetByProviderUserID(
	ctx context.Context, provider, providerUserID string,
) (*models.OAuthLink, error) {
	query := `SELECT ` + oauthLinkColumns + `
		FROM customer_oauth_links
		WHERE provider = $1 AND provider_user_id = $2`
	row := r.db.QueryRow(ctx, query, provider, providerUserID)
	return scanOAuthLink(row)
}

// GetByCustomerID returns all OAuth links for a customer.
func (r *OAuthLinkRepository) GetByCustomerID(
	ctx context.Context, customerID string,
) ([]*models.OAuthLink, error) {
	query := `SELECT ` + oauthLinkColumns + `
		FROM customer_oauth_links
		WHERE customer_id = $1
		ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, query, customerID)
	if err != nil {
		return nil, fmt.Errorf("query oauth links: %w", err)
	}
	defer rows.Close()

	var links []*models.OAuthLink
	for rows.Next() {
		link, err := scanOAuthLinkRow(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// Delete removes an OAuth link by customer and provider.
func (r *OAuthLinkRepository) Delete(
	ctx context.Context, customerID, provider string,
) error {
	query := `DELETE FROM customer_oauth_links
		WHERE customer_id = $1 AND provider = $2`
	tag, err := r.db.Exec(ctx, query, customerID, provider)
	if err != nil {
		return fmt.Errorf("delete oauth link: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sharederrors.ErrNotFound
	}
	return nil
}

// UpdateTokens updates the stored OAuth tokens for a link.
func (r *OAuthLinkRepository) UpdateTokens(
	ctx context.Context, id string,
	accessTokenEnc, refreshTokenEnc []byte,
	expiresAt *time.Time,
) error {
	query := `UPDATE customer_oauth_links
		SET access_token_encrypted = $1,
			refresh_token_encrypted = $2,
			token_expires_at = $3,
			updated_at = NOW()
		WHERE id = $4`
	tag, err := r.db.Exec(ctx, query,
		accessTokenEnc, refreshTokenEnc, expiresAt, id)
	if err != nil {
		return fmt.Errorf("update oauth tokens: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sharederrors.ErrNotFound
	}
	return nil
}

// CountByCustomerID returns the number of OAuth links for a customer.
func (r *OAuthLinkRepository) CountByCustomerID(
	ctx context.Context, customerID string,
) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM customer_oauth_links WHERE customer_id = $1`,
		customerID,
	).Scan(&count)
	return count, err
}

func scanOAuthLink(row pgx.Row) (*models.OAuthLink, error) {
	var link models.OAuthLink
	err := row.Scan(
		&link.ID, &link.CustomerID, &link.Provider,
		&link.ProviderUserID, &link.Email, &link.DisplayName,
		&link.AvatarURL, &link.AccessTokenEncrypted,
		&link.RefreshTokenEncrypted, &link.TokenExpiresAt,
		&link.CreatedAt, &link.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, sharederrors.ErrNotFound
		}
		return nil, fmt.Errorf("scan oauth link: %w", err)
	}
	return &link, nil
}

func scanOAuthLinkRow(rows pgx.Rows) (*models.OAuthLink, error) {
	var link models.OAuthLink
	err := rows.Scan(
		&link.ID, &link.CustomerID, &link.Provider,
		&link.ProviderUserID, &link.Email, &link.DisplayName,
		&link.AvatarURL, &link.AccessTokenEncrypted,
		&link.RefreshTokenEncrypted, &link.TokenExpiresAt,
		&link.CreatedAt, &link.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan oauth link row: %w", err)
	}
	return &link, nil
}
```

**Verify:**
```bash
go build ./internal/controller/repository/...
```

**Commit:**
```
feat(repository): add OAuthLinkRepository for customer_oauth_links

Implements Create, GetByProviderUserID, GetByCustomerID, Delete,
UpdateTokens, and CountByCustomerID. Uses parameterized queries
with pgx/v5.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 5: OAuth Link Repository Tests

- [ ] Write table-driven tests for OAuthLinkRepository

**File to create:** `internal/controller/repository/oauth_link_repo_test.go`

Write table-driven unit tests covering:

1. **Create** — happy path, duplicate `(provider, provider_user_id)` returns conflict error
2. **GetByProviderUserID** — found, not found returns `ErrNotFound`
3. **GetByCustomerID** — returns all links, empty list for unknown customer
4. **Delete** — happy path, not found returns `ErrNotFound`
5. **UpdateTokens** — happy path, not found returns `ErrNotFound`
6. **CountByCustomerID** — zero count, correct count after inserts

Use mock `pgxpool.Pool` or a `mockDB` struct with function fields matching the repository pattern established in existing repo tests.

**Verify:**
```bash
go test -race -run TestOAuthLink ./internal/controller/repository/...
```

**Commit:**
```
test(repository): add OAuthLinkRepository tests

Table-driven tests with testify for Create, GetByProviderUserID,
GetByCustomerID, Delete, UpdateTokens, and CountByCustomerID.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 6: OAuth Provider Abstraction

- [ ] Create OAuth provider interface and Google/GitHub implementations

**File to create:** `internal/controller/services/oauth_provider.go`

```go
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	utilpkg "github.com/AbuGosok/VirtueStack/internal/shared/util"
)

// OAuthProvider abstracts a single OAuth 2.0 identity provider.
type OAuthProvider interface {
	// Name returns the provider identifier ("google" or "github").
	Name() string

	// AuthorizationURL builds the OAuth consent redirect URL with PKCE.
	AuthorizationURL(codeChallenge, state, redirectURI string) string

	// ExchangeCode exchanges an authorization code for tokens.
	ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI string) (*OAuthTokens, error)

	// GetUserInfo fetches the authenticated user's profile using the access token.
	GetUserInfo(ctx context.Context, accessToken string) (*models.OAuthUserInfo, error)
}

// OAuthTokens holds the token response from a provider.
type OAuthTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	TokenType    string
}

// ssrfSafeTransport returns an http.Transport that blocks requests to
// private and metadata IP addresses, preventing SSRF attacks.
func ssrfSafeTransport() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address %q: %w", addr, err)
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("dns lookup %q: %w", host, err)
			}
			for _, ip := range ips {
				if utilpkg.IsPrivateIP(ip) {
					return nil, fmt.Errorf("blocked SSRF: %s resolves to private IP %s", host, ip)
				}
			}
			dialer := &net.Dialer{Timeout: 10 * time.Second}
			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

// ssrfSafeClient returns an *http.Client configured to block SSRF.
func ssrfSafeClient() *http.Client {
	return &http.Client{
		Transport: ssrfSafeTransport(),
		Timeout:   30 * time.Second,
	}
}
```

**File to create:** `internal/controller/services/oauth_google.go`

```go
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
)

// GoogleOAuthProvider implements OAuthProvider for Google.
type GoogleOAuthProvider struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

// NewGoogleOAuthProvider creates a Google OAuth provider.
func NewGoogleOAuthProvider(clientID, clientSecret string) *GoogleOAuthProvider {
	return &GoogleOAuthProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   ssrfSafeClient(),
	}
}

func (g *GoogleOAuthProvider) Name() string { return models.OAuthProviderGoogle }

func (g *GoogleOAuthProvider) AuthorizationURL(
	codeChallenge, state, redirectURI string,
) string {
	params := url.Values{
		"client_id":             {g.clientID},
		"redirect_uri":         {redirectURI},
		"response_type":        {"code"},
		"scope":                {"openid email profile"},
		"state":                {state},
		"code_challenge":       {codeChallenge},
		"code_challenge_method": {"S256"},
		"access_type":          {"offline"},
		"prompt":               {"consent"},
	}
	return googleAuthURL + "?" + params.Encode()
}

func (g *GoogleOAuthProvider) ExchangeCode(
	ctx context.Context, code, codeVerifier, redirectURI string,
) (*OAuthTokens, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, googleTokenURL,
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google token exchange: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google token exchange failed (status %d): %s",
			resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	return &OAuthTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		TokenType:    tokenResp.TokenType,
	}, nil
}

func (g *GoogleOAuthProvider) GetUserInfo(
	ctx context.Context, accessToken string,
) (*models.OAuthUserInfo, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, googleUserInfoURL, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google userinfo: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read userinfo response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google userinfo failed (status %d): %s",
			resp.StatusCode, string(body))
	}

	var profile struct {
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, fmt.Errorf("parse userinfo: %w", err)
	}

	return &models.OAuthUserInfo{
		ProviderUserID: profile.Sub,
		Email:          profile.Email,
		Name:           profile.Name,
		AvatarURL:      profile.Picture,
	}, nil
}
```

**File to create:** `internal/controller/services/oauth_github.go`

```go
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

const (
	githubAuthURL     = "https://github.com/login/oauth/authorize"
	githubTokenURL    = "https://github.com/login/oauth/access_token"
	githubUserURL     = "https://api.github.com/user"
	githubEmailsURL   = "https://api.github.com/user/emails"
)

// GitHubOAuthProvider implements OAuthProvider for GitHub.
type GitHubOAuthProvider struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

// NewGitHubOAuthProvider creates a GitHub OAuth provider.
func NewGitHubOAuthProvider(clientID, clientSecret string) *GitHubOAuthProvider {
	return &GitHubOAuthProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   ssrfSafeClient(),
	}
}

func (g *GitHubOAuthProvider) Name() string { return models.OAuthProviderGitHub }

func (g *GitHubOAuthProvider) AuthorizationURL(
	codeChallenge, state, redirectURI string,
) string {
	params := url.Values{
		"client_id":             {g.clientID},
		"redirect_uri":         {redirectURI},
		"scope":                {"read:user user:email"},
		"state":                {state},
		"code_challenge":       {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return githubAuthURL + "?" + params.Encode()
}

func (g *GitHubOAuthProvider) ExchangeCode(
	ctx context.Context, code, codeVerifier, redirectURI string,
) (*OAuthTokens, error) {
	data := url.Values{
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, githubTokenURL,
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github token exchange: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github token exchange failed (status %d): %s",
			resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("github oauth error: %s — %s",
			tokenResp.Error, tokenResp.ErrorDesc)
	}

	return &OAuthTokens{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
	}, nil
}

func (g *GitHubOAuthProvider) GetUserInfo(
	ctx context.Context, accessToken string,
) (*models.OAuthUserInfo, error) {
	userReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet, githubUserURL, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build user request: %w", err)
	}
	userReq.Header.Set("Authorization", "Bearer "+accessToken)
	userReq.Header.Set("Accept", "application/vnd.github+json")

	userResp, err := g.httpClient.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("github user api: %w", err)
	}
	defer userResp.Body.Close()

	userBody, err := io.ReadAll(io.LimitReader(userResp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read user response: %w", err)
	}
	if userResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github user api failed (status %d): %s",
			userResp.StatusCode, string(userBody))
	}

	var user struct {
		ID        int    `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(userBody, &user); err != nil {
		return nil, fmt.Errorf("parse user response: %w", err)
	}

	email := user.Email
	if email == "" {
		fetchedEmail, err := g.fetchPrimaryEmail(ctx, accessToken)
		if err != nil {
			return nil, fmt.Errorf("fetch github primary email: %w", err)
		}
		email = fetchedEmail
	}

	name := user.Name
	if name == "" {
		name = user.Login
	}

	return &models.OAuthUserInfo{
		ProviderUserID: strconv.Itoa(user.ID),
		Email:          email,
		Name:           name,
		AvatarURL:      user.AvatarURL,
	}, nil
}

// fetchPrimaryEmail retrieves the verified primary email from GitHub
// when the /user endpoint does not include one (private email setting).
func (g *GitHubOAuthProvider) fetchPrimaryEmail(
	ctx context.Context, accessToken string,
) (string, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, githubEmailsURL, nil,
	)
	if err != nil {
		return "", fmt.Errorf("build emails request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github emails api: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read emails response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github emails api failed (status %d): %s",
			resp.StatusCode, string(body))
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", fmt.Errorf("parse emails response: %w", err)
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	return "", fmt.Errorf("no verified primary email found on GitHub account")
}
```

**Verify:**
```bash
go build ./internal/controller/services/...
```

**Commit:**
```
feat(services): add OAuth provider interface with Google and GitHub implementations

OAuthProvider interface abstracts authorization URL generation, code
exchange, and user info fetching. Both providers use SSRF-safe HTTP
clients. GitHub provider fetches primary email via /user/emails when
the public profile email is private.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 7: OAuth Service — Core Logic

- [ ] Create OAuth service with account linking, login, and registration logic

**File to create:** `internal/controller/services/oauth_service.go`

```go
package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// OAuthServiceConfig holds dependencies for the OAuth service.
type OAuthServiceConfig struct {
	OAuthLinkRepo      *repository.OAuthLinkRepository
	CustomerRepo       *repository.CustomerRepository
	AuthService        *AuthService
	Providers          map[string]OAuthProvider
	EncryptionKey      string
	AllowRegistration  bool
	PrimaryBillingProv string
	Logger             *slog.Logger
}

// OAuthService handles OAuth authentication and account linking.
type OAuthService struct {
	oauthLinkRepo      *repository.OAuthLinkRepository
	customerRepo       *repository.CustomerRepository
	authService        *AuthService
	providers          map[string]OAuthProvider
	encryptionKey      string
	allowRegistration  bool
	primaryBillingProv string
	logger             *slog.Logger
}

// NewOAuthService creates a new OAuthService.
func NewOAuthService(cfg OAuthServiceConfig) *OAuthService {
	return &OAuthService{
		oauthLinkRepo:      cfg.OAuthLinkRepo,
		customerRepo:       cfg.CustomerRepo,
		authService:        cfg.AuthService,
		providers:          cfg.Providers,
		encryptionKey:      cfg.EncryptionKey,
		allowRegistration:  cfg.AllowRegistration,
		primaryBillingProv: cfg.PrimaryBillingProv,
		logger:             cfg.Logger.With("component", "oauth-service"),
	}
}

// GetAuthorizationURL generates the OAuth provider redirect URL with PKCE.
func (s *OAuthService) GetAuthorizationURL(
	provider, codeChallenge, state, redirectURI string,
) (string, error) {
	p, err := s.getProvider(provider)
	if err != nil {
		return "", err
	}
	return p.AuthorizationURL(codeChallenge, state, redirectURI), nil
}

// HandleCallback processes the OAuth callback after user authorization.
// Returns auth tokens for an existing or newly created customer.
func (s *OAuthService) HandleCallback(
	ctx context.Context,
	provider, code, codeVerifier, redirectURI, ipAddress, userAgent string,
) (*models.AuthTokens, string, error) {
	p, err := s.getProvider(provider)
	if err != nil {
		return nil, "", err
	}

	tokens, err := p.ExchangeCode(ctx, code, codeVerifier, redirectURI)
	if err != nil {
		return nil, "", fmt.Errorf("exchange oauth code: %w", err)
	}

	userInfo, err := p.GetUserInfo(ctx, tokens.AccessToken)
	if err != nil {
		return nil, "", fmt.Errorf("get oauth user info: %w", err)
	}
	userInfo.Email = strings.ToLower(strings.TrimSpace(userInfo.Email))

	if userInfo.Email == "" {
		return nil, "", sharederrors.NewValidationError(
			"email", "OAuth provider did not return an email address")
	}

	customer, err := s.resolveCustomer(ctx, provider, userInfo)
	if err != nil {
		return nil, "", err
	}

	if err := s.storeOAuthTokens(ctx, customer.ID, provider, userInfo, tokens); err != nil {
		s.logger.Warn("failed to store oauth tokens",
			"customer_id", customer.ID, "provider", provider, "error", err)
	}

	s.authService.EnforceCustomerSessionLimit(ctx, customer.ID)
	authTokens, refreshToken, err := s.authService.CreateLoginSession(
		ctx, customer.ID, "customer", "",
		ipAddress, userAgent, CustomerRefreshTokenDuration,
	)
	if err != nil {
		return nil, "", fmt.Errorf("create oauth session: %w", err)
	}

	s.logger.Info("oauth login successful",
		"customer_id", customer.ID, "provider", provider)
	return authTokens, refreshToken, nil
}

// resolveCustomer finds or creates a customer from OAuth user info.
// Implements the account linking strategy from billplan.md §8.5.
func (s *OAuthService) resolveCustomer(
	ctx context.Context, provider string, info *models.OAuthUserInfo,
) (*models.Customer, error) {
	link, err := s.oauthLinkRepo.GetByProviderUserID(
		ctx, provider, info.ProviderUserID)
	if err == nil && link != nil {
		return s.customerRepo.GetByID(ctx, link.CustomerID)
	}
	if err != nil && !sharederrors.Is(err, sharederrors.ErrNotFound) {
		return nil, fmt.Errorf("lookup oauth link: %w", err)
	}

	existing, err := s.customerRepo.GetByEmail(ctx, info.Email)
	if err != nil && !sharederrors.Is(err, sharederrors.ErrNotFound) {
		return nil, fmt.Errorf("lookup customer by email: %w", err)
	}

	if existing != nil {
		return s.linkExistingCustomer(ctx, provider, info, existing)
	}

	return s.createOAuthCustomer(ctx, provider, info)
}

// linkExistingCustomer handles the case where a customer with matching
// email exists but has no OAuth link for this provider.
func (s *OAuthService) linkExistingCustomer(
	ctx context.Context,
	provider string,
	info *models.OAuthUserInfo,
	customer *models.Customer,
) (*models.Customer, error) {
	switch customer.Status {
	case models.CustomerStatusPendingVerification:
		return nil, sharederrors.NewValidationError(
			"email", "Please verify your email address first")
	case models.CustomerStatusSuspended, models.CustomerStatusDeleted:
		return nil, sharederrors.ErrForbidden
	case models.CustomerStatusActive:
		// Continue to linking logic below
	default:
		return nil, fmt.Errorf("unexpected customer status: %s", customer.Status)
	}

	if customer.WHMCSClientID != nil {
		return nil, sharederrors.NewValidationError("email",
			"This email is linked to a billing account. "+
				"Log in with your password and link OAuth from account settings.")
	}

	oauthLink := &models.OAuthLink{
		CustomerID:     customer.ID,
		Provider:       provider,
		ProviderUserID: info.ProviderUserID,
		Email:          info.Email,
		DisplayName:    info.Name,
		AvatarURL:      info.AvatarURL,
	}
	if _, err := s.oauthLinkRepo.Create(ctx, oauthLink); err != nil {
		return nil, fmt.Errorf("auto-link oauth: %w", err)
	}

	s.logger.Info("auto-linked oauth account",
		"customer_id", customer.ID, "provider", provider)
	return customer, nil
}

// createOAuthCustomer creates a new customer from OAuth user info.
func (s *OAuthService) createOAuthCustomer(
	ctx context.Context, provider string, info *models.OAuthUserInfo,
) (*models.Customer, error) {
	if !s.allowRegistration {
		return nil, sharederrors.NewValidationError("registration",
			"Self-registration is disabled. Contact your provider.")
	}

	customer := &models.Customer{
		Email:        info.Email,
		Name:         info.Name,
		AuthProvider: provider,
		Status:       models.CustomerStatusActive,
	}

	created, err := s.customerRepo.Create(ctx, customer)
	if err != nil {
		return nil, fmt.Errorf("create oauth customer: %w", err)
	}

	oauthLink := &models.OAuthLink{
		CustomerID:     created.ID,
		Provider:       provider,
		ProviderUserID: info.ProviderUserID,
		Email:          info.Email,
		DisplayName:    info.Name,
		AvatarURL:      info.AvatarURL,
	}
	if _, err := s.oauthLinkRepo.Create(ctx, oauthLink); err != nil {
		return nil, fmt.Errorf("create oauth link for new customer: %w", err)
	}

	s.logger.Info("created oauth customer",
		"customer_id", created.ID, "provider", provider)
	return created, nil
}

// storeOAuthTokens encrypts and persists OAuth access/refresh tokens.
func (s *OAuthService) storeOAuthTokens(
	ctx context.Context,
	customerID, provider string,
	info *models.OAuthUserInfo,
	tokens *OAuthTokens,
) error {
	link, err := s.oauthLinkRepo.GetByProviderUserID(
		ctx, provider, info.ProviderUserID)
	if err != nil {
		return fmt.Errorf("get link for token storage: %w", err)
	}

	var accessEnc, refreshEnc []byte

	if tokens.AccessToken != "" {
		encrypted, err := crypto.Encrypt(tokens.AccessToken, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("encrypt access token: %w", err)
		}
		accessEnc = []byte(encrypted)
	}

	if tokens.RefreshToken != "" {
		encrypted, err := crypto.Encrypt(tokens.RefreshToken, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("encrypt refresh token: %w", err)
		}
		refreshEnc = []byte(encrypted)
	}

	var expiresAt *time.Time
	if !tokens.ExpiresAt.IsZero() {
		expiresAt = &tokens.ExpiresAt
	}

	return s.oauthLinkRepo.UpdateTokens(ctx, link.ID, accessEnc, refreshEnc, expiresAt)
}

// LinkAccount links an OAuth provider to an authenticated customer's account.
// Used from account settings when a logged-in customer wants to add OAuth.
func (s *OAuthService) LinkAccount(
	ctx context.Context,
	customerID, provider, code, codeVerifier, redirectURI string,
) error {
	p, err := s.getProvider(provider)
	if err != nil {
		return err
	}

	tokens, err := p.ExchangeCode(ctx, code, codeVerifier, redirectURI)
	if err != nil {
		return fmt.Errorf("exchange code for link: %w", err)
	}

	userInfo, err := p.GetUserInfo(ctx, tokens.AccessToken)
	if err != nil {
		return fmt.Errorf("get user info for link: %w", err)
	}

	existing, err := s.oauthLinkRepo.GetByProviderUserID(
		ctx, provider, userInfo.ProviderUserID)
	if err == nil && existing != nil {
		if existing.CustomerID == customerID {
			return nil
		}
		return sharederrors.NewValidationError("provider",
			"This OAuth account is already linked to another customer")
	}

	oauthLink := &models.OAuthLink{
		CustomerID:     customerID,
		Provider:       provider,
		ProviderUserID: userInfo.ProviderUserID,
		Email:          userInfo.Email,
		DisplayName:    userInfo.Name,
		AvatarURL:      userInfo.AvatarURL,
	}
	if _, err := s.oauthLinkRepo.Create(ctx, oauthLink); err != nil {
		return fmt.Errorf("create link: %w", err)
	}

	if err := s.storeOAuthTokens(ctx, customerID, provider, userInfo, tokens); err != nil {
		s.logger.Warn("failed to store tokens after link",
			"customer_id", customerID, "provider", provider, "error", err)
	}

	s.logger.Info("linked oauth account",
		"customer_id", customerID, "provider", provider)
	return nil
}

// UnlinkAccount removes an OAuth provider from a customer's account.
// Prevents unlinking the last auth method when no password is set.
func (s *OAuthService) UnlinkAccount(
	ctx context.Context, customerID, provider string,
) error {
	customer, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		return fmt.Errorf("get customer for unlink: %w", err)
	}

	linkCount, err := s.oauthLinkRepo.CountByCustomerID(ctx, customerID)
	if err != nil {
		return fmt.Errorf("count oauth links: %w", err)
	}

	hasPassword := customer.PasswordHash != nil && *customer.PasswordHash != ""
	if !hasPassword && linkCount <= 1 {
		return sharederrors.NewValidationError("provider",
			"Cannot unlink last OAuth provider without a password set. "+
				"Set a password first.")
	}

	return s.oauthLinkRepo.Delete(ctx, customerID, provider)
}

// GetLinkedAccounts returns all OAuth links for a customer.
func (s *OAuthService) GetLinkedAccounts(
	ctx context.Context, customerID string,
) ([]*models.OAuthLink, error) {
	return s.oauthLinkRepo.GetByCustomerID(ctx, customerID)
}

func (s *OAuthService) getProvider(name string) (OAuthProvider, error) {
	p, ok := s.providers[name]
	if !ok {
		return nil, sharederrors.NewValidationError(
			"provider", fmt.Sprintf("OAuth provider %q is not enabled", name))
	}
	return p, nil
}
```

**Verify:**
```bash
go build ./internal/controller/services/...
```

**Commit:**
```
feat(services): add OAuthService with account linking and login flow

Implements HandleCallback (code exchange → user info → resolve/create
customer → session), LinkAccount (JWT-protected linking from settings),
UnlinkAccount (prevents orphaned accounts), and GetLinkedAccounts.
Account linking follows the billplan §8.5 strategy: auto-link for
non-WHMCS accounts, reject WHMCS-linked accounts (must link via settings).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 8: Expose AuthService Methods for OAuth

- [ ] Export `CreateLoginSession` and `EnforceCustomerSessionLimit` from AuthService

**File:** `internal/controller/services/auth_service.go`

The existing `createLoginSession` and `enforceCustomerSessionLimit` methods are lowercase (unexported). The OAuthService needs to call them. Add exported wrappers:

```go
// CreateLoginSession creates a new authenticated session for a customer.
// Exported for use by OAuthService.
func (s *AuthService) CreateLoginSession(
	ctx context.Context,
	userID, userType, role, ipAddress, userAgent string,
	refreshDuration time.Duration,
) (*models.AuthTokens, string, error) {
	return s.createLoginSession(ctx, userID, userType, role,
		ipAddress, userAgent, refreshDuration)
}

// EnforceCustomerSessionLimit removes oldest sessions exceeding the limit.
// Exported for use by OAuthService.
func (s *AuthService) EnforceCustomerSessionLimit(
	ctx context.Context, customerID string,
) {
	s.enforceCustomerSessionLimit(ctx, customerID)
}
```

**Verify:**
```bash
go build ./internal/controller/services/...
```

**Commit:**
```
feat(auth): export CreateLoginSession and EnforceCustomerSessionLimit

Adds exported wrappers around the existing unexported methods so that
OAuthService can create sessions and enforce limits without duplicating logic.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 9: OAuth HTTP Handlers

- [ ] Create OAuth handler endpoints for authorize, callback, and account linking

**File to create:** `internal/controller/api/customer/oauth.go`

```go
package customer

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// OAuthAuthorize handles GET /auth/oauth/:provider/authorize.
// Redirects the browser to the OAuth provider's consent page with PKCE.
func (h *CustomerHandler) OAuthAuthorize(c *gin.Context) {
	provider := c.Param("provider")
	if !models.IsValidOAuthProvider(provider) {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_PROVIDER", "Unsupported OAuth provider")
		return
	}

	var req models.OAuthAuthorizeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"VALIDATION_ERROR", "Missing code_challenge, state, or redirect_uri")
		return
	}

	if err := middleware.ValidateStruct(&req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest,
			"VALIDATION_ERROR", "Invalid parameters")
		return
	}

	authURL, err := h.oauthService.GetAuthorizationURL(
		provider, req.CodeChallenge, req.State, req.RedirectURI)
	if err != nil {
		h.logger.Error("failed to generate oauth authorization URL",
			"provider", provider, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusBadRequest,
			"OAUTH_ERROR", err.Error())
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// OAuthCallback handles POST /auth/oauth/:provider/callback.
// Exchanges the authorization code for tokens, resolves the customer,
// and returns JWT tokens.
func (h *CustomerHandler) OAuthCallback(c *gin.Context) {
	provider := c.Param("provider")
	if !models.IsValidOAuthProvider(provider) {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_PROVIDER", "Unsupported OAuth provider")
		return
	}

	var req models.OAuthCallbackRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest,
			"VALIDATION_ERROR", "Invalid callback request")
		return
	}

	authTokens, refreshToken, err := h.oauthService.HandleCallback(
		c.Request.Context(), provider,
		req.Code, req.CodeVerifier, req.RedirectURI,
		c.ClientIP(), c.Request.UserAgent(),
	)
	if err != nil {
		h.logger.Error("oauth callback failed",
			"provider", provider, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		var valErr sharederrors.ValidationError
		if errors.As(err, &valErr) {
			middleware.RespondWithError(c, http.StatusBadRequest,
				"OAUTH_ERROR", valErr.Error())
			return
		}
		if sharederrors.Is(err, sharederrors.ErrForbidden) {
			middleware.RespondWithError(c, http.StatusForbidden,
				"ACCOUNT_SUSPENDED", "Account is suspended or deleted")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"OAUTH_ERROR", "OAuth authentication failed")
		return
	}

	middleware.SetAuthCookies(c, authTokens.AccessToken,
		refreshToken, "customer")

	c.JSON(http.StatusOK, models.Response{Data: authTokens})
}

// ListOAuthLinks handles GET /account/oauth.
// Returns the customer's linked OAuth providers.
func (h *CustomerHandler) ListOAuthLinks(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	links, err := h.oauthService.GetLinkedAccounts(c.Request.Context(), customerID)
	if err != nil {
		h.logger.Error("failed to list oauth links",
			"customer_id", customerID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"OAUTH_ERROR", "Failed to list linked accounts")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: links})
}

// LinkOAuthAccount handles POST /account/oauth/:provider/link.
// Links a new OAuth provider to the authenticated customer's account.
func (h *CustomerHandler) LinkOAuthAccount(c *gin.Context) {
	provider := c.Param("provider")
	if !models.IsValidOAuthProvider(provider) {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_PROVIDER", "Unsupported OAuth provider")
		return
	}

	var req models.OAuthCallbackRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest,
			"VALIDATION_ERROR", "Invalid request")
		return
	}

	customerID := middleware.GetUserID(c)
	err := h.oauthService.LinkAccount(
		c.Request.Context(), customerID, provider,
		req.Code, req.CodeVerifier, req.RedirectURI,
	)
	if err != nil {
		h.logger.Error("failed to link oauth account",
			"customer_id", customerID, "provider", provider,
			"error", err, "correlation_id", middleware.GetCorrelationID(c))

		var valErr sharederrors.ValidationError
		if errors.As(err, &valErr) {
			middleware.RespondWithError(c, http.StatusConflict,
				"LINK_FAILED", valErr.Error())
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"LINK_FAILED", "Failed to link OAuth account")
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Data: gin.H{"message": "OAuth account linked successfully"},
	})
}

// UnlinkOAuthAccount handles DELETE /account/oauth/:provider.
// Removes an OAuth provider link from the authenticated customer's account.
func (h *CustomerHandler) UnlinkOAuthAccount(c *gin.Context) {
	provider := c.Param("provider")
	if !models.IsValidOAuthProvider(provider) {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_PROVIDER", "Unsupported OAuth provider")
		return
	}

	customerID := middleware.GetUserID(c)
	err := h.oauthService.UnlinkAccount(
		c.Request.Context(), customerID, provider)
	if err != nil {
		h.logger.Error("failed to unlink oauth account",
			"customer_id", customerID, "provider", provider,
			"error", err, "correlation_id", middleware.GetCorrelationID(c))

		var valErr sharederrors.ValidationError
		if errors.As(err, &valErr) {
			middleware.RespondWithError(c, http.StatusBadRequest,
				"UNLINK_FAILED", valErr.Error())
			return
		}
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound,
				"NOT_FOUND", "OAuth link not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"UNLINK_FAILED", "Failed to unlink OAuth account")
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Data: gin.H{"message": "OAuth account unlinked successfully"},
	})
}
```

**Verify:**
```bash
go build ./internal/controller/api/customer/...
```

**Commit:**
```
feat(api): add OAuth handlers — authorize, callback, link, unlink

OAuthAuthorize redirects to provider with PKCE challenge. OAuthCallback
exchanges code for tokens and returns JWT. ListOAuthLinks, LinkOAuthAccount,
and UnlinkOAuthAccount manage account-level OAuth settings (JWT-only).

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 10: Register OAuth Routes

- [ ] Wire OAuth routes into customer route registration

**File:** `internal/controller/api/customer/routes.go`

### 10a. Update `RegisterCustomerRoutes` signature

Add `oauthEnabled bool` parameter and pass it to `registerAuthRoutes`:

```go
func RegisterCustomerRoutes(
	router *gin.RouterGroup,
	handler *CustomerHandler,
	notifyHandler *NotificationsHandler,
	apiKeyRepo *repository.CustomerAPIKeyRepository,
	allowSelfRegistration bool,
	oauthEnabled bool,
) {
```

### 10b. Update `registerAuthRoutes`

Add OAuth routes conditionally:

```go
func registerAuthRoutes(
	customer *gin.RouterGroup,
	handler *CustomerHandler,
	allowSelfRegistration bool,
	oauthEnabled bool,
) {
	auth := customer.Group("/auth")
	{
		// ... existing routes unchanged ...

		if allowSelfRegistration {
			auth.POST("/register", middleware.RegistrationRateLimit(), handler.Register)
			auth.POST("/verify-email", middleware.RegistrationRateLimit(), handler.VerifyEmail)
		}

		if oauthEnabled {
			oauth := auth.Group("/oauth/:provider")
			oauth.Use(middleware.LoginRateLimit())
			{
				oauth.GET("/authorize", handler.OAuthAuthorize)
				oauth.POST("/callback", handler.OAuthCallback)
			}
		}
	}
}
```

### 10c. Add OAuth account management routes

Add to the account routes section (JWT-only):

```go
func registerOAuthAccountRoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	oauth := protected.Group("/account/oauth")
	{
		oauth.GET("", handler.ListOAuthLinks)
		oauth.POST("/:provider/link", handler.LinkOAuthAccount)
		oauth.DELETE("/:provider", handler.UnlinkOAuthAccount)
	}
}
```

Call it from `RegisterCustomerRoutes` inside the account group:

```go
	if oauthEnabled {
		registerOAuthAccountRoutes(accountGroup, handler)
	}
```

### 10d. Update call site in `server.go`

Update the `RegisterCustomerRoutes` call in `internal/controller/server.go` to pass the new `oauthEnabled` parameter:

```go
oauthEnabled := s.config.OAuth.Google.Enabled || s.config.OAuth.GitHub.Enabled
customer.RegisterCustomerRoutes(
	apiV1, s.customerHandler, s.notifyHandler,
	s.customerAPIKeyRepo, s.config.AllowSelfRegistration, oauthEnabled,
)
```

**Verify:**
```bash
go build ./internal/controller/...
```

**Commit:**
```
feat(routes): register OAuth auth and account management routes

OAuth auth routes (/auth/oauth/:provider/authorize and /callback) are
registered when any OAuth provider is enabled. Account management
routes (/account/oauth/*) for link/unlink require JWT authentication.
Both gate on config flags.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 11: Update CustomerHandler Struct for OAuth

- [ ] Add `oauthService` field to `CustomerHandler` and update the constructor

**File:** `internal/controller/api/customer/handler.go` (or wherever CustomerHandler is defined)

Add the `oauthService` field:

```go
type CustomerHandler struct {
	// ... existing fields ...
	oauthService *services.OAuthService
}
```

Update the handler constructor or factory to accept and wire `OAuthService`.

**File:** `internal/controller/dependencies.go`

In `InitializeServices()`, after the auth service initialization, add:

```go
// OAuth setup
oauthLinkRepo := repository.NewOAuthLinkRepository(s.dbPool)

var oauthService *services.OAuthService
oauthProviders := make(map[string]services.OAuthProvider)
if s.config.OAuth.Google.Enabled {
	oauthProviders[models.OAuthProviderGoogle] = services.NewGoogleOAuthProvider(
		s.config.OAuth.Google.ClientID,
		string(s.config.OAuth.Google.ClientSecret.Value()),
	)
}
if s.config.OAuth.GitHub.Enabled {
	oauthProviders[models.OAuthProviderGitHub] = services.NewGitHubOAuthProvider(
		s.config.OAuth.GitHub.ClientID,
		string(s.config.OAuth.GitHub.ClientSecret.Value()),
	)
}
if len(oauthProviders) > 0 {
	oauthService = services.NewOAuthService(services.OAuthServiceConfig{
		OAuthLinkRepo:      oauthLinkRepo,
		CustomerRepo:       customerRepo,
		AuthService:        authService,
		Providers:          oauthProviders,
		EncryptionKey:      s.config.EncryptionKey,
		AllowRegistration:  s.config.AllowSelfRegistration,
		PrimaryBillingProv: primaryBillingProvider,
		Logger:             s.logger,
	})
}
```

Pass `oauthService` to the customer handler constructor.

**Verify:**
```bash
go build ./internal/controller/...
```

**Commit:**
```
feat(deps): wire OAuthService into CustomerHandler via dependencies.go

Initializes OAuthLinkRepository and OAuthService with configured
providers. Only creates the service when at least one OAuth provider
is enabled. Passes to CustomerHandler for use in OAuth endpoints.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 12: OAuth Service Tests

- [ ] Write comprehensive tests for OAuthService

**File to create:** `internal/controller/services/oauth_service_test.go`

Write table-driven tests covering:

1. **HandleCallback — new customer creation** (self-registration enabled, no existing account)
2. **HandleCallback — existing link found** (returns existing customer, logs in)
3. **HandleCallback — auto-link by email** (non-WHMCS customer, email matches)
4. **HandleCallback — reject WHMCS customer** (email matches but has `whmcs_client_id`)
5. **HandleCallback — reject pending verification** (email exists, status = pending)
6. **HandleCallback — reject suspended account** (email exists, status = suspended)
7. **HandleCallback — registration disabled, no matching email** (returns error)
8. **HandleCallback — code exchange failure** (provider returns error)
9. **LinkAccount — happy path** (JWT-authenticated, new provider link)
10. **LinkAccount — already linked to another customer** (returns conflict)
11. **LinkAccount — idempotent re-link** (same customer, same provider — no-op)
12. **UnlinkAccount — happy path** (has password or other links)
13. **UnlinkAccount — last auth method, no password** (blocked with error)
14. **GetLinkedAccounts — returns all links**

Use mock structs for `OAuthLinkRepository`, `CustomerRepository`, `AuthService`, and `OAuthProvider`. Each mock uses function fields.

**Verify:**
```bash
go test -race -run TestOAuthService ./internal/controller/services/...
```

**Commit:**
```
test(services): add comprehensive OAuthService tests

14 table-driven tests covering account linking strategy, registration
gating, WHMCS coexistence, and edge cases (suspended accounts, last
auth method protection). Uses mocked repos and providers.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 13: OAuth Handler Tests

- [ ] Write tests for OAuth HTTP handlers

**File to create:** `internal/controller/api/customer/oauth_test.go`

Write table-driven tests covering:

1. **OAuthAuthorize — valid provider** (returns 307 redirect)
2. **OAuthAuthorize — invalid provider** (returns 400)
3. **OAuthAuthorize — missing code_challenge** (returns 400)
4. **OAuthCallback — successful login** (returns 200 with tokens)
5. **OAuthCallback — invalid provider** (returns 400)
6. **OAuthCallback — validation error from service** (returns 400)
7. **OAuthCallback — forbidden (suspended account)** (returns 403)
8. **OAuthCallback — internal error** (returns 500)
9. **ListOAuthLinks — returns links** (JWT auth)
10. **LinkOAuthAccount — successful** (returns 200)
11. **UnlinkOAuthAccount — successful** (returns 200)
12. **UnlinkOAuthAccount — not found** (returns 404)
13. **UnlinkOAuthAccount — last method** (returns 400)

Use httptest.NewRecorder and mock OAuthService.

**Verify:**
```bash
go test -race -run TestOAuth ./internal/controller/api/customer/...
```

**Commit:**
```
test(api): add OAuth handler tests

13 test cases covering authorize redirect, callback success/failure,
account linking, unlinking, and error paths. Uses httptest recorder
and mocked OAuthService.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 14: OAuth Provider Tests

- [ ] Write tests for Google and GitHub OAuth providers

**File to create:** `internal/controller/services/oauth_google_test.go`

Tests:
1. **AuthorizationURL** — contains correct query params, PKCE challenge, scopes
2. **ExchangeCode — success** (mocked HTTP response with tokens)
3. **ExchangeCode — error response from Google** (returns error)
4. **GetUserInfo — success** (returns parsed profile)
5. **GetUserInfo — error response** (returns error)

**File to create:** `internal/controller/services/oauth_github_test.go`

Tests:
1. **AuthorizationURL** — contains correct params with `read:user user:email` scope
2. **ExchangeCode — success** (mocked HTTP with JSON Accept header)
3. **ExchangeCode — oauth error in response body** (GitHub returns error field)
4. **GetUserInfo — success with public email** (returns user directly)
5. **GetUserInfo — private email, fetches from /user/emails** (mocked second call)
6. **GetUserInfo — no verified primary email** (returns error)

Use `httptest.NewServer` to mock Google/GitHub API endpoints.

**Verify:**
```bash
go test -race -run TestGoogle ./internal/controller/services/...
go test -race -run TestGitHub ./internal/controller/services/...
```

**Commit:**
```
test(services): add Google and GitHub OAuth provider tests

Tests AuthorizationURL generation, code exchange, and user info
fetching with httptest mock servers. Covers GitHub's private email
fallback via /user/emails endpoint.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 15: PKCE Utility for Frontend

- [ ] Create PKCE code generation utility in the customer portal

**File to create:** `webui/customer/lib/pkce.ts`

```typescript
/**
 * PKCE (Proof Key for Code Exchange) utilities for OAuth 2.0.
 * Generates code verifier and code challenge per RFC 7636.
 */

function base64URLEncode(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

/** Generate a cryptographically random code verifier (43–128 chars). */
export function generateCodeVerifier(): string {
  const buffer = new Uint8Array(32);
  crypto.getRandomValues(buffer);
  return base64URLEncode(buffer.buffer);
}

/** Generate the S256 code challenge from a code verifier. */
export async function generateCodeChallenge(verifier: string): Promise<string> {
  const encoder = new TextEncoder();
  const data = encoder.encode(verifier);
  const digest = await crypto.subtle.digest("SHA-256", data);
  return base64URLEncode(digest);
}

/** Generate a cryptographically random state parameter. */
export function generateState(): string {
  const buffer = new Uint8Array(16);
  crypto.getRandomValues(buffer);
  return base64URLEncode(buffer.buffer);
}
```

**Verify:**
```bash
cd webui/customer && npm run type-check
```

**Commit:**
```
feat(customer-ui): add PKCE utility for OAuth code challenge generation

Implements RFC 7636 code verifier (32 random bytes, base64url),
S256 code challenge (SHA-256 of verifier), and random state parameter.
Uses Web Crypto API for browser-native cryptographic operations.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 16: OAuth API Client Functions

- [ ] Add OAuth API functions to the customer API client

**File:** `webui/customer/lib/api-client.ts`

Add a new `oauthApi` export:

```typescript
export const oauthApi = {
  /** Initiate OAuth redirect to provider. */
  async getAuthorizeURL(
    provider: string,
    codeChallenge: string,
    state: string,
    redirectURI: string
  ): Promise<string> {
    const params = new URLSearchParams({
      code_challenge: codeChallenge,
      state,
      redirect_uri: redirectURI,
    });
    return `${API_BASE_URL}/customer/auth/oauth/${provider}/authorize?${params.toString()}`;
  },

  /** Exchange authorization code for JWT tokens. */
  async callback(
    provider: string,
    code: string,
    codeVerifier: string,
    redirectURI: string,
    state: string
  ): Promise<AuthResponse> {
    return apiClient.post<AuthResponse>(
      `/customer/auth/oauth/${provider}/callback`,
      { code, code_verifier: codeVerifier, redirect_uri: redirectURI, state }
    );
  },

  /** List linked OAuth accounts for the current customer. */
  async listLinks(): Promise<OAuthLink[]> {
    return apiClient.get<OAuthLink[]>("/customer/account/oauth");
  },

  /** Link an OAuth provider to the authenticated customer. */
  async linkAccount(
    provider: string,
    code: string,
    codeVerifier: string,
    redirectURI: string,
    state: string
  ): Promise<{ message: string }> {
    return apiClient.post<{ message: string }>(
      `/customer/account/oauth/${provider}/link`,
      { code, code_verifier: codeVerifier, redirect_uri: redirectURI, state }
    );
  },

  /** Unlink an OAuth provider from the authenticated customer. */
  async unlinkAccount(provider: string): Promise<void> {
    return apiClient.delete<void>(`/customer/account/oauth/${provider}`);
  },
};

export interface OAuthLink {
  id: string;
  customer_id: string;
  provider: string;
  email?: string;
  display_name?: string;
  avatar_url?: string;
  created_at: string;
  updated_at: string;
}

export interface AuthResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  requires_2fa?: boolean;
  temp_token?: string;
}
```

**Verify:**
```bash
cd webui/customer && npm run type-check
```

**Commit:**
```
feat(customer-ui): add OAuth API client functions

Adds oauthApi with getAuthorizeURL, callback, listLinks, linkAccount,
and unlinkAccount. Includes OAuthLink and AuthResponse interfaces.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 17: OAuth Callback Page

- [ ] Create frontend OAuth callback page to handle provider redirect

**File to create:** `webui/customer/app/auth/callback/page.tsx`

```tsx
"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Loader2, AlertCircle } from "lucide-react";

import { Card, CardContent, CardHeader, CardTitle } from "@virtuestack/ui";
import { Button } from "@virtuestack/ui";
import { oauthApi } from "@/lib/api-client";
import { useAuth } from "@/lib/auth-context";

export default function OAuthCallbackPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { refreshAuthState } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const processed = useRef(false);

  useEffect(() => {
    if (processed.current) return;
    processed.current = true;

    const code = searchParams.get("code");
    const state = searchParams.get("state");
    const errorParam = searchParams.get("error");

    if (errorParam) {
      setError(
        searchParams.get("error_description") ??
          "OAuth authorization was denied"
      );
      return;
    }

    if (!code || !state) {
      setError("Missing authorization code or state parameter");
      return;
    }

    const storedState = sessionStorage.getItem("oauth_state");
    const codeVerifier = sessionStorage.getItem("oauth_code_verifier");
    const provider = sessionStorage.getItem("oauth_provider");
    const redirectURI = sessionStorage.getItem("oauth_redirect_uri");
    const mode = sessionStorage.getItem("oauth_mode");

    sessionStorage.removeItem("oauth_state");
    sessionStorage.removeItem("oauth_code_verifier");
    sessionStorage.removeItem("oauth_provider");
    sessionStorage.removeItem("oauth_redirect_uri");
    sessionStorage.removeItem("oauth_mode");

    if (state !== storedState) {
      setError("Invalid state parameter — possible CSRF attack");
      return;
    }

    if (!codeVerifier || !provider || !redirectURI) {
      setError("Missing PKCE data. Please try again.");
      return;
    }

    const handleCallback = async () => {
      try {
        if (mode === "link") {
          await oauthApi.linkAccount(
            provider, code, codeVerifier, redirectURI, state
          );
          router.push("/settings?linked=" + provider);
        } else {
          await oauthApi.callback(
            provider, code, codeVerifier, redirectURI, state
          );
          await refreshAuthState();
          router.push("/vms");
        }
      } catch (err) {
        const message =
          err instanceof Error ? err.message : "OAuth authentication failed";
        setError(message);
      }
    };

    handleCallback();
  }, [searchParams, router, refreshAuthState]);

  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center p-4 bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-blue-950">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-destructive">
              <AlertCircle className="h-5 w-5" />
              Authentication Failed
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm text-muted-foreground">{error}</p>
            <Button onClick={() => router.push("/login")} className="w-full">
              Back to Login
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4 bg-gradient-to-br from-blue-50 to-indigo-100 dark:from-gray-900 dark:to-blue-950">
      <Card className="w-full max-w-md">
        <CardContent className="flex flex-col items-center gap-4 py-12">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
          <p className="text-muted-foreground">Completing authentication...</p>
        </CardContent>
      </Card>
    </div>
  );
}
```

**Verify:**
```bash
cd webui/customer && npm run type-check
```

**Commit:**
```
feat(customer-ui): add OAuth callback page

Handles OAuth provider redirect by validating state, exchanging code
via PKCE, and redirecting to /vms on success. Supports both login and
account-linking modes via sessionStorage flag. Shows error state with
back-to-login button on failure.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 18: Login Page — OAuth Buttons

- [ ] Add "Sign in with Google" and "Sign in with GitHub" buttons to the login page

**File:** `webui/customer/app/login/page.tsx`

### 18a. Add OAuth button helper

Add an `OAuthButtons` component and the `startOAuthFlow` function. The component renders Google and GitHub buttons only when the backend reports OAuth is enabled (via a `/customer/auth/features` endpoint or client-side env var).

```tsx
import { generateCodeVerifier, generateCodeChallenge, generateState } from "@/lib/pkce";
import { oauthApi } from "@/lib/api-client";

function OAuthButtons() {
  const [isLoading, setIsLoading] = useState<string | null>(null);

  const startOAuth = async (provider: string) => {
    setIsLoading(provider);
    try {
      const codeVerifier = generateCodeVerifier();
      const codeChallenge = await generateCodeChallenge(codeVerifier);
      const state = generateState();
      const redirectURI = `${window.location.origin}/auth/callback`;

      sessionStorage.setItem("oauth_code_verifier", codeVerifier);
      sessionStorage.setItem("oauth_state", state);
      sessionStorage.setItem("oauth_provider", provider);
      sessionStorage.setItem("oauth_redirect_uri", redirectURI);
      sessionStorage.setItem("oauth_mode", "login");

      const url = await oauthApi.getAuthorizeURL(
        provider, codeChallenge, state, redirectURI
      );
      window.location.href = url;
    } catch {
      setIsLoading(null);
    }
  };

  return (
    <div className="space-y-2">
      <Button
        type="button"
        variant="outline"
        className="w-full"
        disabled={isLoading !== null}
        onClick={() => startOAuth("google")}
      >
        {isLoading === "google" ? (
          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
        ) : (
          <GoogleIcon className="mr-2 h-4 w-4" />
        )}
        Sign in with Google
      </Button>
      <Button
        type="button"
        variant="outline"
        className="w-full"
        disabled={isLoading !== null}
        onClick={() => startOAuth("github")}
      >
        {isLoading === "github" ? (
          <Loader2 className="mr-2 h-4 w-4 animate-spin" />
        ) : (
          <GithubIcon className="mr-2 h-4 w-4" />
        )}
        Sign in with GitHub
      </Button>
    </div>
  );
}
```

### 18b. Integrate into login form

Insert the OAuth buttons between the login form and the "Forgot your password?" link. Add a visual separator:

```tsx
{/* After the Sign In button, before Forgot Password link */}
<div className="relative my-4">
  <div className="absolute inset-0 flex items-center">
    <span className="w-full border-t" />
  </div>
  <div className="relative flex justify-center text-xs uppercase">
    <span className="bg-background px-2 text-muted-foreground">Or continue with</span>
  </div>
</div>
<OAuthButtons />
```

### 18c. Conditionally show OAuth buttons

Only render `OAuthButtons` when OAuth is configured. Add an environment variable `NEXT_PUBLIC_OAUTH_ENABLED=true` that controls visibility, or query a backend features endpoint.

**Verify:**
```bash
cd webui/customer && npm run type-check && npm run lint
```

**Commit:**
```
feat(customer-ui): add OAuth login buttons to login page

Shows "Sign in with Google" and "Sign in with GitHub" buttons with
PKCE flow initiation. Buttons store code_verifier and state in
sessionStorage and redirect to provider. Separated from password
form with "Or continue with" divider.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 19: Account Settings — OAuth Section

- [ ] Add OAuth link management to customer account settings page

**File:** `webui/customer/app/settings/page.tsx` (or appropriate settings component)

Add an "Linked Accounts" section to the settings page:

- Shows currently linked OAuth providers (Google, GitHub) with email and avatar
- "Link Google Account" / "Link GitHub Account" buttons for unlinked providers
- "Unlink" button for each linked provider
- Confirmation dialog before unlinking
- Warning when unlinking would remove the last auth method (no password set)
- Success/error toast notifications

The link flow reuses the same PKCE utilities from Task 15, with `sessionStorage.setItem("oauth_mode", "link")` to distinguish from login flow in the callback page.

Use TanStack Query for fetching linked accounts (`useQuery` with `oauthApi.listLinks()`), and `useMutation` for link/unlink operations.

**Verify:**
```bash
cd webui/customer && npm run type-check && npm run lint
```

**Commit:**
```
feat(customer-ui): add OAuth account linking section to settings

Shows linked OAuth providers with link/unlink buttons. Link flow
uses PKCE with "link" mode flag. Unlink shows confirmation dialog
and warns if removing last auth method. Uses TanStack Query for
data fetching and mutations.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 20: Customer Repository Updates for Nullable PasswordHash

- [ ] Update CustomerRepository queries to handle nullable `password_hash`

**File:** `internal/controller/repository/customer_repo.go`

### 20a. Update `Create` method

The `INSERT` statement must handle `NULL` password_hash for OAuth-only customers:

```go
// In the Create method, change the password_hash parameter from
// customer.PasswordHash to handle the *string:
func (r *CustomerRepository) Create(ctx context.Context, customer *models.Customer) (*models.Customer, error) {
	// ... existing query ...
	// Use customer.PasswordHash directly — pgx handles *string → NULL
}
```

### 20b. Update `scanCustomer` helper

Update the scan to use `*string` for password_hash:

```go
// scanCustomer must scan into a *string for password_hash.
func scanCustomer(row pgx.Row) (*models.Customer, error) {
	var c models.Customer
	err := row.Scan(
		&c.ID, &c.Email, &c.PasswordHash, // *string scans NULL correctly
		// ... rest of fields ...
		&c.AuthProvider,
		// ...
	)
	// ...
}
```

### 20c. Update `UpdatePassword`

Ensure the method stores the hash as a `*string`:

```go
func (r *CustomerRepository) UpdatePassword(
	ctx context.Context, customerID, passwordHash string,
) error {
	// Uses $2 directly — pgx handles string → non-NULL VARCHAR
}
```

### 20d. Add `auth_provider` to all SELECT column lists

Update the `customerColumns` constant to include `auth_provider`.

**Verify:**
```bash
go build ./internal/controller/repository/...
go test -race ./internal/controller/repository/...
```

**Commit:**
```
feat(repository): update CustomerRepository for nullable password_hash and auth_provider

Updates column lists, Create, scan helpers, and UpdatePassword to
handle *string PasswordHash (nullable for OAuth-only) and new
AuthProvider column. pgx handles *string ↔ NULL mapping natively.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 21: Provisioning Handler Updates

- [ ] Update provisioning customer and password handlers for nullable PasswordHash

**File:** `internal/controller/api/provisioning/customers.go`

Update `CreateOrGetCustomer` to assign PasswordHash as pointer:

```go
customer := &models.Customer{
	Email:        email,
	PasswordHash: &passwordHash,
	AuthProvider: models.AuthProviderLocal,
	// ...
}
```

**File:** `internal/controller/api/provisioning/password.go`

Update `SetPassword` and `ResetPassword` to use pointer assignment.

**Verify:**
```bash
go build ./internal/controller/api/provisioning/...
```

**Commit:**
```
fix(provisioning): update handlers for nullable Customer.PasswordHash

Assigns PasswordHash via pointer in CreateOrGetCustomer, SetPassword,
and ResetPassword. Sets AuthProvider to 'local' for WHMCS-created
customers.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 22: Model Tests Update

- [ ] Update existing Customer model tests for new fields

**File:** `internal/controller/models/customer_test.go`

Add test cases:
1. Customer with nil PasswordHash (OAuth-only) serializes correctly
2. Customer with AuthProvider field values
3. IsValidOAuthProvider returns correct results

**File to create:** `internal/controller/models/oauth_link_test.go`

Add test cases:
1. IsValidOAuthProvider — "google" returns true
2. IsValidOAuthProvider — "github" returns true
3. IsValidOAuthProvider — "facebook" returns false
4. IsValidOAuthProvider — "" returns false
5. OAuthLink JSON serialization excludes sensitive fields

**Verify:**
```bash
go test -race ./internal/controller/models/...
```

**Commit:**
```
test(models): add OAuth model tests and update Customer tests

Tests IsValidOAuthProvider, OAuthLink JSON serialization (sensitive
fields excluded), and Customer with nullable PasswordHash and
AuthProvider field.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 23: SSRF-Safe HTTP Client Test

- [ ] Write tests verifying the SSRF-safe transport blocks private IPs

**File to create:** `internal/controller/services/oauth_provider_test.go`

Write tests:
1. **SSRF blocked — localhost** — request to 127.0.0.1 fails
2. **SSRF blocked — private range** — request to 10.0.0.1 fails
3. **SSRF blocked — metadata** — request to 169.254.169.254 fails
4. **Allowed — public IP** — request to httptest server on 127.0.0.1 with override works

**Verify:**
```bash
go test -race -run TestSSRF ./internal/controller/services/...
```

**Commit:**
```
test(services): verify SSRF-safe transport blocks private IPs

Tests that the OAuth HTTP client blocks requests resolving to
localhost, RFC-1918 ranges, and cloud metadata endpoints.

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Task 24: Final Verification

- [ ] Run full build, lint, and test suite to verify all changes

**Steps:**

```bash
# 1. Build everything
make build-controller

# 2. Run all unit tests with race detector
make test-race

# 3. Run linter (if golangci-lint is installed)
make lint

# 4. Build frontend
cd webui/customer && npm run type-check && npm run lint && npm run build

# 5. Verify migration files exist
ls -la migrations/000078_oauth_customer_links.*
```

**Expected results:**
- Controller builds without errors
- All tests pass (existing + new OAuth tests)
- Frontend type-checks, lints, and builds
- Migration files exist with valid SQL

**Commit:**
```
chore: verify Phase 8 (OAuth) — all tests pass, builds clean

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>
```

---

## Summary of Deliverables

| # | Deliverable | Files Changed/Created |
|---|------------|----------------------|
| 1 | OAuth migration (000078) | `migrations/000078_oauth_customer_links.{up,down}.sql` |
| 2 | OAuthLink model | `internal/controller/models/oauth_link.go` |
| 3 | Customer model update | `internal/controller/models/customer.go` |
| 4 | OAuth link repository | `internal/controller/repository/oauth_link_repo.go` |
| 5 | OAuth link repo tests | `internal/controller/repository/oauth_link_repo_test.go` |
| 6 | OAuth provider interface + Google/GitHub | `internal/controller/services/oauth_provider.go`, `oauth_google.go`, `oauth_github.go` |
| 7 | OAuth service | `internal/controller/services/oauth_service.go` |
| 8 | AuthService exported methods | `internal/controller/services/auth_service.go` |
| 9 | OAuth HTTP handlers | `internal/controller/api/customer/oauth.go` |
| 10 | OAuth route registration | `internal/controller/api/customer/routes.go`, `internal/controller/server.go` |
| 11 | Handler + dependency wiring | `internal/controller/api/customer/handler.go`, `internal/controller/dependencies.go` |
| 12 | OAuth service tests | `internal/controller/services/oauth_service_test.go` |
| 13 | OAuth handler tests | `internal/controller/api/customer/oauth_test.go` |
| 14 | OAuth provider tests | `internal/controller/services/oauth_google_test.go`, `oauth_github_test.go` |
| 15 | PKCE utility | `webui/customer/lib/pkce.ts` |
| 16 | OAuth API client | `webui/customer/lib/api-client.ts` |
| 17 | OAuth callback page | `webui/customer/app/auth/callback/page.tsx` |
| 18 | Login page OAuth buttons | `webui/customer/app/login/page.tsx` |
| 19 | Settings OAuth section | `webui/customer/app/settings/page.tsx` |
| 20 | Customer repository update | `internal/controller/repository/customer_repo.go` |
| 21 | Provisioning handler update | `internal/controller/api/provisioning/customers.go`, `password.go` |
| 22 | Model tests | `internal/controller/models/customer_test.go`, `oauth_link_test.go` |
| 23 | SSRF transport tests | `internal/controller/services/oauth_provider_test.go` |
| 24 | Final verification | (no files — build/test confirmation) |

## API Endpoints Introduced

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| `GET` | `/customer/auth/oauth/:provider/authorize` | None | Redirect to OAuth provider |
| `POST` | `/customer/auth/oauth/:provider/callback` | None | Exchange code for tokens |
| `GET` | `/customer/account/oauth` | JWT | List linked accounts |
| `POST` | `/customer/account/oauth/:provider/link` | JWT | Link new OAuth account |
| `DELETE` | `/customer/account/oauth/:provider` | JWT | Unlink OAuth account |

## Environment Variables Used (from Phase 1)

| Variable | Required When |
|----------|---------------|
| `OAUTH_GOOGLE_ENABLED` | — (default: false) |
| `OAUTH_GOOGLE_CLIENT_ID` | `OAUTH_GOOGLE_ENABLED=true` |
| `OAUTH_GOOGLE_CLIENT_SECRET` | `OAUTH_GOOGLE_ENABLED=true` |
| `OAUTH_GITHUB_ENABLED` | — (default: false) |
| `OAUTH_GITHUB_CLIENT_ID` | `OAUTH_GITHUB_ENABLED=true` |
| `OAUTH_GITHUB_CLIENT_SECRET` | `OAUTH_GITHUB_ENABLED=true` |
