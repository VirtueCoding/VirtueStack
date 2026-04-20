// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// CustomerAPIKeyRepository provides database operations for customer API keys.
// API keys are used for programmatic access to the VirtueStack API.
// The raw key value is never stored - only its SHA-256 hash.
type CustomerAPIKeyRepository struct {
	db DB
}

// NewCustomerAPIKeyRepository creates a new CustomerAPIKeyRepository.
func NewCustomerAPIKeyRepository(db DB) *CustomerAPIKeyRepository {
	return &CustomerAPIKeyRepository{db: db}
}

// scanCustomerAPIKey scans a single customer API key row.
func scanCustomerAPIKey(row pgx.Row) (models.CustomerAPIKey, error) {
	var key models.CustomerAPIKey
	var vmIDs []string
	var permissions []string
	var allowedIPs []string

	err := row.Scan(
		&key.ID, &key.CustomerID, &key.Name, &key.KeyHash,
		&allowedIPs, &vmIDs, &permissions, &key.LastUsedAt,
		&key.CreatedAt, &key.RevokedAt, &key.ExpiresAt,
	)

	key.AllowedIPs = allowedIPs
	key.VMIDs = vmIDs
	key.Permissions = permissions
	key.IsActive = key.RevokedAt == nil

	return key, err
}

const customerAPIKeySelectCols = `
	id, customer_id, name, key_hash,
	allowed_ips, vm_ids, permissions, last_used_at,
	created_at, revoked_at, expires_at`

// Create inserts a new customer API key into the database.
// The key hash should be a SHA-256 hash of the raw key value.
func (r *CustomerAPIKeyRepository) Create(ctx context.Context, key *models.CustomerAPIKey) error {
	const q = `
		INSERT INTO customer_api_keys (
			id, customer_id, name, key_hash, allowed_ips, vm_ids, permissions
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + customerAPIKeySelectCols

	row := r.db.QueryRow(ctx, q,
		key.ID, key.CustomerID, key.Name, key.KeyHash, key.AllowedIPs, key.VMIDs, key.Permissions,
	)

	created, err := scanCustomerAPIKey(row)
	if err != nil {
		return fmt.Errorf("creating customer API key: %w", err)
	}
	*key = created
	return nil
}

// GetByID returns a customer API key by its UUID.
// Returns ErrNotFound if the key doesn't exist.
func (r *CustomerAPIKeyRepository) GetByID(ctx context.Context, id string) (*models.CustomerAPIKey, error) {
	const q = `SELECT ` + customerAPIKeySelectCols + ` FROM customer_api_keys WHERE id = $1`
	key, err := ScanRow(ctx, r.db, q, []any{id}, scanCustomerAPIKey)
	if err != nil {
		return nil, fmt.Errorf("getting customer API key %s: %w", id, err)
	}
	return &key, nil
}

// GetByIDAndCustomer returns a customer API key by ID, ensuring it belongs to the customer.
// Returns ErrNotFound if the key doesn't exist or doesn't belong to the customer.
func (r *CustomerAPIKeyRepository) GetByIDAndCustomer(ctx context.Context, id, customerID string) (*models.CustomerAPIKey, error) {
	const q = `SELECT ` + customerAPIKeySelectCols + ` FROM customer_api_keys WHERE id = $1 AND customer_id = $2`
	key, err := ScanRow(ctx, r.db, q, []any{id, customerID}, scanCustomerAPIKey)
	if err != nil {
		return nil, fmt.Errorf("getting customer API key %s for customer %s: %w", id, customerID, err)
	}
	return &key, nil
}

// GetByHash returns an active customer API key by its SHA-256 hash.
// This is used by the API key authentication middleware.
// Returns ErrNotFound if the key doesn't exist or is revoked.
func (r *CustomerAPIKeyRepository) GetByHash(ctx context.Context, keyHash string) (*models.CustomerAPIKey, error) {
	const q = `SELECT ` + customerAPIKeySelectCols + ` FROM customer_api_keys WHERE key_hash = $1 AND revoked_at IS NULL`
	key, err := ScanRow(ctx, r.db, q, []any{keyHash}, scanCustomerAPIKey)
	if err != nil {
		return nil, fmt.Errorf("getting customer API key by hash: %w", err)
	}
	return &key, nil
}

// ListByCustomer returns all API keys for a customer, optionally including revoked keys.
// Pagination is intentionally omitted: the per-customer API key count is bounded
// by application policy (typically ≤20 keys per customer), so an unbounded query
// is safe and simplifies the caller.
func (r *CustomerAPIKeyRepository) ListByCustomer(ctx context.Context, customerID string, includeRevoked bool) ([]models.CustomerAPIKey, error) {
	q := `SELECT ` + customerAPIKeySelectCols + ` FROM customer_api_keys WHERE customer_id = $1`
	if !includeRevoked {
		q += ` AND revoked_at IS NULL`
	}
	q += ` ORDER BY created_at DESC`

	keys, err := ScanRows(ctx, r.db, q, []any{customerID}, func(rows pgx.Rows) (models.CustomerAPIKey, error) {
		return scanCustomerAPIKey(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing customer API keys for customer %s: %w", customerID, err)
	}
	return keys, nil
}

// UpdateLastUsed updates the last_used_at timestamp for a key.
func (r *CustomerAPIKeyRepository) UpdateLastUsed(ctx context.Context, id string) error {
	const q = `UPDATE customer_api_keys SET last_used_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("updating last_used_at for customer API key %s: %w", id, err)
	}
	return nil
}

