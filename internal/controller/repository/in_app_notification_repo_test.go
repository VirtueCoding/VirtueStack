package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockNotifDB struct {
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFunc    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (m *mockNotifDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockNotifDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return mockNotifRow{err: pgx.ErrNoRows}
}

func (m *mockNotifDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, sql, args...)
	}
	return nil, nil
}

func (m *mockNotifDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return nil, nil
}

type mockNotifRow struct {
	values []any
	err    error
}

func (m mockNotifRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	for i, d := range dest {
		if i < len(m.values) {
			switch ptr := d.(type) {
			case *string:
				*ptr = m.values[i].(string)
			case **string:
				v := m.values[i].(string)
				*ptr = &v
			case *bool:
				*ptr = m.values[i].(bool)
			case *[]byte:
				*ptr = m.values[i].([]byte)
			case *time.Time:
				*ptr = m.values[i].(time.Time)
			case *int:
				*ptr = m.values[i].(int)
			}
		}
	}
	return nil
}

func TestInAppNotificationRepository_MarkAsRead(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		customerID   string
		adminID      string
		rowsAffected int64
		execErr      error
		wantErr      bool
		isNotFound   bool
	}{
		{
			name:         "customer notification found",
			id:           "notif-1",
			customerID:   "cust-1",
			rowsAffected: 1,
		},
		{
			name:         "admin notification found",
			id:           "notif-2",
			adminID:      "admin-1",
			rowsAffected: 1,
		},
		{
			name:         "notification not found",
			id:           "notif-3",
			customerID:   "cust-1",
			rowsAffected: 0,
			wantErr:      true,
			isNotFound:   true,
		},
		{
			name:       "database error",
			id:         "notif-4",
			customerID: "cust-1",
			execErr:    errors.New("connection refused"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockNotifDB{
				execFunc: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
					if tt.execErr != nil {
						return pgconn.CommandTag{}, tt.execErr
					}
					if tt.customerID != "" {
						assert.Contains(t, sql, "customer_id")
					} else {
						assert.Contains(t, sql, "admin_id")
					}
					return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tt.rowsAffected)), nil
				},
			}
			repo := NewInAppNotificationRepository(db)
			err := repo.MarkAsRead(context.Background(), tt.id, tt.customerID, tt.adminID)
			if tt.wantErr {
				require.Error(t, err)
				if tt.isNotFound {
					assert.True(t, errors.Is(err, sharederrors.ErrNotFound))
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestInAppNotificationRepository_MarkAllAsRead(t *testing.T) {
	tests := []struct {
		name       string
		customerID string
		adminID    string
		execErr    error
		wantErr    bool
	}{
		{name: "customer bulk read", customerID: "cust-1"},
		{name: "admin bulk read", adminID: "admin-1"},
		{name: "db error", customerID: "cust-1", execErr: errors.New("fail"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockNotifDB{
				execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
					if tt.execErr != nil {
						return pgconn.CommandTag{}, tt.execErr
					}
					assert.Contains(t, sql, "UPDATE notifications SET read = TRUE")
					return pgconn.NewCommandTag("UPDATE 5"), nil
				},
			}
			repo := NewInAppNotificationRepository(db)
			err := repo.MarkAllAsRead(context.Background(), tt.customerID, tt.adminID)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestInAppNotificationRepository_GetUnreadCount(t *testing.T) {
	tests := []struct {
		name       string
		customerID string
		adminID    string
		count      int
		queryErr   error
		wantErr    bool
	}{
		{name: "customer count", customerID: "cust-1", count: 5},
		{name: "admin count", adminID: "admin-1", count: 3},
		{name: "db error", customerID: "cust-1", queryErr: errors.New("fail"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockNotifDB{
				queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
					if tt.queryErr != nil {
						return mockNotifRow{err: tt.queryErr}
					}
					if tt.customerID != "" {
						assert.Contains(t, sql, "customer_id")
					} else {
						assert.Contains(t, sql, "admin_id")
					}
					return mockNotifRow{values: []any{tt.count}}
				},
			}
			repo := NewInAppNotificationRepository(db)
			count, err := repo.GetUnreadCount(context.Background(), tt.customerID, tt.adminID)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.count, count)
		})
	}
}

func TestInAppNotificationRepository_DeleteOld(t *testing.T) {
	tests := []struct {
		name     string
		affected int64
		execErr  error
		wantErr  bool
	}{
		{name: "deleted some", affected: 10},
		{name: "none to delete", affected: 0},
		{name: "db error", execErr: errors.New("fail"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockNotifDB{
				execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
					if tt.execErr != nil {
						return pgconn.CommandTag{}, tt.execErr
					}
					assert.Contains(t, sql, "DELETE FROM notifications")
					return pgconn.NewCommandTag(fmt.Sprintf("DELETE %d", tt.affected)), nil
				},
			}
			repo := NewInAppNotificationRepository(db)
			count, err := repo.DeleteOld(context.Background(), 90*24*time.Hour)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.affected, count)
		})
	}
}

func TestInAppNotificationRepository_Create(t *testing.T) {
	custID := "cust-1"
	now := time.Now()

	db := &mockNotifDB{
		queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
			assert.Contains(t, sql, "INSERT INTO notifications")
			return mockNotifRow{
				values: []any{
					"new-id",
					custID,
					"",
					string(models.NotifTypeVMStatusChange),
					"VM Status",
					"Your VM changed status",
					[]byte("{}"),
					false,
					now,
				},
			}
		},
	}
	repo := NewInAppNotificationRepository(db)
	n := &models.InAppNotification{
		CustomerID: &custID,
		Type:       models.NotifTypeVMStatusChange,
		Title:      "VM Status",
		Message:    "Your VM changed status",
		Data:       json.RawMessage("{}"),
	}
	err := repo.Create(context.Background(), n)
	require.NoError(t, err)
	assert.Equal(t, "new-id", n.ID)
}

