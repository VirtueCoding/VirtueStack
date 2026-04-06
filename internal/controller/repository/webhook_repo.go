// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository/cursor"
)

// WebhookMaxFailCount is the consecutive-failure threshold at which a webhook
// is automatically disabled to prevent continued delivery attempts.
const WebhookMaxFailCount = 50

// WebhookRepository provides database operations for webhooks and their deliveries.
type WebhookRepository struct {
	db DB
}

// NewWebhookRepository creates a new WebhookRepository with the given database connection.
func NewWebhookRepository(db DB) *WebhookRepository {
	return &WebhookRepository{db: db}
}

// Delivery status constants.
const (
	DeliveryStatusPending   = "pending"
	DeliveryStatusDelivered = "delivered"
	DeliveryStatusFailed    = "failed"
	DeliveryStatusRetrying  = "retrying"
)

// WebhookListFilter holds filter parameters for listing webhooks.
type WebhookListFilter struct {
	models.PaginationParams
	CustomerID *string
	Active     *bool
	Event      *string // Filter by event subscription
}

// DeliveryListFilter holds filter parameters for listing deliveries.
type DeliveryListFilter struct {
	models.PaginationParams
	WebhookID *string
	Status    *string
	Event     *string
}

// ============================================================================
// Webhook Operations
// ============================================================================

const webhookSelectCols = `
	id, customer_id, url, secret_hash, events, active, fail_count,
	last_success_at, last_failure_at, created_at, updated_at`

func scanWebhook(row pgx.Row) (models.CustomerWebhook, error) {
	var w models.CustomerWebhook
	err := row.Scan(
		&w.ID, &w.CustomerID, &w.URL, &w.SecretHash, &w.Events, &w.IsActive, &w.FailCount,
		&w.LastSuccessAt, &w.LastFailureAt, &w.CreatedAt, &w.UpdatedAt,
	)
	return w, err
}