// Revoke marks a customer API key as revoked.
// Once revoked, a key cannot be used for authentication.
func (r *CustomerAPIKeyRepository) Revoke(ctx context.Context, id, customerID string) error {
	now := time.Now().UTC()
	const q = `UPDATE customer_api_keys SET revoked_at = $1 WHERE id = $2 AND customer_id = $3 AND revoked_at IS NULL`
	tag, err := r.db.Exec(ctx, q, now, id, customerID)
	if err != nil {
		return fmt.Errorf("revoking customer API key %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("revoking customer API key %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// Rotate generates a new key hash for an existing API key, effectively rotating it.
// The old key becomes invalid immediately. Returns the new key ID (same as old).
func (r *CustomerAPIKeyRepository) Rotate(ctx context.Context, id, customerID, newKeyHash string) error {
	const q = `UPDATE customer_api_keys SET key_hash = $1 WHERE id = $2 AND customer_id = $3 AND revoked_at IS NULL`
	tag, err := r.db.Exec(ctx, q, newKeyHash, id, customerID)
	if err != nil {
		return fmt.Errorf("rotating customer API key %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("rotating customer API key %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// UpdatePermissions updates the permissions for an API key.
func (r *CustomerAPIKeyRepository) UpdatePermissions(ctx context.Context, id, customerID string, permissions []string) error {
	const q = `UPDATE customer_api_keys SET permissions = $1 WHERE id = $2 AND customer_id = $3 AND revoked_at IS NULL`
	tag, err := r.db.Exec(ctx, q, permissions, id, customerID)
	if err != nil {
		return fmt.Errorf("updating permissions for customer API key %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating permissions for customer API key %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// Delete permanently removes a customer API key from the database.
// Prefer Revoke for audit trail purposes.
func (r *CustomerAPIKeyRepository) Delete(ctx context.Context, id, customerID string) error {
	const q = `DELETE FROM customer_api_keys WHERE id = $1 AND customer_id = $2`
	tag, err := r.db.Exec(ctx, q, id, customerID)
	if err != nil {
		return fmt.Errorf("deleting customer API key %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting customer API key %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// CountByCustomer returns the number of active API keys for a customer.
func (r *CustomerAPIKeyRepository) CountByCustomer(ctx context.Context, customerID string) (int, error) {
	const q = `SELECT COUNT(*) FROM customer_api_keys WHERE customer_id = $1 AND revoked_at IS NULL`
	return CountRows(ctx, r.db, q, customerID)
}