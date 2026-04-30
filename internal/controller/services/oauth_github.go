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
	githubAuthURL   = "https://github.com/login/oauth/authorize"
	githubTokenURL  = "https://github.com/login/oauth/access_token"
	githubUserURL   = "https://api.github.com/user"
	githubEmailsURL = "https://api.github.com/user/emails"
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
		"client_id":              {g.clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {"read:user user:email"},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return githubAuthURL + "?" + params.Encode()
}

func (g *GitHubOAuthProvider) ExchangeCode(
	ctx context.Context, code, codeVerifier, redirectURI string,
) (*OAuthTokens, error) {
	return g.exchangeCodeWithURL(ctx, githubTokenURL, code, codeVerifier, redirectURI)
}

func (g *GitHubOAuthProvider) exchangeCodeWithURL(
	ctx context.Context, tokenURL, code, codeVerifier, redirectURI string,
) (*OAuthTokens, error) {
	data := url.Values{
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, tokenURL,
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
		ExpiresAt:   time.Now().Add(8 * time.Hour),
	}, nil
}

func (g *GitHubOAuthProvider) GetUserInfo(
	ctx context.Context, accessToken string,
) (*models.OAuthUserInfo, error) {
	return g.getUserInfoWithURL(ctx, githubUserURL, githubEmailsURL, accessToken)
}

func (g *GitHubOAuthProvider) getUserInfoWithURL(
	ctx context.Context, userURL, emailsURL, accessToken string,
) (*models.OAuthUserInfo, error) {
	userReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet, userURL, nil,
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
		fetchedEmail, err := g.fetchPrimaryEmailFromURL(ctx, emailsURL, accessToken)
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

// fetchPrimaryEmailFromURL retrieves the verified primary email from GitHub
// when the /user endpoint does not include one (private email setting).
func (g *GitHubOAuthProvider) fetchPrimaryEmailFromURL(
	ctx context.Context, emailsEndpointURL, accessToken string,
) (string, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, emailsEndpointURL, nil,
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