func TestInAppNotificationRepository_ListByCustomer_CursorPagination(t *testing.T) {
	tests := []struct {
		name     string
		cursor   string
		perPage  int
		queryErr error
		wantErr  bool
	}{
		{name: "first page no cursor", cursor: "", perPage: 20},
		{name: "with cursor", cursor: "2024-01-01T00:00:00Z", perPage: 10},
		{name: "db error", cursor: "", perPage: 20, queryErr: errors.New("fail"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockNotifDB{
				queryFunc: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
					if tt.queryErr != nil {
						return nil, tt.queryErr
					}
					assert.Contains(t, sql, "customer_id")
					if tt.cursor != "" {
						assert.True(t, strings.Contains(sql, "created_at < $"))
						require.Len(t, args, 3)
						assert.Equal(t, "2024-01-01T00:00:00Z", args[1])
					}
					return &emptyRows{}, nil
				},
			}
			repo := NewInAppNotificationRepository(db)
			results, hasMore, err := repo.ListByCustomer(context.Background(), "cust-1", false, tt.cursor, tt.perPage)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.False(t, hasMore)
			assert.Empty(t, results)
		})
	}
}

func TestInAppNotificationRepository_ListByCustomer_DecodesOpaqueCursor(t *testing.T) {
	cursor := models.EncodeCursor("2024-01-01T00:00:00Z", "next")
	db := &mockNotifDB{
		queryFunc: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
			assert.Contains(t, sql, "created_at < $")
			require.Len(t, args, 3)
			assert.Equal(t, "cust-1", args[0])
			assert.Equal(t, "2024-01-01T00:00:00Z", args[1])
			assert.Equal(t, 11, args[2])
			return &emptyRows{}, nil
		},
	}

	repo := NewInAppNotificationRepository(db)
	results, hasMore, err := repo.ListByCustomer(context.Background(), "cust-1", false, cursor, 10)
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.False(t, hasMore)
}

// emptyRows implements pgx.Rows for zero-result queries.
type emptyRows struct{}

func (e *emptyRows) Close()                                       {}
func (e *emptyRows) Err() error                                   { return nil }
func (e *emptyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (e *emptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (e *emptyRows) Next() bool                                   { return false }
func (e *emptyRows) Scan(_ ...any) error                          { return nil }
func (e *emptyRows) Values() ([]any, error)                       { return nil, nil }
func (e *emptyRows) RawValues() [][]byte                          { return nil }
func (e *emptyRows) Conn() *pgx.Conn                              { return nil }
