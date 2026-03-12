// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	apierrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// NotificationPreferenceRepository provides database operations for notification preferences.
type NotificationPreferenceRepository struct {
	db DB
}

// NewNotificationPreferenceRepository creates a new NotificationPreferenceRepository.
func NewNotificationPreferenceRepository(db DB) *NotificationPreferenceRepository {
	return &NotificationPreferenceRepository{db: db}
}

const notificationPreferenceSelectCols = `
	id, customer_id, email_enabled, telegram_enabled, events, created_at, updated_at`

// scanNotificationPreference scans a single notification preference row.
func scanNotificationPreference(row pgx.Row) (models.NotificationPreferences, error) {
	var p models.NotificationPreferences
	err := row.Scan(
		&p.ID, &p.CustomerID, &p.EmailEnabled, &p.TelegramEnabled,
		&p.Events, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

// GetByCustomerID returns notification preferences for a customer.
// Returns ErrNotFound if no preferences exist for the customer.
func (r *NotificationPreferenceRepository) GetByCustomerID(ctx context.Context, customerID string) (*models.NotificationPreferences, error) {
	const q = `SELECT ` + notificationPreferenceSelectCols + ` FROM notification_preferences WHERE customer_id = $1`
	prefs, err := ScanRow(ctx, r.db, q, []any{customerID}, scanNotificationPreference)
	if err != nil {
		return nil, fmt.Errorf("getting notification preferences for customer %s: %w", customerID, err)
	}
	return &prefs, nil
}

// Create creates new notification preferences for a customer.
func (r *NotificationPreferenceRepository) Create(ctx context.Context, prefs *models.NotificationPreferences) error {
	const q = `
		INSERT INTO notification_preferences (
			customer_id, email_enabled, telegram_enabled, events
		) VALUES ($1, $2, $3, $4)
		RETURNING ` + notificationPreferenceSelectCols

	row := r.db.QueryRow(ctx, q,
		prefs.CustomerID, prefs.EmailEnabled, prefs.TelegramEnabled, prefs.Events,
	)
	created, err := scanNotificationPreference(row)
	if err != nil {
		return fmt.Errorf("creating notification preferences: %w", err)
	}
	*prefs = created
	return nil
}

// Update updates notification preferences for a customer.
func (r *NotificationPreferenceRepository) Update(ctx context.Context, prefs *models.NotificationPreferences) error {
	const q = `
		UPDATE notification_preferences SET
			email_enabled = $1,
			telegram_enabled = $2,
			events = $3,
			updated_at = NOW()
		WHERE customer_id = $4
		RETURNING ` + notificationPreferenceSelectCols

	row := r.db.QueryRow(ctx, q,
		prefs.EmailEnabled, prefs.TelegramEnabled, prefs.Events, prefs.CustomerID,
	)
	updated, err := scanNotificationPreference(row)
	if err != nil {
		return fmt.Errorf("updating notification preferences: %w", err)
	}
	*prefs = updated
	return nil
}

// Upsert creates or updates notification preferences for a customer.
func (r *NotificationPreferenceRepository) Upsert(ctx context.Context, prefs *models.NotificationPreferences) error {
	const q = `
		INSERT INTO notification_preferences (
			customer_id, email_enabled, telegram_enabled, events
		) VALUES ($1, $2, $3, $4)
		ON CONFLICT (customer_id) DO UPDATE SET
			email_enabled = EXCLUDED.email_enabled,
			telegram_enabled = EXCLUDED.telegram_enabled,
			events = EXCLUDED.events,
			updated_at = NOW()
		RETURNING ` + notificationPreferenceSelectCols

	row := r.db.QueryRow(ctx, q,
		prefs.CustomerID, prefs.EmailEnabled, prefs.TelegramEnabled, prefs.Events,
	)
	upserted, err := scanNotificationPreference(row)
	if err != nil {
		return fmt.Errorf("upserting notification preferences: %w", err)
	}
	*prefs = upserted
	return nil
}

// Delete removes notification preferences for a customer.
func (r *NotificationPreferenceRepository) Delete(ctx context.Context, customerID string) error {
	const q = `DELETE FROM notification_preferences WHERE customer_id = $1`
	tag, err := r.db.Exec(ctx, q, customerID)
	if err != nil {
		return fmt.Errorf("deleting notification preferences: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoRowsAffected
	}
	return nil
}

// GetOrCreate returns notification preferences for a customer, creating defaults if not found.
func (r *NotificationPreferenceRepository) GetOrCreate(ctx context.Context, customerID string) (*models.NotificationPreferences, error) {
	prefs, err := r.GetByCustomerID(ctx, customerID)
	if err == nil {
		return prefs, nil
	}

	// Check if it's a "not found" error
	if errors.Is(err, apierrors.ErrNotFound) {
		// Create default preferences
		prefs = &models.NotificationPreferences{
			CustomerID:      customerID,
			EmailEnabled:    true,
			TelegramEnabled: false,
			Events:          []string{"vm.created", "vm.deleted", "vm.suspended", "backup.failed"},
		}
		if createErr := r.Create(ctx, prefs); createErr != nil {
			return nil, fmt.Errorf("creating default notification preferences: %w", createErr)
		}
		return prefs, nil
	}

	return nil, err
}

// NotificationEventRepository provides database operations for notification events.
type NotificationEventRepository struct {
	db DB
}

// NewNotificationEventRepository creates a new NotificationEventRepository.
func NewNotificationEventRepository(db DB) *NotificationEventRepository {
	return &NotificationEventRepository{db: db}
}

const notificationEventSelectCols = `
	id, customer_id, event_type, resource_type, resource_id, data, status, error, created_at`

// scanNotificationEvent scans a single notification event row.
func scanNotificationEvent(row pgx.Row) (models.NotificationEvent, error) {
	var e models.NotificationEvent
	err := row.Scan(
		&e.ID, &e.CustomerID, &e.EventType, &e.ResourceType,
		&e.ResourceID, &e.Data, &e.Status, &e.Error, &e.CreatedAt,
	)
	return e, err
}

// Create logs a notification event.
func (r *NotificationEventRepository) Create(ctx context.Context, event *models.NotificationEvent) error {
	const q = `
		INSERT INTO notification_events (
			customer_id, event_type, resource_type, resource_id, data, status, error
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + notificationEventSelectCols

	row := r.db.QueryRow(ctx, q,
		event.CustomerID, event.EventType, event.ResourceType,
		event.ResourceID, event.Data, event.Status, event.Error,
	)
	created, err := scanNotificationEvent(row)
	if err != nil {
		return fmt.Errorf("creating notification event: %w", err)
	}
	*event = created
	return nil
}

// ListByCustomer returns notification events for a customer with pagination.
func (r *NotificationEventRepository) ListByCustomer(ctx context.Context, customerID string, filter NotificationEventFilter) ([]models.NotificationEvent, int, error) {
	where := "customer_id = $1"
	args := []any{customerID}
	idx := 2

	if filter.EventType != nil {
		where += fmt.Sprintf(" AND event_type = $%d", idx)
		args = append(args, *filter.EventType)
		idx++
	}
	if filter.Status != nil {
		where += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, *filter.Status)
		idx++
	}

	countQ := "SELECT COUNT(*) FROM notification_events WHERE " + where
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting notification events: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM notification_events WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		notificationEventSelectCols, where, idx, idx+1,
	)
	args = append(args, limit, offset)

	events, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.NotificationEvent, error) {
		return scanNotificationEvent(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing notification events: %w", err)
	}
	return events, total, nil
}

// NotificationEventFilter holds query parameters for filtering notification events.
type NotificationEventFilter struct {
	EventType *string `form:"event_type"`
	Status    *string `form:"status"`
	models.PaginationParams
}

// notificationRepoSentinel is unused, keeps the declaration section clean.
