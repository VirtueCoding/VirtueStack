package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// PreActionWebhookRepository handles database operations for pre-action webhooks.
type PreActionWebhookRepository struct {
	db DB
}

// NewPreActionWebhookRepository creates a new PreActionWebhookRepository.
func NewPreActionWebhookRepository(db DB) *PreActionWebhookRepository {
	return &PreActionWebhookRepository{db: db}
}

const preActionWebhookSelectCols = `
	id, name, url, secret, events, timeout_ms, fail_open, is_active, created_at, updated_at`

func scanPreActionWebhook(row pgx.Row) (models.PreActionWebhook, error) {
	var w models.PreActionWebhook
	err := row.Scan(
		&w.ID, &w.Name, &w.URL, &w.Secret, &w.Events,
		&w.TimeoutMs, &w.FailOpen, &w.IsActive, &w.CreatedAt, &w.UpdatedAt,
	)
	return w, err
}

// Create inserts a new pre-action webhook.
func (r *PreActionWebhookRepository) Create(ctx context.Context, webhook *models.PreActionWebhook) error {
	const q = `
		INSERT INTO pre_action_webhooks (name, url, secret, events, timeout_ms, fail_open, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + preActionWebhookSelectCols

	row := r.db.QueryRow(ctx, q,
		webhook.Name, webhook.URL, webhook.Secret, webhook.Events,
		webhook.TimeoutMs, webhook.FailOpen, webhook.IsActive,
	)
	created, err := scanPreActionWebhook(row)
	if err != nil {
		return fmt.Errorf("creating pre-action webhook: %w", err)
	}
	*webhook = created
	return nil
}

// List returns all pre-action webhooks ordered by creation date.
func (r *PreActionWebhookRepository) List(ctx context.Context) ([]models.PreActionWebhook, error) {
	const q = `SELECT ` + preActionWebhookSelectCols + ` FROM pre_action_webhooks ORDER BY created_at DESC LIMIT 1000`
	webhooks, err := ScanRows(ctx, r.db, q, nil, func(rows pgx.Rows) (models.PreActionWebhook, error) {
		return scanPreActionWebhook(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing pre-action webhooks: %w", err)
	}
	return webhooks, nil
}

// ListActiveForEvent returns active pre-action webhooks subscribed to a given event.
func (r *PreActionWebhookRepository) ListActiveForEvent(ctx context.Context, eventType string) ([]models.PreActionWebhook, error) {
	const q = `SELECT ` + preActionWebhookSelectCols + ` FROM pre_action_webhooks WHERE is_active = TRUE AND $1 = ANY(events)`
	webhooks, err := ScanRows(ctx, r.db, q, []any{eventType}, func(rows pgx.Rows) (models.PreActionWebhook, error) {
		return scanPreActionWebhook(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing active pre-action webhooks for event %s: %w", eventType, err)
	}
	return webhooks, nil
}

// GetByID returns a pre-action webhook by ID.
func (r *PreActionWebhookRepository) GetByID(ctx context.Context, id string) (*models.PreActionWebhook, error) {
	const q = `SELECT ` + preActionWebhookSelectCols + ` FROM pre_action_webhooks WHERE id = $1`
	w, err := scanPreActionWebhook(r.db.QueryRow(ctx, q, id))
	if err != nil {
		return nil, fmt.Errorf("getting pre-action webhook %s: %w", id, err)
	}
	return &w, nil
}

// Update applies partial updates to a pre-action webhook.
func (r *PreActionWebhookRepository) Update(ctx context.Context, id string, req *models.PreActionWebhookUpdateRequest) (*models.PreActionWebhook, error) {
	sets := []string{}
	args := []any{}
	idx := 1

	if req.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		args = append(args, *req.Name)
		idx++
	}
	if req.URL != nil {
		sets = append(sets, fmt.Sprintf("url = $%d", idx))
		args = append(args, *req.URL)
		idx++
	}
	if req.Secret != nil {
		sets = append(sets, fmt.Sprintf("secret = $%d", idx))
		args = append(args, *req.Secret)
		idx++
	}
	if req.Events != nil {
		sets = append(sets, fmt.Sprintf("events = $%d", idx))
		args = append(args, req.Events)
		idx++
	}
	if req.TimeoutMs != nil {
		sets = append(sets, fmt.Sprintf("timeout_ms = $%d", idx))
		args = append(args, *req.TimeoutMs)
		idx++
	}
	if req.FailOpen != nil {
		sets = append(sets, fmt.Sprintf("fail_open = $%d", idx))
		args = append(args, *req.FailOpen)
		idx++
	}
	if req.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", idx))
		args = append(args, *req.IsActive)
		idx++
	}

	if len(sets) == 0 {
		return r.GetByID(ctx, id)
	}

	sets = append(sets, "updated_at = NOW()")
	args = append(args, id)

	q := fmt.Sprintf(
		"UPDATE pre_action_webhooks SET %s WHERE id = $%d RETURNING %s",
		strings.Join(sets, ", "), idx, preActionWebhookSelectCols,
	)
	w, err := scanPreActionWebhook(r.db.QueryRow(ctx, q, args...))
	if err != nil {
		return nil, fmt.Errorf("updating pre-action webhook %s: %w", id, err)
	}
	return &w, nil
}

// Delete removes a pre-action webhook.
func (r *PreActionWebhookRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM pre_action_webhooks WHERE id = $1`
	ct, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting pre-action webhook %s: %w", id, err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("pre-action webhook %s not found", id)
	}
	return nil
}
