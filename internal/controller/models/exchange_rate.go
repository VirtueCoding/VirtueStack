package models

import "time"

// ExchangeRateSource constants define how a rate was obtained.
const (
	ExchangeRateSourceAPI   = "api"
	ExchangeRateSourceAdmin = "admin"
)

// ExchangeRate represents a currency conversion rate to USD.
type ExchangeRate struct {
	Currency  string    `json:"currency" db:"currency"`
	RateToUSD float64   `json:"rate_to_usd" db:"rate_to_usd"`
	Source    string    `json:"source" db:"source"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// UpdateExchangeRateRequest holds fields for setting an exchange rate.
type UpdateExchangeRateRequest struct {
	RateToUSD float64 `json:"rate_to_usd" validate:"required,gt=0"`
}
