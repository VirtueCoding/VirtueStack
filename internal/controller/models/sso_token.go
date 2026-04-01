package models

import "time"

const SSOTokenTTL = 5 * time.Minute

// SSOToken represents a short-lived, single-use browser bootstrap token used for
// WHMCS → Customer WebUI single sign-on without exposing JWTs in URL query strings.
type SSOToken struct {
	ID           string    `json:"id"`
	TokenHash    []byte    `json:"-"`
	CustomerID   string    `json:"customer_id"`
	VMID         string    `json:"vm_id"`
	RedirectPath string    `json:"redirect_path"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}
