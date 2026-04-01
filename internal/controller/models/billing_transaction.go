package models

import "time"

// Billing transaction type constants.
const (
	BillingTxTypeCredit     = "credit"
	BillingTxTypeDebit      = "debit"
	BillingTxTypeAdjustment = "adjustment"
	BillingTxTypeRefund     = "refund"
)

// Billing transaction reference type constants.
const (
	BillingRefTypePayment     = "payment"
	BillingRefTypeVMUsage     = "vm_usage"
	BillingRefTypeAdminAdjust = "admin_adjustment"
	BillingRefTypeRefund      = "refund"
)

// BillingTransaction represents an immutable entry in the credit ledger.
// Each transaction records a balance change with a snapshot of the resulting balance.
type BillingTransaction struct {
	ID             string    `json:"id" db:"id"`
	CustomerID     string    `json:"customer_id" db:"customer_id"`
	Type           string    `json:"type" db:"type"`
	Amount         int64     `json:"amount" db:"amount"`
	BalanceAfter   int64     `json:"balance_after" db:"balance_after"`
	Description    string    `json:"description" db:"description"`
	ReferenceType  *string   `json:"reference_type,omitempty" db:"reference_type"`
	ReferenceID    *string   `json:"reference_id,omitempty" db:"reference_id"`
	IdempotencyKey *string   `json:"-" db:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// CreateBillingTransactionRequest holds fields for inserting a ledger entry.
type CreateBillingTransactionRequest struct {
	CustomerID     string  `json:"customer_id" validate:"required,uuid"`
	Type           string  `json:"type" validate:"required,oneof=credit debit adjustment refund"`
	Amount         int64   `json:"amount" validate:"required"`
	Description    string  `json:"description" validate:"required,max=500"`
	ReferenceType  *string `json:"reference_type,omitempty" validate:"omitempty,max=30"`
	ReferenceID    *string `json:"reference_id,omitempty" validate:"omitempty,uuid"`
	IdempotencyKey *string `json:"-"`
}

// AdminCreditAdjustmentRequest holds fields for admin manual credit operations.
type AdminCreditAdjustmentRequest struct {
	Amount      int64  `json:"amount" validate:"required,ne=0"`
	Description string `json:"description" validate:"required,max=500"`
}
