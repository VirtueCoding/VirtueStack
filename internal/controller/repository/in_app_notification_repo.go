package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// InAppNotificationRepository provides database operations for in-app notifications.
type InAppNotificationRepository struct {
	db DB
}

// NewInAppNotificationRepository creates a new InAppNotificationRepository.
func NewInAppNotificationRepository(db DB) *InAppNotificationRepository {
	return &InAppNotificationRepository{db: db}
}

const inAppNotifSelectCols = `id, customer_id, admin_id, type, title, message, data, read, created_at`

func scanInAppNotification(row pgx.Row) (models.InAppNotification, error) {
	var n models.InAppNotification
	var data []byte
	err := row.Scan(&n.ID, &n.CustomerID, &n.AdminID, &n.Type, &n.Title, &n.Message, &data, &n.Read, &n.CreatedAt)
	if err != nil {
		return n, err
	}
	n.Data = json.RawMessage(data)
	return n, nil
}

func scanInAppNotificationRow(rows pgx.Rows) (models.InAppNotification, error) {
	var n models.InAppNotification
	var data []byte
	err := rows.Scan(&n.ID, &n.CustomerID, &n.AdminID, &n.Type, &n.Title, &n.Message, &data, &n.Read, &n.CreatedAt)
	if err != nil {
		return n, err
	}
	n.Data = json.RawMessage(data)
	return n, nil
}

// Create inserts a new in-app notification and populates ID + CreatedAt.
func (r *InAppNotificationRepository) Create(ctx context.Context, n *models.InAppNotification) error {
	if n.Data == nil {
		n.Data = json.RawMessage("{}")
	}
	const q = `INSERT INTO notifications (customer_id, admin_id, type, title, message, data)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + inAppNotifSelectCols
	created, err := ScanRow(ctx, r.db, q,
		[]any{n.CustomerID, n.AdminID, n.Type, n.Title, n.Message, []byte(n.Data)},
		scanInAppNotification,
	)
	if err != nil {
		return fmt.Errorf("creating in-app notification: %w", err)
	}
	*n = created
	return nil
}

// ListByCustomer returns notifications for a customer with cursor-based pagination.
func (r *InAppNotificationRepository) ListByCustomer(
	ctx context.Context, customerID string, unreadOnly bool, cursor string, perPage int,
) ([]models.InAppNotification, bool, error) {
	return r.listByRecipient(ctx, "customer_id", customerID, unreadOnly, cursor, perPage)
}

// ListByAdmin returns notifications for an admin with cursor-based pagination.
func (r *InAppNotificationRepository) ListByAdmin(
	ctx context.Context, adminID string, unreadOnly bool, cursor string, perPage int,
) ([]models.InAppNotification, bool, error) {
	return r.listByRecipient(ctx, "admin_id", adminID, unreadOnly, cursor, perPage)
}

func (r *InAppNotificationRepository) listByRecipient(
	ctx context.Context, col, id string, unreadOnly bool, cursor string, perPage int,
) ([]models.InAppNotification, bool, error) {
	q := `SELECT ` + inAppNotifSelectCols + ` FROM notifications WHERE ` + col + ` = $1`
	args := []any{id}
	argIdx := 2

	if unreadOnly {
		q += ` AND NOT read`
	}
	if cursor != "" {
		decodedCursor := models.PaginationParams{Cursor: cursor}.DecodeCursor()
		if decodedCursor.LastID != "" {
			cursor = decodedCursor.LastID
		}
		q += fmt.Sprintf(` AND created_at < $%d`, argIdx)
		args = append(args, cursor)
		argIdx++
	}
	_ = argIdx

	q += ` ORDER BY created_at DESC LIMIT $` + fmt.Sprintf("%d", len(args)+1)
	args = append(args, perPage+1)

	results, err := ScanRows(ctx, r.db, q, args, scanInAppNotificationRow)
	if err != nil {
		return nil, false, fmt.Errorf("listing notifications: %w", err)
	}

	hasMore := len(results) > perPage
	if hasMore {
		results = results[:perPage]
	}
	return results, hasMore, nil
}

// MarkAsRead marks a single notification as read, verifying ownership.
func (r *InAppNotificationRepository) MarkAsRead(ctx context.Context, id, customerID, adminID string) error {
	q := `UPDATE notifications SET read = TRUE WHERE id = $1`
	args := []any{id}
	if customerID != "" {
		q += ` AND customer_id = $2`
		args = append(args, customerID)
	} else {
		q += ` AND admin_id = $2`
		args = append(args, adminID)
	}
	tag, err := r.db.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("marking notification as read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sharederrors.ErrNotFound
	}
	return nil
}

// MarkAllAsRead marks all notifications as read for a recipient.
func (r *InAppNotificationRepository) MarkAllAsRead(ctx context.Context, customerID, adminID string) error {
	if customerID != "" {
		_, err := r.db.Exec(ctx, `UPDATE notifications SET read = TRUE WHERE customer_id = $1 AND NOT read`, customerID)
		if err != nil {
			return fmt.Errorf("marking all notifications as read: %w", err)
		}
		return nil
	}
	_, err := r.db.Exec(ctx, `UPDATE notifications SET read = TRUE WHERE admin_id = $1 AND NOT read`, adminID)
	if err != nil {
		return fmt.Errorf("marking all notifications as read: %w", err)
	}
	return nil
}

// GetUnreadCount returns the number of unread notifications for a recipient.
func (r *InAppNotificationRepository) GetUnreadCount(ctx context.Context, customerID, adminID string) (int, error) {
	if customerID != "" {
		return CountRows(ctx, r.db, `SELECT COUNT(*) FROM notifications WHERE customer_id = $1 AND NOT read`, customerID)
	}
	return CountRows(ctx, r.db, `SELECT COUNT(*) FROM notifications WHERE admin_id = $1 AND NOT read`, adminID)
}

// DeleteOld removes read notifications older than the given threshold.
func (r *InAppNotificationRepository) DeleteOld(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	tag, err := r.db.Exec(ctx, `DELETE FROM notifications WHERE read = TRUE AND created_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("deleting old notifications: %w", err)
	}
	return tag.RowsAffected(), nil
}