// Create inserts a new webhook into the database.
func (r *WebhookRepository) Create(ctx context.Context, webhook *models.CustomerWebhook) error {
	const q = `
		INSERT INTO webhooks (
			customer_id, url, secret_hash, events, active
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + webhookSelectCols

	row := r.db.QueryRow(ctx, q,
		webhook.CustomerID, webhook.URL, webhook.SecretHash, webhook.Events, webhook.IsActive,
	)
	created, err := scanWebhook(row)
	if err != nil {
		return fmt.Errorf("creating webhook: %w", err)
	}
	*webhook = created
	return nil
}

// GetByID returns a webhook by its UUID. Returns ErrNotFound if no webhook matches.
func (r *WebhookRepository) GetByID(ctx context.Context, id string) (*models.CustomerWebhook, error) {
	const q = `SELECT ` + webhookSelectCols + ` FROM webhooks WHERE id = $1`
	webhook, err := ScanRow(ctx, r.db, q, []any{id}, scanWebhook)
	if err != nil {
		return nil, fmt.Errorf("getting webhook %s: %w", id, err)
	}
	return &webhook, nil
}

// GetByIDAndCustomer returns a webhook by ID, ensuring it belongs to the customer.
// Returns ErrNotFound if no webhook matches or if it belongs to a different customer.
func (r *WebhookRepository) GetByIDAndCustomer(ctx context.Context, id, customerID string) (*models.CustomerWebhook, error) {
	const q = `SELECT ` + webhookSelectCols + ` FROM webhooks WHERE id = $1 AND customer_id = $2`
	webhook, err := ScanRow(ctx, r.db, q, []any{id, customerID}, scanWebhook)
	if err != nil {
		return nil, fmt.Errorf("getting webhook %s for customer %s: %w", id, customerID, err)
	}
	return &webhook, nil
}

// List returns a paginated list of webhooks with optional filters.
func (r *WebhookRepository) List(ctx context.Context, filter WebhookListFilter) ([]models.CustomerWebhook, bool, string, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.CustomerID != nil {
		where = append(where, fmt.Sprintf("customer_id = $%d", idx))
		args = append(args, *filter.CustomerID)
		idx++
	}
	if filter.Active != nil {
		where = append(where, fmt.Sprintf("active = $%d", idx))
		args = append(args, *filter.Active)
		idx++
	}
	if filter.Event != nil {
		where = append(where, fmt.Sprintf("$%d = ANY(events)", idx))
		args = append(args, *filter.Event)
		idx++
	}

	clause := strings.Join(where, " AND ")

	cp := cursor.ParseParams(filter.PaginationParams)
	var extraArg any
	clause, idx, extraArg = cursor.BuildWhereClause(clause, cp, true, idx)
	if extraArg != nil {
		args = append(args, extraArg)
	}

	listQ := fmt.Sprintf(
		"SELECT %s FROM webhooks WHERE %s ORDER BY id DESC LIMIT $%d",
		webhookSelectCols, clause, idx,
	)
	args = append(args, filter.PerPage+1)

	webhooks, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.CustomerWebhook, error) {
		return scanWebhook(rows)
	})
	if err != nil {
		return nil, false, "", fmt.Errorf("listing webhooks: %w", err)
	}

	webhooks, hasMore, lastID := cursor.TrimResults(webhooks, filter.PerPage, func(w models.CustomerWebhook) string { return w.ID })
	return webhooks, hasMore, lastID, nil
}

// ListByCustomer returns all webhooks for a specific customer.
func (r *WebhookRepository) ListByCustomer(ctx context.Context, customerID string) ([]models.CustomerWebhook, error) {
	const q = `SELECT ` + webhookSelectCols + ` FROM webhooks WHERE customer_id = $1 ORDER BY created_at DESC`
	webhooks, err := ScanRows(ctx, r.db, q, []any{customerID}, func(rows pgx.Rows) (models.CustomerWebhook, error) {
		return scanWebhook(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing webhooks for customer %s: %w", customerID, err)
	}
	return webhooks, nil
}

// ListActiveForEvent returns all active webhooks subscribed to a specific event.
func (r *WebhookRepository) ListActiveForEvent(ctx context.Context, event string) ([]models.CustomerWebhook, error) {
	const q = `SELECT ` + webhookSelectCols + ` FROM webhooks WHERE active = TRUE AND $1 = ANY(events)`
	webhooks, err := ScanRows(ctx, r.db, q, []any{event}, func(rows pgx.Rows) (models.CustomerWebhook, error) {
		return scanWebhook(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing active webhooks for event %s: %w", event, err)
	}
	return webhooks, nil
}

// Update updates a webhook's URL, events, active status, and optional secret hash.
func (r *WebhookRepository) Update(ctx context.Context, id string, url *string, events []string, active *bool, secretHash *string) error {
	// Build dynamic update query
	updates := []string{}
	args := []any{}
	idx := 1

	if url != nil {
		updates = append(updates, fmt.Sprintf("url = $%d", idx))
		args = append(args, *url)
		idx++
	}
	if events != nil {
		updates = append(updates, fmt.Sprintf("events = $%d", idx))
		args = append(args, events)
		idx++
	}
	if active != nil {
		updates = append(updates, fmt.Sprintf("active = $%d", idx))
		args = append(args, *active)
		idx++
	}
	if secretHash != nil {
		updates = append(updates, fmt.Sprintf("secret_hash = $%d", idx))
		args = append(args, *secretHash)
		idx++
	}

	if len(updates) == 0 {
		return nil // Nothing to update
	}

	args = append(args, id)
	q := fmt.Sprintf("UPDATE webhooks SET %s WHERE id = $%d", strings.Join(updates, ", "), idx)

	tag, err := r.db.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("updating webhook %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating webhook %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// Delete removes a webhook from the database.
func (r *WebhookRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM webhooks WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting webhook %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting webhook %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// DeleteByCustomer removes a webhook, ensuring it belongs to the customer.
func (r *WebhookRepository) DeleteByCustomer(ctx context.Context, id, customerID string) error {
	const q = `DELETE FROM webhooks WHERE id = $1 AND customer_id = $2`
	tag, err := r.db.Exec(ctx, q, id, customerID)
	if err != nil {
		return fmt.Errorf("deleting webhook %s for customer %s: %w", id, customerID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting webhook %s for customer %s: %w", id, customerID, ErrNoRowsAffected)
	}
	return nil
}

// WebhookDeliveryStatusResult reports the post-update webhook delivery counters and transition state.
type WebhookDeliveryStatusResult struct {
	FailCount        int
	Active           bool
	DisabledByUpdate bool
}

// UpdateDeliveryStatus updates the webhook's delivery status after an attempt.
// If success is true, resets fail_count. If false, increments fail_count.
// Auto-disables webhook if fail_count reaches WebhookMaxFailCount and reports whether this update performed that transition.
func (r *WebhookRepository) UpdateDeliveryStatus(ctx context.Context, id string, success bool) (*WebhookDeliveryStatusResult, error) {
	if success {
		const q = `
			UPDATE webhooks
			SET fail_count = 0,
			    last_success_at = NOW()
			WHERE id = $1
			RETURNING fail_count, active, FALSE`
		result, err := ScanRow(ctx, r.db, q, []any{id}, func(row pgx.Row) (WebhookDeliveryStatusResult, error) {
			var result WebhookDeliveryStatusResult
			err := row.Scan(&result.FailCount, &result.Active, &result.DisabledByUpdate)
			return result, err
		})
		if err != nil {
			return nil, fmt.Errorf("updating webhook %s delivery status: %w", id, err)
		}
		return &result, nil
	}

	// WebhookMaxFailCount is passed as a bind parameter ($2) to avoid embedding
	// a constant directly into the SQL string via fmt.Sprintf.
	const q = `
		WITH prior AS (
			SELECT active
			FROM webhooks
			WHERE id = $1
			FOR UPDATE
		),
		updated AS (
			UPDATE webhooks
			SET fail_count = fail_count + 1,
			    last_failure_at = NOW(),
			    active = CASE WHEN fail_count + 1 >= $2 THEN FALSE ELSE active END
			FROM prior
			WHERE webhooks.id = $1
			RETURNING webhooks.fail_count, webhooks.active, prior.active AS was_active
		)
		SELECT fail_count, active, (NOT active AND was_active) AS disabled_by_update
		FROM updated`
	result, err := ScanRow(ctx, r.db, q, []any{id, WebhookMaxFailCount}, func(row pgx.Row) (WebhookDeliveryStatusResult, error) {
		var result WebhookDeliveryStatusResult
		err := row.Scan(&result.FailCount, &result.Active, &result.DisabledByUpdate)
		return result, err
	})
	if err != nil {
		return nil, fmt.Errorf("updating webhook %s delivery status: %w", id, err)
	}
	return &result, nil
}

// CountByCustomer returns the number of webhooks for a customer.
func (r *WebhookRepository) CountByCustomer(ctx context.Context, customerID string) (int, error) {
	const q = `SELECT COUNT(*) FROM webhooks WHERE customer_id = $1`
	return CountRows(ctx, r.db, q, customerID)
}

// ============================================================================
// Delivery Operations
// ============================================================================

const deliverySelectCols = `
	id, webhook_id, event, idempotency_key, payload, status, attempt_count,
	max_attempts, next_retry_at, response_status, response_body, error_message,
	delivered_at, created_at, updated_at`

func scanDelivery(row pgx.Row) (models.WebhookDelivery, error) {
	var d models.WebhookDelivery
	err := row.Scan(
		&d.ID, &d.WebhookID, &d.Event, &d.IdempotencyKey, &d.Payload, &d.Status, &d.AttemptCount,
		&d.MaxAttempts, &d.NextRetryAt, &d.ResponseStatus, &d.ResponseBody, &d.ErrorMessage,
		&d.DeliveredAt, &d.CreatedAt, &d.UpdatedAt,
	)
	return d, err
}

// CreateDelivery inserts a new delivery record into the database.
func (r *WebhookRepository) CreateDelivery(ctx context.Context, delivery *models.WebhookDelivery) error {
	const q = `
		INSERT INTO webhook_deliveries (
			webhook_id, event, idempotency_key, payload, status, max_attempts
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + deliverySelectCols

	row := r.db.QueryRow(ctx, q,
		delivery.WebhookID, delivery.Event, delivery.IdempotencyKey, delivery.Payload,
		delivery.Status, delivery.MaxAttempts,
	)
	created, err := scanDelivery(row)
	if err != nil {
		return fmt.Errorf("creating webhook delivery: %w", err)
	}
	*delivery = created
	return nil
}

// GetDeliveryByID returns a delivery by its UUID.
func (r *WebhookRepository) GetDeliveryByID(ctx context.Context, id string) (*models.WebhookDelivery, error) {
	const q = `SELECT ` + deliverySelectCols + ` FROM webhook_deliveries WHERE id = $1`
	delivery, err := ScanRow(ctx, r.db, q, []any{id}, scanDelivery)
	if err != nil {
		return nil, fmt.Errorf("getting delivery %s: %w", id, err)
	}
	return &delivery, nil
}

// GetDeliveryByIdempotencyKey returns a delivery by its idempotency key.
func (r *WebhookRepository) GetDeliveryByIdempotencyKey(ctx context.Context, key string) (*models.WebhookDelivery, error) {
	const q = `SELECT ` + deliverySelectCols + ` FROM webhook_deliveries WHERE idempotency_key = $1`
	delivery, err := ScanRow(ctx, r.db, q, []any{key}, scanDelivery)
	if err != nil {
		return nil, fmt.Errorf("getting delivery by idempotency key %s: %w", key, err)
	}
	return &delivery, nil
}

// ListDeliveries returns a paginated list of deliveries with optional filters.
func (r *WebhookRepository) ListDeliveries(ctx context.Context, filter DeliveryListFilter) ([]models.WebhookDelivery, bool, string, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.WebhookID != nil {
		where = append(where, fmt.Sprintf("webhook_id = $%d", idx))
		args = append(args, *filter.WebhookID)
		idx++
	}
	if filter.Status != nil {
		where = append(where, fmt.Sprintf("status = $%d", idx))
		args = append(args, *filter.Status)
		idx++
	}
	if filter.Event != nil {
		where = append(where, fmt.Sprintf("event = $%d", idx))
		args = append(args, *filter.Event)
		idx++
	}

	clause := strings.Join(where, " AND ")

	cp := cursor.ParseParams(filter.PaginationParams)
	var extraArg any
	clause, idx, extraArg = cursor.BuildWhereClause(clause, cp, true, idx)
	if extraArg != nil {
		args = append(args, extraArg)
	}

	listQ := fmt.Sprintf(
		"SELECT %s FROM webhook_deliveries WHERE %s ORDER BY id DESC LIMIT $%d",
		deliverySelectCols, clause, idx,
	)
	args = append(args, filter.PerPage+1)

	deliveries, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.WebhookDelivery, error) {
		return scanDelivery(rows)
	})
	if err != nil {
		return nil, false, "", fmt.Errorf("listing deliveries: %w", err)
	}

	deliveries, hasMore, lastID := cursor.TrimResults(deliveries, filter.PerPage, func(d models.WebhookDelivery) string { return d.ID })
	return deliveries, hasMore, lastID, nil
}

