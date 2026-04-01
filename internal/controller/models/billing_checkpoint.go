package models

import "time"

// BillingVMCheckpoint records a durable per-VM hourly billing checkpoint.
// The composite key (vm_id, charge_hour) makes double-billing physically
// impossible at the database level.
type BillingVMCheckpoint struct {
	VMID          string    `json:"vm_id" db:"vm_id"`
	ChargeHour    time.Time `json:"charge_hour" db:"charge_hour"`
	Amount        int64     `json:"amount" db:"amount"`
	TransactionID *string   `json:"transaction_id,omitempty" db:"transaction_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}
