// Package models provides data model types for VirtueStack Controller.
package models

import "time"

// Customer status constants define the account states of a customer.
const (
	CustomerStatusActive    = "active"
	CustomerStatusSuspended = "suspended"
	CustomerStatusDeleted   = "deleted"
)

// Customer represents a customer account as stored in the database.
type Customer struct {
	ID                  string   `json:"id" db:"id"`
	Email               string   `json:"email" db:"email"`
	PasswordHash        string   `json:"-" db:"password_hash"`        // Never expose
	Name                string   `json:"name" db:"name"`
	WHMCSClientID       *int     `json:"whmcs_client_id,omitempty" db:"whmcs_client_id"`
	TOTPSecretEncrypted *string  `json:"-" db:"totp_secret_encrypted"` // Never expose
	TOTPEnabled         bool     `json:"totp_enabled" db:"totp_enabled"`
	TOTPBackupCodesHash []string `json:"-" db:"totp_backup_codes_hash"` // Never expose
	Status              string   `json:"status" db:"status"`
	Timestamps
}

// Admin represents an administrative user account with elevated privileges.
type Admin struct {
	ID                  string    `json:"id" db:"id"`
	Email               string    `json:"email" db:"email"`
	PasswordHash        string    `json:"-" db:"password_hash"`
	Name                string    `json:"name" db:"name"`
	TOTPSecretEncrypted string    `json:"-" db:"totp_secret_encrypted"`
	TOTPEnabled         bool      `json:"totp_enabled" db:"totp_enabled"`
	TOTPBackupCodesHash []string  `json:"-" db:"totp_backup_codes_hash"`
	Role                string    `json:"role" db:"role"`
	MaxSessions         int       `json:"max_sessions" db:"max_sessions"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
}

// Session represents an authenticated user session stored in the database.
type Session struct {
	ID               string     `json:"id" db:"id"`
	UserID           string     `json:"user_id" db:"user_id"`
	UserType         string     `json:"user_type" db:"user_type"` // "customer" or "admin"
	RefreshTokenHash string     `json:"-" db:"refresh_token_hash"`
	IPAddress        *string    `json:"ip_address,omitempty" db:"ip_address"`
	UserAgent        *string    `json:"user_agent,omitempty" db:"user_agent"`
	ExpiresAt        time.Time  `json:"expires_at" db:"expires_at"`
	LastReauthAt     *time.Time `json:"last_reauth_at,omitempty" db:"last_reauth_at"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
}

// LoginRequest holds credentials submitted during a login attempt.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,min=1,max=128"`
}

// Verify2FARequest holds the temporary token and TOTP code for two-factor verification.
type Verify2FARequest struct {
	TempToken string `json:"temp_token" validate:"required"`
	TOTPCode  string `json:"totp_code" validate:"required,len=6,numeric"`
}

// AuthTokens holds the result of a successful authentication, including access and refresh tokens.
type AuthTokens struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`          // "Bearer"
	ExpiresIn   int    `json:"expires_in"`          // seconds
	Requires2FA bool   `json:"requires_2fa,omitempty"`
	TempToken   string `json:"temp_token,omitempty"`
}

// PasswordReset represents a password reset token stored in the database.
type PasswordReset struct {
	ID        string     `json:"id" db:"id"`
	UserID    string     `json:"user_id" db:"user_id"`
	UserType  string     `json:"user_type" db:"user_type"`
	TokenHash string     `json:"-" db:"token_hash"`
	ExpiresAt time.Time  `json:"expires_at" db:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty" db:"used_at"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}
