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
