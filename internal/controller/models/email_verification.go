package models

import "time"

// EmailVerificationToken stores a one-time token used to verify customer email ownership.
type EmailVerificationToken struct {
	ID        string    `json:"id" db:"id"`
	CustomerID string   `json:"customer_id" db:"customer_id"`
	TokenHash string    `json:"-" db:"token_hash"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