// ListDeliveriesByWebhook returns deliveries for a specific webhook with cursor-based pagination.
func (r *WebhookRepository) ListDeliveriesByWebhook(ctx context.Context, webhookID string, perPage int, cursorStr string) ([]models.WebhookDelivery, bool, string, error) {
	where := "webhook_id = $1"
	args := []any{webhookID}
	idx := 2

	pp := models.PaginationParams{PerPage: perPage, Cursor: cursorStr}
	cp := pp.DecodeCursor()
	if cp.LastID != "" {
		where += fmt.Sprintf(" AND id < $%d", idx)
		args = append(args, cp.LastID)
		idx++
	}

	listQ := fmt.Sprintf(
		"SELECT %s FROM webhook_deliveries WHERE %s ORDER BY id DESC LIMIT $%d",
		deliverySelectCols, where, idx,
	)
	args = append(args, perPage+1)

	deliveries, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.WebhookDelivery, error) {
		return scanDelivery(rows)
	})
	if err != nil {
		return nil, false, "", fmt.Errorf("listing deliveries for webhook %s: %w", webhookID, err)
	}

	hasMore := len(deliveries) > perPage
	if hasMore {
		deliveries = deliveries[:perPage]
	}
	var lastID string
	if len(deliveries) > 0 {
		lastID = deliveries[len(deliveries)-1].ID
	}
	return deliveries, hasMore, lastID, nil
}

