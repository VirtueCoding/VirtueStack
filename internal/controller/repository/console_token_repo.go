package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ConsoleTokenRepository manages console token persistence.
type ConsoleTokenRepository struct {
	db DB
}

// NewConsoleTokenRepository creates a new console token repository.
func NewConsoleTokenRepository(db *pgxpool.Pool) *ConsoleTokenRepository {
	return &ConsoleTokenRepository{db: db}
}

// Create inserts a new console token.
func (r *ConsoleTokenRepository) Create(ctx context.Context, token *models.ConsoleToken) error {
	query := `
		INSERT INTO console_tokens (token_hash, user_id, user_type, vm_id, console_type, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.Exec(ctx, query,
		token.TokenHash, token.UserID, token.UserType,
		token.VMID, token.ConsoleType, token.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("creating console token: %w", err)
	}
	return nil
}

// ConsumeByHash atomically finds and deletes a token.
// Returns the token if valid, or ErrNotFound if it doesn't exist, is expired, or already consumed.
// Uses DELETE...RETURNING for atomic single-use semantics.
func (r *ConsoleTokenRepository) ConsumeByHash(ctx context.Context, tokenHash []byte, vmID, consoleType string) (*models.ConsoleToken, error) {
	query := `
		DELETE FROM console_tokens
		WHERE token_hash = $1
		  AND vm_id = $2
		  AND console_type = $3
		  AND expires_at > NOW()
		RETURNING id, user_id, user_type, created_at, expires_at
	`
	token := &models.ConsoleToken{
		TokenHash:   tokenHash,
		VMID:        vmID,
		ConsoleType: consoleType,
	}
	err := r.db.QueryRow(ctx, query, tokenHash, vmID, consoleType).Scan(
		&token.ID, &token.UserID, &token.UserType, &token.CreatedAt, &token.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, sharederrors.ErrNotFound
		}
		return nil, fmt.Errorf("consuming console token: %w", err)
	}
	return token, nil
}

// DeleteExpired removes expired tokens (cleanup job).
func (r *ConsoleTokenRepository) DeleteExpired(ctx context.Context) (int64, error) {
	result, err := r.db.Exec(ctx, `DELETE FROM console_tokens WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("deleting expired console tokens: %w", err)
	}
	return result.RowsAffected(), nil
}

// DeleteByVMID removes all tokens for a VM (called on VM deletion).
func (r *ConsoleTokenRepository) DeleteByVMID(ctx context.Context, vmID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM console_tokens WHERE vm_id = $1`, vmID)
	if err != nil {
		return fmt.Errorf("deleting console tokens for VM %s: %w", vmID, err)
	}
	return nil
}