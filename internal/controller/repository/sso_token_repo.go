package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
)

// SSOTokenRepository manages server-side opaque SSO tokens.
type SSOTokenRepository struct {
	db DB
}

func NewSSOTokenRepository(db DB) *SSOTokenRepository {
	return &SSOTokenRepository{db: db}
}

func (r *SSOTokenRepository) Create(ctx context.Context, token *models.SSOToken) error {
	const q = `
		INSERT INTO sso_tokens (token_hash, customer_id, vm_id, redirect_path, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`
	if err := r.db.QueryRow(ctx, q,
		token.TokenHash,
		token.CustomerID,
		token.VMID,
		token.RedirectPath,
		token.ExpiresAt,
	).Scan(&token.ID, &token.CreatedAt); err != nil {
		return fmt.Errorf("creating sso token: %w", err)
	}
	return nil
}

// ConsumeByHash atomically consumes a valid token and returns its payload.
func (r *SSOTokenRepository) ConsumeByHash(ctx context.Context, tokenHash []byte) (*models.SSOToken, error) {
	const q = `
		DELETE FROM sso_tokens
		WHERE token_hash = $1
		  AND expires_at > NOW()
		RETURNING id, customer_id, vm_id, redirect_path, created_at, expires_at`
	token := &models.SSOToken{TokenHash: tokenHash}
	var vmID *string
	err := r.db.QueryRow(ctx, q, tokenHash).Scan(
		&token.ID,
		&token.CustomerID,
		&vmID,
		&token.RedirectPath,
		&token.CreatedAt,
		&token.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, sharederrors.ErrNotFound
		}
		return nil, fmt.Errorf("consuming sso token: %w", err)
	}
	token.VMID = vmID
	return token, nil
}

func (r *SSOTokenRepository) DeleteExpired(ctx context.Context) (int64, error) {
	result, err := r.db.Exec(ctx, `DELETE FROM sso_tokens WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("deleting expired sso tokens: %w", err)
	}
	return result.RowsAffected(), nil
}
