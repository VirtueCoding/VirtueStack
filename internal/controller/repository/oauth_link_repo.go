package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
)

const oauthLinkColumns = `id, customer_id, provider, provider_user_id,
	email, display_name, avatar_url, access_token_encrypted,
	refresh_token_encrypted, token_expires_at, created_at, updated_at`

// OAuthLinkRepository handles database operations for customer_oauth_links.
type OAuthLinkRepository struct {
	db DB
}

// NewOAuthLinkRepository creates a new OAuthLinkRepository.
func NewOAuthLinkRepository(db DB) *OAuthLinkRepository {
	return &OAuthLinkRepository{db: db}
}

// Create inserts a new OAuth link.
func (r *OAuthLinkRepository) Create(
	ctx context.Context, link *models.OAuthLink,
) (*models.OAuthLink, error) {
	query := `INSERT INTO customer_oauth_links (
		customer_id, provider, provider_user_id, email,
		display_name, avatar_url, access_token_encrypted,
		refresh_token_encrypted, token_expires_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	RETURNING ` + oauthLinkColumns

	row := r.db.QueryRow(ctx, query,
		link.CustomerID, link.Provider, link.ProviderUserID,
		link.Email, link.DisplayName, link.AvatarURL,
		link.AccessTokenEncrypted, link.RefreshTokenEncrypted,
		link.TokenExpiresAt,
	)
	return scanOAuthLink(row)
}

// GetByProviderUserID looks up an OAuth link by provider and provider user ID.
func (r *OAuthLinkRepository) GetByProviderUserID(
	ctx context.Context, provider, providerUserID string,
) (*models.OAuthLink, error) {
	query := `SELECT ` + oauthLinkColumns + `
		FROM customer_oauth_links
		WHERE provider = $1 AND provider_user_id = $2`
	row := r.db.QueryRow(ctx, query, provider, providerUserID)
	return scanOAuthLink(row)
}

// GetByCustomerID returns all OAuth links for a customer.
func (r *OAuthLinkRepository) GetByCustomerID(
	ctx context.Context, customerID string,
) ([]*models.OAuthLink, error) {
	query := `SELECT ` + oauthLinkColumns + `
		FROM customer_oauth_links
		WHERE customer_id = $1
		ORDER BY created_at ASC`
	rows, err := r.db.Query(ctx, query, customerID)
	if err != nil {
		return nil, fmt.Errorf("query oauth links: %w", err)
	}
	defer rows.Close()

	var links []*models.OAuthLink
	for rows.Next() {
		link, err := scanOAuthLinkRow(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// Delete removes an OAuth link by customer and provider.
func (r *OAuthLinkRepository) Delete(
	ctx context.Context, customerID, provider string,
) error {
	query := `DELETE FROM customer_oauth_links
		WHERE customer_id = $1 AND provider = $2`
	tag, err := r.db.Exec(ctx, query, customerID, provider)
	if err != nil {
		return fmt.Errorf("delete oauth link: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sharederrors.ErrNotFound
	}
	return nil
}

// UpdateTokens updates the stored OAuth tokens for a link.
func (r *OAuthLinkRepository) UpdateTokens(
	ctx context.Context, id string,
	accessTokenEnc, refreshTokenEnc []byte,
	expiresAt *time.Time,
) error {
	query := `UPDATE customer_oauth_links
		SET access_token_encrypted = $1,
			refresh_token_encrypted = $2,
			token_expires_at = $3,
			updated_at = NOW()
		WHERE id = $4`
	tag, err := r.db.Exec(ctx, query,
		accessTokenEnc, refreshTokenEnc, expiresAt, id)
	if err != nil {
		return fmt.Errorf("update oauth tokens: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sharederrors.ErrNotFound
	}
	return nil
}

// CountByCustomerID returns the number of OAuth links for a customer.
func (r *OAuthLinkRepository) CountByCustomerID(
	ctx context.Context, customerID string,
) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM customer_oauth_links WHERE customer_id = $1`,
		customerID,
	).Scan(&count)
	return count, err
}

func scanOAuthLink(row pgx.Row) (*models.OAuthLink, error) {
	var link models.OAuthLink
	err := row.Scan(
		&link.ID, &link.CustomerID, &link.Provider,
		&link.ProviderUserID, &link.Email, &link.DisplayName,
		&link.AvatarURL, &link.AccessTokenEncrypted,
		&link.RefreshTokenEncrypted, &link.TokenExpiresAt,
		&link.CreatedAt, &link.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, sharederrors.ErrNotFound
		}
		return nil, fmt.Errorf("scan oauth link: %w", err)
	}
	return &link, nil
}

func scanOAuthLinkRow(rows pgx.Rows) (*models.OAuthLink, error) {
	var link models.OAuthLink
	err := rows.Scan(
		&link.ID, &link.CustomerID, &link.Provider,
		&link.ProviderUserID, &link.Email, &link.DisplayName,
		&link.AvatarURL, &link.AccessTokenEncrypted,
		&link.RefreshTokenEncrypted, &link.TokenExpiresAt,
		&link.CreatedAt, &link.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan oauth link row: %w", err)
	}
	return &link, nil
}
