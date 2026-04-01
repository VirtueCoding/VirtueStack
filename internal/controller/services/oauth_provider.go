package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
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
			host, port, err := net.SplitHostPort(addr)
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
			// Dial using the resolved IP to prevent DNS rebinding attacks
			dialer := &net.Dialer{Timeout: 10 * time.Second}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
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
