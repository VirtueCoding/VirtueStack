package repository

import (
	"context"
	"fmt"
	"time"
)

// BillingCheckpointRepository provides database operations for hourly billing checkpoints.
type BillingCheckpointRepository struct {
	db DB
}

// NewBillingCheckpointRepository creates a new BillingCheckpointRepository.
func NewBillingCheckpointRepository(db DB) *BillingCheckpointRepository {
	return &BillingCheckpointRepository{db: db}
}

// RecordCheckpoint inserts a billing checkpoint. Uses ON CONFLICT DO NOTHING
// to safely handle duplicate attempts (HA double-execution).
func (r *BillingCheckpointRepository) RecordCheckpoint(
	ctx context.Context,
	vmID string,
	chargeHour time.Time,
	amount int64,
	transactionID *string,
) error {
	q := `INSERT INTO billing_vm_checkpoints (vm_id, charge_hour, amount, transaction_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (vm_id, charge_hour) DO NOTHING`
	_, err := r.db.Exec(ctx, q, vmID, chargeHour, amount, transactionID)
	if err != nil {
		return fmt.Errorf("record billing checkpoint: %w", err)
	}
	return nil
}

// ExistsForHour checks if a billing checkpoint exists for the given VM and hour.
func (r *BillingCheckpointRepository) ExistsForHour(
	ctx context.Context, vmID string, chargeHour time.Time,
) (bool, error) {
	q := `SELECT EXISTS(
		SELECT 1 FROM billing_vm_checkpoints
		WHERE vm_id = $1 AND charge_hour = $2)`
	var exists bool
	if err := r.db.QueryRow(ctx, q, vmID, chargeHour).Scan(&exists); err != nil {
		return false, fmt.Errorf("check billing checkpoint: %w", err)
	}
	return exists, nil
}
