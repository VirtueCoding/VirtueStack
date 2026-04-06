// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

type SystemWebhookRepository struct {
	db            DB
	encryptionKey string
}

// WithEncryptionKey returns a repository bound to the same database handle but
// using the provided encryption key for secret decode/encode operations.
func (r *SystemWebhookRepository) WithEncryptionKey(encryptionKey string) *SystemWebhookRepository {
	if r == nil {
		return nil
	}
	return &SystemWebhookRepository{
		db:            r.db,
		encryptionKey: encryptionKey,
	}
}

// NewSystemWebhookRepository creates a repository without secret encryption support.
func NewSystemWebhookRepository(db DB) *SystemWebhookRepository {
	return &SystemWebhookRepository{db: db}
}

// NewEncryptedSystemWebhookRepository creates a repository that encrypts stored webhook secrets.
func NewEncryptedSystemWebhookRepository(db DB, encryptionKey string) *SystemWebhookRepository {
	return &SystemWebhookRepository{db: db, encryptionKey: encryptionKey}
}

const systemWebhookSelectCols = `
	id, name, url, secret, events, is_active, created_at, updated_at`

func scanSystemWebhook(row pgx.Row) (models.SystemWebhook, error) {
	var w models.SystemWebhook
	err := row.Scan(
		&w.ID, &w.Name, &w.URL, &w.Secret, &w.Events, &w.IsActive, &w.CreatedAt, &w.UpdatedAt,
	)
	return w, err
}

func (r *SystemWebhookRepository) decodeSecret(webhook *models.SystemWebhook) error {
	decrypted, err := decryptWebhookSecret(webhook.Secret, r.encryptionKey)
	if err != nil {
		return fmt.Errorf("decoding system webhook secret for %s: %w", webhook.ID, err)
	}
	webhook.Secret = decrypted
	return nil
}

