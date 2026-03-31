package models

import (
	"encoding/json"
	"time"
)

// Billing payment gateway constants.
const (
	PaymentGatewayStripe      = "stripe"
	PaymentGatewayPayPal      = "paypal"
	PaymentGatewayBTCPay      = "btcpay"
	PaymentGatewayNOWPayments = "nowpayments"
	PaymentGatewayAdmin       = "admin"
)

// Billing payment status constants.
const (
	PaymentStatusPending   = "pending"
	PaymentStatusCompleted = "completed"
	PaymentStatusFailed    = "failed"
	PaymentStatusRefunded  = "refunded"
)

// BillingPayment tracks payment gateway interactions.
type BillingPayment struct {
	ID               string          `json:"id" db:"id"`
	CustomerID       string          `json:"customer_id" db:"customer_id"`
	Gateway          string          `json:"gateway" db:"gateway"`
	GatewayPaymentID *string         `json:"gateway_payment_id,omitempty" db:"gateway_payment_id"`
	Amount           int64           `json:"amount" db:"amount"`
	Currency         string          `json:"currency" db:"currency"`
	Status           string          `json:"status" db:"status"`
	ReuseKey         *string         `json:"-" db:"reuse_key"`
	Metadata         json.RawMessage `json:"-" db:"metadata"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}
