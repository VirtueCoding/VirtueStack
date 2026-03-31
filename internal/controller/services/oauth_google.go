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
		"client_id":              {g.clientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {"openid email profile"},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
	}
	return googleAuthURL + "?" + params.Encode()
}

func (g *GoogleOAuthProvider) ExchangeCode(
	ctx context.Context, code, codeVerifier, redirectURI string,
) (*OAuthTokens, error) {
	return g.exchangeCodeWithURL(ctx, googleTokenURL, code, codeVerifier, redirectURI)
}

func (g *GoogleOAuthProvider) exchangeCodeWithURL(
	ctx context.Context, tokenURL, code, codeVerifier, redirectURI string,
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
		ctx, http.MethodPost, tokenURL,
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
	return g.getUserInfoWithURL(ctx, googleUserInfoURL, accessToken)
}

func (g *GoogleOAuthProvider) getUserInfoWithURL(
	ctx context.Context, userInfoURL, accessToken string,
) (*models.OAuthUserInfo, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, userInfoURL, nil,
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