// UpdateDeliveryAttempt updates a delivery after an attempt.
// The update is conditional on the active delivery lease so a stale worker
// cannot overwrite a newer terminal state after another worker re-claims it.
func (r *WebhookRepository) UpdateDeliveryAttempt(ctx context.Context, id string, claimedLeaseUntil time.Time, success bool, responseStatus int, responseBody, errMsg string, nextRetryAt *time.Time) error {
	var q string
	var args []any

	if success {
		q = `
			UPDATE webhook_deliveries
			SET status = $4,
			    attempt_count = attempt_count + 1,
			    response_status = $5,
			    response_body = $6,
			    delivered_at = NOW(),
			    next_retry_at = NULL
			WHERE id = $1
			  AND status = $2
			  AND next_retry_at = $3`
		args = []any{id, DeliveryStatusRetrying, claimedLeaseUntil, DeliveryStatusDelivered, responseStatus, responseBody}
	} else {
		q = `
			UPDATE webhook_deliveries
			SET status = CASE WHEN attempt_count + 1 >= max_attempts THEN $4 ELSE $5 END,
			    attempt_count = attempt_count + 1,
			    response_status = $6,
			    response_body = $7,
			    error_message = $8,
			    next_retry_at = $9
			WHERE id = $1
			  AND status = $2
			  AND next_retry_at = $3`
		args = []any{id, DeliveryStatusRetrying, claimedLeaseUntil, DeliveryStatusFailed, DeliveryStatusRetrying, responseStatus, responseBody, errMsg, nextRetryAt}
	}

	tag, err := r.db.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("updating delivery %s attempt: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating delivery %s attempt: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// MarkDeliveryFailed marks a delivery as permanently failed.
// The update is conditional on the active delivery lease so a stale worker
// cannot overwrite a newer terminal state after another worker re-claims it.
func (r *WebhookRepository) MarkDeliveryFailed(ctx context.Context, id string, claimedLeaseUntil time.Time, errMsg string) error {
	const q = `
		UPDATE webhook_deliveries
		SET status = $4,
		    error_message = $5,
		    next_retry_at = NULL
		WHERE id = $1
		  AND status = $2
		  AND next_retry_at = $3`

	tag, err := r.db.Exec(ctx, q, id, DeliveryStatusRetrying, claimedLeaseUntil, DeliveryStatusFailed, errMsg)
	if err != nil {
		return fmt.Errorf("marking delivery %s failed: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("marking delivery %s failed: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// GetPendingDeliveries returns deliveries that are ready to be processed.
// This includes pending deliveries and retries that are due.
func (r *WebhookRepository) GetPendingDeliveries(ctx context.Context, limit int) ([]models.WebhookDelivery, error) {
	const q = `
		SELECT ` + deliverySelectCols + `
		FROM webhook_deliveries
		WHERE status IN ('pending', 'retrying')
		  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		ORDER BY created_at ASC
		LIMIT $1`

	deliveries, err := ScanRows(ctx, r.db, q, []any{limit}, func(rows pgx.Rows) (models.WebhookDelivery, error) {
		return scanDelivery(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("getting pending deliveries: %w", err)
	}
	return deliveries, nil
}

// ClaimDelivery atomically leases a due delivery so only one worker can process it.
// A leased delivery is marked retrying and hidden from other processors until nextRetryAt.
func (r *WebhookRepository) ClaimDelivery(ctx context.Context, id string, nextRetryAt time.Time) (*models.WebhookDelivery, error) {
	const q = `
		UPDATE webhook_deliveries
		SET status = $2,
		    next_retry_at = $3,
		    updated_at = NOW()
		WHERE id = $1
		  AND status IN ('pending', 'retrying')
		  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		RETURNING ` + deliverySelectCols

	delivery, err := ScanRow(ctx, r.db, q, []any{id, DeliveryStatusRetrying, nextRetryAt}, scanDelivery)
	if err != nil {
		return nil, fmt.Errorf("claiming delivery %s: %w", id, err)
	}

	return &delivery, nil
}

// ClaimPendingDeliveries atomically leases due deliveries so concurrent processors
// cannot select and send the same row twice.
func (r *WebhookRepository) ClaimPendingDeliveries(ctx context.Context, limit int, nextRetryAt time.Time) ([]models.WebhookDelivery, error) {
	const q = `
		WITH due AS (
			SELECT id
			FROM webhook_deliveries
			WHERE status IN ('pending', 'retrying')
			  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
			ORDER BY created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE webhook_deliveries AS wd
		SET status = $2,
		    next_retry_at = $3,
		    updated_at = NOW()
		FROM due
		WHERE wd.id = due.id
		RETURNING
			wd.id, wd.webhook_id, wd.event, wd.idempotency_key, wd.payload,
			wd.status, wd.attempt_count, wd.max_attempts, wd.next_retry_at,
			wd.response_status, wd.response_body, wd.error_message,
			wd.delivered_at, wd.created_at, wd.updated_at`

	deliveries, err := ScanRows(ctx, r.db, q, []any{limit, DeliveryStatusRetrying, nextRetryAt}, func(rows pgx.Rows) (models.WebhookDelivery, error) {
		return scanDelivery(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("claiming pending deliveries: %w", err)
	}

	return deliveries, nil
}

func (r *WebhookRepository) ResetDeliveryForRetry(ctx context.Context, id string) error {
	const q = `
		UPDATE webhook_deliveries
		SET status = $2,
		    next_retry_at = NULL,
		    error_message = '',
		    updated_at = NOW()
		WHERE id = $1`

	tag, err := r.db.Exec(ctx, q, id, DeliveryStatusPending)
	if err != nil {
		return fmt.Errorf("resetting delivery %s for retry: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("resetting delivery %s for retry: %w", id, ErrNoRowsAffected)
	}
	return nil
}

func (r *WebhookRepository) GetPendingRetries(ctx context.Context, before time.Time) ([]models.WebhookDelivery, error) {
	const q = `
		SELECT ` + deliverySelectCols + `
		FROM webhook_deliveries
		WHERE status = $1
		  AND next_retry_at IS NOT NULL
		  AND next_retry_at <= $2
		ORDER BY next_retry_at ASC`

	deliveries, err := ScanRows(ctx, r.db, q, []any{DeliveryStatusRetrying, before}, func(rows pgx.Rows) (models.WebhookDelivery, error) {
		return scanDelivery(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("getting pending retries: %w", err)
	}
	return deliveries, nil
}

// DeliveryStatusCount holds the count of deliveries for a single status value.
type DeliveryStatusCount struct {
	Status string
	Count  int
}

// CountDeliveriesByStatus returns the total delivery count and per-status counts
// for the given webhook using a single DB aggregation query (COUNT(*) GROUP BY status).
// This avoids fetching individual delivery rows for statistical purposes.
func (r *WebhookRepository) CountDeliveriesByStatus(ctx context.Context, webhookID string) (total int, counts []DeliveryStatusCount, err error) {
	const q = `
		SELECT status, COUNT(*) AS cnt
		FROM webhook_deliveries
		WHERE webhook_id = $1
		GROUP BY status`

	rows, queryErr := r.db.Query(ctx, q, webhookID)
	if queryErr != nil {
		return 0, nil, fmt.Errorf("counting deliveries by status for webhook %s: %w", webhookID, queryErr)
	}
	defer rows.Close()

	for rows.Next() {
		var sc DeliveryStatusCount
		if scanErr := rows.Scan(&sc.Status, &sc.Count); scanErr != nil {
			return 0, nil, fmt.Errorf("scanning delivery status count: %w", scanErr)
		}
		counts = append(counts, sc)
		total += sc.Count
	}
	if rowErr := rows.Err(); rowErr != nil {
		return 0, nil, fmt.Errorf("iterating delivery status counts: %w", rowErr)
	}
	return total, counts, nil
}
