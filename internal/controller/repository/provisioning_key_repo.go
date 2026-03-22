// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// ProvisioningKeyRepository provides database operations for provisioning API keys.
type ProvisioningKeyRepository struct {
	db DB
}

// NewProvisioningKeyRepository creates a new ProvisioningKeyRepository.
func NewProvisioningKeyRepository(db DB) *ProvisioningKeyRepository {
	return &ProvisioningKeyRepository{db: db}
}

// scanProvisioningKey scans a single provisioning key row.
func scanProvisioningKey(row pgx.Row) (models.ProvisioningKey, error) {
	var key models.ProvisioningKey
	var allowedIPs []string

	err := row.Scan(
		&key.ID, &key.Name, &key.KeyHash,
		&allowedIPs, &key.LastUsedAt, &key.RevokedAt,
		&key.CreatedAt, &key.CreatedBy, &key.Description,
	)

	// Convert []string to proper type
	key.AllowedIPs = allowedIPs

	return key, err
}

const provisioningKeySelectCols = `
	id, name, key_hash,
	allowed_ips, last_used_at, revoked_at,
	created_at, created_by, description`

// Create inserts a new provisioning key into the database.
func (r *ProvisioningKeyRepository) Create(ctx context.Context, key *models.ProvisioningKey) error {
	const q = `
		INSERT INTO provisioning_keys (
			id, name, key_hash, allowed_ips, created_by, description
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + provisioningKeySelectCols

	row := r.db.QueryRow(ctx, q,
		key.ID, key.Name, key.KeyHash, key.AllowedIPs, key.CreatedBy, key.Description,
	)

	created, err := scanProvisioningKey(row)
	if err != nil {
		return fmt.Errorf("creating provisioning key: %w", err)
	}
	*key = created
	return nil
}

// GetByID returns a provisioning key by its UUID.
func (r *ProvisioningKeyRepository) GetByID(ctx context.Context, id string) (*models.ProvisioningKey, error) {
	const q = `SELECT ` + provisioningKeySelectCols + ` FROM provisioning_keys WHERE id = $1`
	key, err := ScanRow(ctx, r.db, q, []any{id}, scanProvisioningKey)
	if err != nil {
		return nil, fmt.Errorf("getting provisioning key %s: %w", id, err)
	}
	return &key, nil
}

// GetByHash returns an active provisioning key by its SHA-256 hash.
// This is used by the APIKeyAuth middleware for key validation.
// Returns ErrNotFound if the key doesn't exist or is revoked.
func (r *ProvisioningKeyRepository) GetByHash(ctx context.Context, keyHash string) (*models.ProvisioningKey, error) {
	const q = `SELECT ` + provisioningKeySelectCols + ` FROM provisioning_keys WHERE key_hash = $1 AND revoked_at IS NULL`
	key, err := ScanRow(ctx, r.db, q, []any{keyHash}, scanProvisioningKey)
	if err != nil {
		return nil, fmt.Errorf("getting provisioning key by hash: %w", err)
	}
	return &key, nil
}

// List returns all provisioning keys with optional filters.
// Pagination is intentionally omitted: the total number of provisioning keys is
// expected to be very small (typically single digits) in any deployment, making
// an unbounded query safe for internal administrative use.
func (r *ProvisioningKeyRepository) List(ctx context.Context, includeRevoked bool) ([]models.ProvisioningKey, error) {
	q := `SELECT ` + provisioningKeySelectCols + ` FROM provisioning_keys`
	if !includeRevoked {
		q += ` WHERE revoked_at IS NULL`
	}
	q += ` ORDER BY created_at DESC`

	keys, err := ScanRows(ctx, r.db, q, nil, func(rows pgx.Rows) (models.ProvisioningKey, error) {
		return scanProvisioningKey(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing provisioning keys: %w", err)
	}
	return keys, nil
}

// UpdateLastUsed updates the last_used_at timestamp for a key.
func (r *ProvisioningKeyRepository) UpdateLastUsed(ctx context.Context, id string) error {
	const q = `UPDATE provisioning_keys SET last_used_at = NOW() WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("updating last_used_at for key %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating last_used_at for key %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// Revoke marks a provisioning key as revoked.
func (r *ProvisioningKeyRepository) Revoke(ctx context.Context, id string) error {
	now := time.Now().UTC()
	const q = `UPDATE provisioning_keys SET revoked_at = $1 WHERE id = $2 AND revoked_at IS NULL`
	tag, err := r.db.Exec(ctx, q, now, id)
	if err != nil {
		return fmt.Errorf("revoking provisioning key %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("revoking provisioning key %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// Delete permanently removes a provisioning key from the database.
func (r *ProvisioningKeyRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM provisioning_keys WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting provisioning key %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting provisioning key %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// UpdateAllowedIPs updates the allowed IPs list for a key.
func (r *ProvisioningKeyRepository) UpdateAllowedIPs(ctx context.Context, id string, allowedIPs []string) error {
	const q = `UPDATE provisioning_keys SET allowed_ips = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, allowedIPs, id)
	if err != nil {
		return fmt.Errorf("updating allowed IPs for key %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating allowed IPs for key %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}