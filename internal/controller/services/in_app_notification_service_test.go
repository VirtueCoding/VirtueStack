package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// notifMockDB implements repository.DB for in-app notification service tests.
type notifMockDB struct {
	unreadCount int
}

func (m *notifMockDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "INSERT INTO notifications"):
		now := time.Now()
		custID := "cust-1"
		return &fakeRow{values: []any{
			"new-id", &custID, (*string)(nil),
			"vm.status_change", "Test", "test msg",
			[]byte("{}"), false, now,
		}}
	case strings.Contains(sql, "SELECT COUNT(*)"):
		return &fakeRow{values: []any{m.unreadCount}}
	default:
		return &fakeRow{scanErr: pgx.ErrNoRows}
	}
}

func (m *notifMockDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return &fakeRows{}, nil
}

func (m *notifMockDB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (m *notifMockDB) Begin(_ context.Context) (pgx.Tx, error) {
	return nil, fmt.Errorf("not implemented")
}

func buildTestInAppService(hub *SSEHub, unreadCount int) *InAppNotificationService {
	db := &notifMockDB{unreadCount: unreadCount}
	repo := repository.NewInAppNotificationRepository(db)
	return NewInAppNotificationService(InAppNotificationServiceConfig{
		Repo:   repo,
		Hub:    hub,
		Logger: slog.Default(),
	})
}

func TestInAppNotificationService_Notify_ValidatesRecipient(t *testing.T) {
	tests := []struct {
		name       string
		customerID *string
		adminID    *string
		wantErr    bool
	}{
		{name: "customer only", customerID: strPtr("cust-1"), wantErr: false},
		{name: "admin only", adminID: strPtr("admin-1"), wantErr: false},
		{name: "both set", customerID: strPtr("cust-1"), adminID: strPtr("admin-1"), wantErr: true},
		{name: "neither set", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := NewSSEHub(slog.Default())
			svc := buildTestInAppService(hub, 0)
			req := &models.CreateInAppNotificationRequest{
				CustomerID: tt.customerID,
				AdminID:    tt.adminID,
				Type:       models.NotifTypeVMStatusChange,
				Title:      "Test",
				Message:    "test msg",
			}
			err := svc.Notify(context.Background(), req)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestInAppNotificationService_Notify_BroadcastsSSE(t *testing.T) {
	hub := NewSSEHub(slog.Default())
	ch := make(chan SSEEvent, 1)
	hub.Register("cust-1", ch)
	defer hub.Unregister("cust-1", ch)

	svc := buildTestInAppService(hub, 0)
	custID := "cust-1"
	req := &models.CreateInAppNotificationRequest{
		CustomerID: &custID,
		Type:       models.NotifTypeBackupCompleted,
		Title:      "Backup Done",
		Message:    "Your backup completed",
		Data:       json.RawMessage(`{"backup_id":"b1"}`),
	}
	err := svc.Notify(context.Background(), req)
	require.NoError(t, err)

	select {
	case event := <-ch:
		assert.Equal(t, "notification", event.Type)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for SSE event")
	}
}

func TestInAppNotificationService_List(t *testing.T) {
	hub := NewSSEHub(slog.Default())
	svc := buildTestInAppService(hub, 0)

	results, hasMore, err := svc.List(context.Background(), "cust-1", "", false, "", 20)
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.False(t, hasMore)
}

func TestInAppNotificationService_MarkAsRead(t *testing.T) {
	hub := NewSSEHub(slog.Default())
	ch := make(chan SSEEvent, 2)
	hub.Register("cust-1", ch)
	defer hub.Unregister("cust-1", ch)

	svc := buildTestInAppService(hub, 3)
	err := svc.MarkAsRead(context.Background(), "notif-1", "cust-1", "")
	require.NoError(t, err)

	select {
	case event := <-ch:
		assert.Equal(t, "unread_count_changed", event.Type)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for unread count SSE")
	}
}

func TestInAppNotificationService_MarkAllAsRead(t *testing.T) {
	hub := NewSSEHub(slog.Default())
	ch := make(chan SSEEvent, 1)
	hub.Register("cust-1", ch)
	defer hub.Unregister("cust-1", ch)

	svc := buildTestInAppService(hub, 0)
	err := svc.MarkAllAsRead(context.Background(), "cust-1", "")
	require.NoError(t, err)

	select {
	case event := <-ch:
		assert.Equal(t, "unread_count_changed", event.Type)
		var countResp models.UnreadCountResponse
		require.NoError(t, json.Unmarshal(event.Data, &countResp))
		assert.Equal(t, 0, countResp.Count)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestInAppNotificationService_GetUnreadCount(t *testing.T) {
	hub := NewSSEHub(slog.Default())
	svc := buildTestInAppService(hub, 7)

	count, err := svc.GetUnreadCount(context.Background(), "cust-1", "")
	require.NoError(t, err)
	assert.Equal(t, 7, count)
}