func (r *SystemWebhookRepository) Create(ctx context.Context, webhook *models.SystemWebhook) error {
	storedSecret, err := encryptWebhookSecret(webhook.Secret, r.encryptionKey)
	if err != nil {
		return fmt.Errorf("encoding system webhook secret: %w", err)
	}

	const q = `
		INSERT INTO system_webhooks (name, url, secret, events, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + systemWebhookSelectCols

	row := r.db.QueryRow(ctx, q, webhook.Name, webhook.URL, storedSecret, webhook.Events, webhook.IsActive)
	created, err := scanSystemWebhook(row)
	if err != nil {
		return fmt.Errorf("creating system webhook: %w", err)
	}
	if err := r.decodeSecret(&created); err != nil {
		return fmt.Errorf("creating system webhook: %w", err)
	}
	*webhook = created
	return nil
}

func (r *SystemWebhookRepository) List(ctx context.Context) ([]models.SystemWebhook, error) {
	const q = `SELECT ` + systemWebhookSelectCols + ` FROM system_webhooks ORDER BY created_at DESC LIMIT 1000`
	webhooks, err := ScanRows(ctx, r.db, q, nil, func(rows pgx.Rows) (models.SystemWebhook, error) {
		return scanSystemWebhook(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing system webhooks: %w", err)
	}
	for i := range webhooks {
		if err := r.decodeSecret(&webhooks[i]); err != nil {
			return nil, fmt.Errorf("listing system webhooks: %w", err)
		}
	}
	return webhooks, nil
}

func (r *SystemWebhookRepository) ListActiveForEvent(ctx context.Context, eventType string) ([]models.SystemWebhook, error) {
	const q = `SELECT ` + systemWebhookSelectCols + ` FROM system_webhooks WHERE is_active = TRUE AND $1 = ANY(events)`
	webhooks, err := ScanRows(ctx, r.db, q, []any{eventType}, func(rows pgx.Rows) (models.SystemWebhook, error) {
		return scanSystemWebhook(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing active system webhooks for event %s: %w", eventType, err)
	}
	for i := range webhooks {
		if err := r.decodeSecret(&webhooks[i]); err != nil {
			return nil, fmt.Errorf("listing active system webhooks for event %s: %w", eventType, err)
		}
	}
	return webhooks, nil
}

func (r *SystemWebhookRepository) GetByID(ctx context.Context, id string) (*models.SystemWebhook, error) {
	const q = `SELECT ` + systemWebhookSelectCols + ` FROM system_webhooks WHERE id = $1`
	webhook, err := ScanRow(ctx, r.db, q, []any{id}, scanSystemWebhook)
	if err != nil {
		return nil, fmt.Errorf("getting system webhook %s: %w", id, err)
	}
	if err := r.decodeSecret(&webhook); err != nil {
		return nil, fmt.Errorf("getting system webhook %s: %w", id, err)
	}
	return &webhook, nil
}

// Update applies partial updates to a system webhook and returns the updated record.
func (r *SystemWebhookRepository) Update(ctx context.Context, id string, req *models.SystemWebhookUpdateRequest, secret *string) (*models.SystemWebhook, error) {
	updates := []string{}
	args := []any{}
	idx := 1

	if req.Name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", idx))
		args = append(args, *req.Name)
		idx++
	}
	if req.URL != nil {
		updates = append(updates, fmt.Sprintf("url = $%d", idx))
		args = append(args, *req.URL)
		idx++
	}
	if secret != nil {
		storedSecret, err := encryptWebhookSecret(*secret, r.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("encoding system webhook secret: %w", err)
		}
		updates = append(updates, fmt.Sprintf("secret = $%d", idx))
		args = append(args, storedSecret)
		idx++
	}
	if req.Events != nil {
		updates = append(updates, fmt.Sprintf("events = $%d", idx))
		args = append(args, req.Events)
		idx++
	}
	if req.IsActive != nil {
		updates = append(updates, fmt.Sprintf("is_active = $%d", idx))
		args = append(args, *req.IsActive)
		idx++
	}

	if len(updates) == 0 {
		return r.GetByID(ctx, id)
	}

	updates = append(updates, "updated_at = NOW()")
	args = append(args, id)
	q := fmt.Sprintf("UPDATE system_webhooks SET %s WHERE id = $%d RETURNING %s", strings.Join(updates, ", "), idx, systemWebhookSelectCols)

	updated, err := ScanRow(ctx, r.db, q, args, scanSystemWebhook)
	if err != nil {
		return nil, fmt.Errorf("updating system webhook %s: %w", id, err)
	}
	if err := r.decodeSecret(&updated); err != nil {
		return nil, fmt.Errorf("updating system webhook %s: %w", id, err)
	}
	return &updated, nil
}

func (r *SystemWebhookRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM system_webhooks WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting system webhook %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting system webhook %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// EncryptPlaintextSecrets migrates legacy plaintext system webhook secrets to encrypted storage.
func (r *SystemWebhookRepository) EncryptPlaintextSecrets(ctx context.Context) error {
	if r.encryptionKey == "" {
		return nil
	}

	const selectQ = `SELECT id, secret FROM system_webhooks WHERE secret NOT LIKE $1`
	rows, err := r.db.Query(ctx, selectQ, webhookSecretEncryptedPrefix+"%")
	if err != nil {
		return fmt.Errorf("querying legacy system webhook secrets: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var secret string
		if err := rows.Scan(&id, &secret); err != nil {
			return fmt.Errorf("scanning legacy system webhook secret: %w", err)
		}

		encodedSecret, err := encryptWebhookSecret(secret, r.encryptionKey)
		if err != nil {
			return fmt.Errorf("encoding legacy system webhook secret %s: %w", id, err)
		}

		const updateQ = `UPDATE system_webhooks SET secret = $1, updated_at = NOW() WHERE id = $2`
		tag, err := r.db.Exec(ctx, updateQ, encodedSecret, id)
		if err != nil {
			return fmt.Errorf("updating legacy system webhook secret %s: %w", id, err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("updating legacy system webhook secret %s: %w", id, ErrNoRowsAffected)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating legacy system webhook secrets: %w", err)
	}

	return nil
}
