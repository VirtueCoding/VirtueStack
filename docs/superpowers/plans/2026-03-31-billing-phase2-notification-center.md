# Billing Phase 2: In-App Notification Center — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an in-app notification center with SSE real-time delivery, supporting both admin and customer portals, to replace reliance on email-only notifications for billing events.

**Architecture:** New `notifications` table with RLS, repository/service layer, SSE hub for real-time push, REST endpoints for CRUD, and React components with EventSource hooks in both portals.

**Tech Stack:** Go 1.26, PostgreSQL 18, Gin SSE streaming, React 19, TanStack Query, EventSource API

**Depends on:** Phase 0 (for billing context), Phase 1 (for config flags)
**Depended on by:** Phase 3 (billing uses notifications), Phase 4 (payment notifications)

---

## Task 1: Database Migration — `notifications` Table

- [ ] Create migration `000073_notifications.up.sql` and `000073_notifications.down.sql`

**Files:**
- `migrations/000073_notifications.up.sql`
- `migrations/000073_notifications.down.sql`

### 1a. Up migration

Create `migrations/000073_notifications.up.sql`:

```sql
SET lock_timeout = '5s';

-- In-app notifications for both customers and admins.
-- Each row belongs to exactly one recipient (customer XOR admin).
CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID REFERENCES customers(id) ON DELETE CASCADE,
    admin_id UUID REFERENCES admins(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    data JSONB NOT NULL DEFAULT '{}',
    read BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notifications_one_recipient_ck
        CHECK (
            (customer_id IS NOT NULL AND admin_id IS NULL) OR
            (customer_id IS NULL AND admin_id IS NOT NULL)
        )
);

-- Partial indexes for fast "unread" queries (the most common access pattern).
CREATE INDEX idx_notifications_customer_unread
    ON notifications(customer_id, created_at DESC) WHERE NOT read;
CREATE INDEX idx_notifications_admin_unread
    ON notifications(admin_id, created_at DESC) WHERE NOT read;

-- Full listing index for paginated queries.
CREATE INDEX idx_notifications_customer_created
    ON notifications(customer_id, created_at DESC);
CREATE INDEX idx_notifications_admin_created
    ON notifications(admin_id, created_at DESC);

-- Row-Level Security: customers can only see their own notifications.
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;

CREATE POLICY notifications_customer_policy ON notifications
    FOR ALL TO PUBLIC
    USING (customer_id = current_setting('app.current_customer_id', true)::UUID);

-- Grant access to the application roles used by pgx.
GRANT SELECT, INSERT, UPDATE, DELETE ON notifications TO app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON notifications TO app_customer;
```

### 1b. Down migration

Create `migrations/000073_notifications.down.sql`:

```sql
SET lock_timeout = '5s';

DROP POLICY IF EXISTS notifications_customer_policy ON notifications;
DROP TABLE IF EXISTS notifications;
```

### Verification

```bash
make migrate-up   # Apply migration (requires running PostgreSQL)
make migrate-down  # Verify rollback works
make migrate-up   # Re-apply
```

---

## Task 2: Notification Model

- [ ] Create `InAppNotification` model struct and notification type constants

**File:** `internal/controller/models/in_app_notification.go`

### 2a. Create the model file

```go
package models

import (
	"encoding/json"
	"time"
)

// In-app notification type constants.
const (
	NotifTypeBillingLowBalance     = "billing.low_balance"
	NotifTypeBillingPaymentReceived = "billing.payment_received"
	NotifTypeBillingVMSuspended    = "billing.vm_suspended"
	NotifTypeBillingInvoiceGenerated = "billing.invoice_generated"
	NotifTypeSystemMaintenance     = "system.maintenance"
	NotifTypeVMStatusChange        = "vm.status_change"
	NotifTypeBackupCompleted       = "backup.completed"
	NotifTypeBackupFailed          = "backup.failed"
)

// ValidNotificationTypes lists all recognized in-app notification types.
var ValidNotificationTypes = []string{
	NotifTypeBillingLowBalance,
	NotifTypeBillingPaymentReceived,
	NotifTypeBillingVMSuspended,
	NotifTypeBillingInvoiceGenerated,
	NotifTypeSystemMaintenance,
	NotifTypeVMStatusChange,
	NotifTypeBackupCompleted,
	NotifTypeBackupFailed,
}

// InAppNotification represents a single in-app notification stored in the
// notifications table. Exactly one of CustomerID or AdminID is non-nil.
type InAppNotification struct {
	ID         string          `json:"id" db:"id"`
	CustomerID *string         `json:"customer_id,omitempty" db:"customer_id"`
	AdminID    *string         `json:"admin_id,omitempty" db:"admin_id"`
	Type       string          `json:"type" db:"type"`
	Title      string          `json:"title" db:"title"`
	Message    string          `json:"message" db:"message"`
	Data       json.RawMessage `json:"data" db:"data"`
	Read       bool            `json:"read" db:"read"`
	CreatedAt  time.Time       `json:"created_at" db:"created_at"`
}

// InAppNotificationResponse is the JSON representation returned by the API.
type InAppNotificationResponse struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Title     string          `json:"title"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data,omitempty"`
	Read      bool            `json:"read"`
	CreatedAt string          `json:"created_at"`
}

// ToResponse converts an InAppNotification to its API response form.
func (n *InAppNotification) ToResponse() *InAppNotificationResponse {
	return &InAppNotificationResponse{
		ID:        n.ID,
		Type:      n.Type,
		Title:     n.Title,
		Message:   n.Message,
		Data:      n.Data,
		Read:      n.Read,
		CreatedAt: n.CreatedAt.Format(time.RFC3339),
	}
}

// CreateInAppNotificationRequest is used internally to create a notification.
type CreateInAppNotificationRequest struct {
	CustomerID *string         `json:"customer_id,omitempty" validate:"omitempty,uuid"`
	AdminID    *string         `json:"admin_id,omitempty" validate:"omitempty,uuid"`
	Type       string          `json:"type" validate:"required,max=50"`
	Title      string          `json:"title" validate:"required,max=255"`
	Message    string          `json:"message" validate:"required,max=2000"`
	Data       json.RawMessage `json:"data,omitempty"`
}

// UnreadCountResponse wraps the unread count for the API.
type UnreadCountResponse struct {
	Count int `json:"count"`
}
```

### Naming rationale

The struct is called `InAppNotification` (not `Notification`) to avoid conflicts with the existing `NotificationEvent` and `NotificationPreferences` models that power the email/Telegram notification system.

---

## Task 3: Notification Repository

- [ ] Create `InAppNotificationRepository` with Create, List, MarkAsRead, MarkAllAsRead, GetUnreadCount, and DeleteOld methods

**File:** `internal/controller/repository/in_app_notification_repo.go`

### 3a. Create the repository

Follow the exact patterns from `vm_repo.go`: const column list, `scanInAppNotification` helper, parameterized queries, `fmt.Errorf` wrapping, `sharederrors.ErrNotFound`.

```go
package repository

// Column list constant
const inAppNotificationSelectCols = `
	id, customer_id, admin_id, type, title, message, data, read, created_at`

// scanInAppNotification scans a pgx.Row into an InAppNotification.
func scanInAppNotification(row pgx.Row) (models.InAppNotification, error) { ... }
```

**Methods to implement** (all take `ctx context.Context` as first param):

| Method | SQL Pattern | Notes |
|--------|-------------|-------|
| `Create(ctx, notif *InAppNotification) error` | `INSERT ... RETURNING` | Populates ID and CreatedAt via RETURNING clause |
| `ListByCustomer(ctx, customerID string, unreadOnly bool, cursor string, perPage int) ([]InAppNotification, string, error)` | `SELECT ... WHERE customer_id = $1 AND (NOT read OR NOT $2) AND created_at < $3 ORDER BY created_at DESC LIMIT $4` | Cursor-based pagination using `created_at` timestamp. Return next cursor as the last row's `created_at` in RFC3339. If `cursor` is empty, omit the `created_at < $3` clause. |
| `ListByAdmin(ctx, adminID string, unreadOnly bool, cursor string, perPage int) ([]InAppNotification, string, error)` | Same as above, using `admin_id = $1` | Identical logic, different WHERE column |
| `MarkAsRead(ctx, id string, customerID *string, adminID *string) error` | `UPDATE notifications SET read = TRUE WHERE id = $1 AND (customer_id = $2 OR admin_id = $3)` | Returns `ErrNotFound` if no row updated. Both customerID and adminID passed; exactly one is non-nil for ownership check. |
| `MarkAllAsRead(ctx, customerID *string, adminID *string) error` | `UPDATE notifications SET read = TRUE WHERE (customer_id = $1 OR admin_id = $2) AND NOT read` | Bulk update. Exactly one of customerID/adminID is non-nil. |
| `GetUnreadCount(ctx, customerID *string, adminID *string) (int, error)` | `SELECT COUNT(*) FROM notifications WHERE (customer_id = $1 OR admin_id = $2) AND NOT read` | Uses the partial index for fast unread counts |
| `DeleteOld(ctx, olderThan time.Duration) (int64, error)` | `DELETE FROM notifications WHERE created_at < $1 AND read = TRUE` | Cleanup scheduler use. Returns number of rows deleted. Only deletes read notifications. |

### Cursor pagination design

Use `created_at` as cursor (ISO 8601 string). The client sends the `created_at` value of the last item as the `cursor` query parameter. The repo parses it with `time.Parse(time.RFC3339Nano, cursor)` and adds `AND created_at < $cursor` to the WHERE clause. Always order by `created_at DESC` and `LIMIT perPage + 1` to detect whether there's a next page (if len(results) > perPage, return results[:perPage] and the next cursor).

### Verification

```bash
go build ./internal/controller/repository/...
```

---

## Task 4: Notification Repository Tests

- [ ] Write table-driven tests for all `InAppNotificationRepository` methods

**File:** `internal/controller/repository/in_app_notification_repo_test.go`

### Test approach

Use the mock `DB` interface already established in the repository package (see `vm_repo.go` pattern with `mockDB`). Each method gets a table-driven test:

| Test function | Cases |
|---------------|-------|
| `TestInAppNotificationRepo_Create` | happy path, nil Data defaults to `{}`, missing customer_id AND admin_id (both nil → DB constraint error) |
| `TestInAppNotificationRepo_ListByCustomer` | no notifications (empty result), multiple notifications returned in order, unreadOnly filter, cursor pagination, per_page limit |
| `TestInAppNotificationRepo_ListByAdmin` | mirrors customer tests with admin_id |
| `TestInAppNotificationRepo_MarkAsRead` | happy path, notification not found → ErrNotFound, wrong owner → ErrNotFound |
| `TestInAppNotificationRepo_MarkAllAsRead` | marks multiple, no unread → no error |
| `TestInAppNotificationRepo_GetUnreadCount` | zero count, positive count |
| `TestInAppNotificationRepo_DeleteOld` | deletes old read notifications, preserves unread, preserves recent |

Use `require.NoError`, `require.Error`, `assert.Equal` from testify.

### Verification

```bash
go test -race -run TestInAppNotificationRepo ./internal/controller/repository/...
```

---

## Task 5: SSE Hub Implementation

- [ ] Create a thread-safe SSE hub that manages per-user client connections and broadcasts events

**File:** `internal/controller/services/sse_hub.go`

### 5a. Design

The SSE hub manages a map of user connections. Each user (identified by `userID` string) can have multiple concurrent SSE connections (e.g., multiple browser tabs). The hub supports:

- **Register(userID, channel)** — add a client channel for a user
- **Unregister(userID, channel)** — remove a specific client channel
- **Broadcast(userID, event)** — send an event to all connections for a user

```go
package services

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// SSEEvent represents a Server-Sent Event payload.
type SSEEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// SSEHub manages SSE client connections per user.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[string]map[chan SSEEvent]struct{} // userID → set of channels
	logger  *slog.Logger
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub(logger *slog.Logger) *SSEHub {
	return &SSEHub{
		clients: make(map[string]map[chan SSEEvent]struct{}),
		logger:  logger.With("component", "sse-hub"),
	}
}
```

### 5b. Methods

| Method | Description |
|--------|-------------|
| `Register(userID string, ch chan SSEEvent)` | Acquires write lock, adds channel to `clients[userID]` map. Creates the inner map if needed. Logs connection count. |
| `Unregister(userID string, ch chan SSEEvent)` | Acquires write lock, removes channel from map. Deletes the user entry if no channels remain. |
| `Broadcast(userID string, event SSEEvent)` | Acquires read lock, iterates over `clients[userID]` channels, sends event via non-blocking select (drop if channel full). Log a warning if a send is dropped. |
| `ConnectionCount(userID string) int` | Acquires read lock, returns len of `clients[userID]`. Used for enforcing max 2 SSE connections per user. |

### 5c. Channel buffer size

Use a buffered channel of size 16 for each client: `make(chan SSEEvent, 16)`. This prevents slow clients from blocking broadcasts while providing backpressure.

### 5d. Non-blocking send pattern

```go
func (h *SSEHub) Broadcast(userID string, event SSEEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	channels, ok := h.clients[userID]
	if !ok {
		return
	}
	for ch := range channels {
		select {
		case ch <- event:
		default:
			h.logger.Warn("dropped SSE event for slow client",
				"user_id", userID, "event_type", event.Type)
		}
	}
}
```

### Verification

```bash
go build ./internal/controller/services/...
```

---

## Task 6: SSE Hub Tests

- [ ] Write concurrency-safe tests for SSEHub

**File:** `internal/controller/services/sse_hub_test.go`

### Test cases

| Test function | Description |
|---------------|-------------|
| `TestSSEHub_RegisterUnregister` | Register a channel, verify ConnectionCount is 1, unregister, verify 0 |
| `TestSSEHub_BroadcastSingleClient` | Register one channel, broadcast event, read from channel, verify event data |
| `TestSSEHub_BroadcastMultipleClients` | Register 3 channels for same user, broadcast, verify all 3 receive the event |
| `TestSSEHub_BroadcastNoClients` | Broadcast to user with no clients — no panic, no error |
| `TestSSEHub_BroadcastDropsSlowClient` | Create unbuffered channel, register it, broadcast 20 events, verify no deadlock (non-blocking send) |
| `TestSSEHub_MultipleUsers` | Register channels for 2 users, broadcast to user A, verify user B does not receive |
| `TestSSEHub_ConcurrentAccess` | Spawn 10 goroutines that register/unregister/broadcast concurrently with `t.Parallel()`, verify no race (run with `-race`) |

### Verification

```bash
go test -race -run TestSSEHub ./internal/controller/services/...
```

---

## Task 7: In-App Notification Service

- [ ] Create `InAppNotificationService` that saves notifications to DB and pushes to SSE hub

**File:** `internal/controller/services/in_app_notification_service.go`

### 7a. Service struct and constructor

Follow the config-struct DI pattern from `VMServiceConfig`:

```go
package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
)

// InAppNotificationServiceConfig holds dependencies for InAppNotificationService.
type InAppNotificationServiceConfig struct {
	Repo   *repository.InAppNotificationRepository
	Hub    *SSEHub
	Logger *slog.Logger
}

// InAppNotificationService manages in-app notification lifecycle.
type InAppNotificationService struct {
	repo   *repository.InAppNotificationRepository
	hub    *SSEHub
	logger *slog.Logger
}

// NewInAppNotificationService creates a new InAppNotificationService.
func NewInAppNotificationService(cfg InAppNotificationServiceConfig) *InAppNotificationService {
	return &InAppNotificationService{
		repo:   cfg.Repo,
		hub:    cfg.Hub,
		logger: cfg.Logger.With("component", "in-app-notification-service"),
	}
}
```

### 7b. Methods

| Method | Description |
|--------|-------------|
| `Notify(ctx, req *CreateInAppNotificationRequest) (*InAppNotification, error)` | 1. Validate that exactly one of CustomerID/AdminID is set. 2. Create `InAppNotification` from request, setting `Data` to `{}` if nil. 3. Call `repo.Create(ctx, &notif)`. 4. Marshal the notification response to JSON and call `hub.Broadcast(userID, SSEEvent{Type: "notification", Data: jsonBytes})`. 5. Return the notification. The SSE broadcast is best-effort — log errors but do not fail the operation. |
| `List(ctx, customerID *string, adminID *string, unreadOnly bool, cursor string, perPage int) ([]InAppNotification, string, error)` | Delegates to `repo.ListByCustomer` or `repo.ListByAdmin` based on which ID is non-nil. Default `perPage` to 20, max 100. |
| `MarkAsRead(ctx, id string, customerID *string, adminID *string) error` | Delegates to `repo.MarkAsRead`. After success, broadcast an SSE event of type `"unread_count_changed"` with the new unread count (call `GetUnreadCount` and broadcast). |
| `MarkAllAsRead(ctx, customerID *string, adminID *string) error` | Delegates to `repo.MarkAllAsRead`. Broadcast `"unread_count_changed"` with count 0. |
| `GetUnreadCount(ctx, customerID *string, adminID *string) (int, error)` | Delegates to `repo.GetUnreadCount`. |
| `StartCleanupScheduler(ctx context.Context, interval time.Duration, maxAge time.Duration)` | Runs in a goroutine. Ticks every `interval` (default 6 hours). Calls `repo.DeleteOld(ctx, maxAge)` (default 90 days). Logs the number of deleted notifications. Stops when `ctx` is cancelled. |

### 7c. SSE event types sent

| SSE `type` field | `data` payload | When sent |
|------------------|----------------|-----------|
| `"notification"` | Full `InAppNotificationResponse` JSON | New notification created via `Notify()` |
| `"unread_count_changed"` | `{"count": N}` | After `MarkAsRead` or `MarkAllAsRead` |
| `"heartbeat"` | `{}` | Every 30 seconds (handled by SSE endpoint, not service) |

### Verification

```bash
go build ./internal/controller/services/...
```

---

## Task 8: In-App Notification Service Tests

- [ ] Write tests for `InAppNotificationService`

**File:** `internal/controller/services/in_app_notification_service_test.go`

### Test approach

Mock the repository with a struct that has function fields (same pattern as existing service tests). The SSE hub can be a real `NewSSEHub(slog.Default())` since it's lightweight and in-memory.

| Test function | Cases |
|---------------|-------|
| `TestInAppNotificationService_Notify` | happy path (customer), happy path (admin), both IDs nil → validation error, both IDs set → validation error, repo error → propagated, SSE broadcast fires (register a channel and verify event received) |
| `TestInAppNotificationService_List` | delegates to customer repo, delegates to admin repo, respects perPage max/default |
| `TestInAppNotificationService_MarkAsRead` | happy path, not found → error, broadcasts unread_count_changed |
| `TestInAppNotificationService_MarkAllAsRead` | marks all, broadcasts unread_count_changed with count 0 |
| `TestInAppNotificationService_GetUnreadCount` | returns correct count |
| `TestInAppNotificationService_CleanupScheduler` | starts, deletes old notifications on tick, stops on context cancel |

### Verification

```bash
go test -race -run TestInAppNotificationService ./internal/controller/services/...
```

---

## Task 9: Customer In-App Notification REST Handlers

- [ ] Create REST handlers for customer notification endpoints

**File:** `internal/controller/api/customer/in_app_notifications.go`

### 9a. Handler struct

```go
package customer

// InAppNotificationsHandler handles in-app notification REST endpoints.
type InAppNotificationsHandler struct {
	service *services.InAppNotificationService
	hub     *services.SSEHub
	authCfg middleware.AuthConfig
	logger  *slog.Logger
}

// NewInAppNotificationsHandler creates a new InAppNotificationsHandler.
func NewInAppNotificationsHandler(
	service *services.InAppNotificationService,
	hub *services.SSEHub,
	authCfg middleware.AuthConfig,
	logger *slog.Logger,
) *InAppNotificationsHandler {
	return &InAppNotificationsHandler{
		service: service,
		hub:     hub,
		authCfg: authCfg,
		logger:  logger.With("component", "in-app-notifications-handler"),
	}
}
```

### 9b. REST endpoints

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| `GET` | `/notifications` | `ListNotifications` | List customer notifications. Query params: `unread` (bool), `cursor` (string), `per_page` (int, default 20, max 100). Uses `middleware.GetUserID(c)`. Returns `models.Response{Data: responses, Meta: meta}` where meta includes `next_cursor`. |
| `POST` | `/notifications/:id/read` | `MarkAsRead` | Mark a single notification as read. Extracts `:id` param and `customerID` from JWT. Returns 204 No Content on success. |
| `POST` | `/notifications/read-all` | `MarkAllAsRead` | Mark all customer notifications as read. Returns 204 No Content. |
| `GET` | `/notifications/unread-count` | `GetUnreadCount` | Returns `{"data": {"count": N}}`. |

### 9c. Handler implementation pattern

Follow the existing handler pattern in `notifications.go`:

```go
func (h *InAppNotificationsHandler) ListNotifications(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	unreadOnly := c.Query("unread") == "true"
	cursor := c.Query("cursor")
	perPage := common.ParsePerPage(c, 20, 100)

	notifications, nextCursor, err := h.service.List(
		c.Request.Context(), &customerID, nil, unreadOnly, cursor, perPage,
	)
	if err != nil {
		h.logger.Error("failed to list notifications",
			"error", err, "customer_id", customerID)
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"INTERNAL_ERROR", "Failed to list notifications")
		return
	}

	responses := make([]*models.InAppNotificationResponse, len(notifications))
	for i := range notifications {
		responses[i] = notifications[i].ToResponse()
	}

	meta := map[string]any{"next_cursor": nextCursor}
	c.JSON(http.StatusOK, models.Response{Data: responses, Meta: meta})
}
```

Error responses always use `middleware.RespondWithError`. Never use `c.JSON` for errors.

### Verification

```bash
go build ./internal/controller/api/customer/...
```

---

## Task 10: Customer SSE Stream Handler

- [ ] Create the SSE streaming endpoint for customer real-time notifications

**File:** `internal/controller/api/customer/in_app_notifications.go` (append to same file from Task 9)

### 10a. SSE endpoint handler

Add `StreamNotifications` method to `InAppNotificationsHandler`:

```go
// StreamNotifications handles GET /notifications/stream.
// This is a Server-Sent Events endpoint for real-time notification delivery.
// Requires JWT authentication only (no API key support for streaming).
func (h *InAppNotificationsHandler) StreamNotifications(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	// Enforce max 2 SSE connections per user
	if h.hub.ConnectionCount(customerID) >= 2 {
		middleware.RespondWithError(c, http.StatusTooManyRequests,
			"SSE_LIMIT_EXCEEDED", "Maximum 2 SSE connections per user")
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering

	ch := make(chan services.SSEEvent, 16)
	h.hub.Register(customerID, ch)
	defer h.hub.Unregister(customerID, ch)

	// Send initial unread count
	count, _ := h.service.GetUnreadCount(
		c.Request.Context(), &customerID, nil,
	)
	initialData, _ := json.Marshal(map[string]int{"count": count})
	c.SSEvent("unread_count", string(initialData))
	c.Writer.Flush()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	clientGone := c.Request.Context().Done()
	for {
		select {
		case <-clientGone:
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			c.SSEvent(event.Type, string(event.Data))
			c.Writer.Flush()
		case <-ticker.C:
			c.SSEvent("heartbeat", "{}")
			c.Writer.Flush()
		}
	}
}
```

### 10b. Key design decisions

- **JWT only:** SSE connections require JWT authentication, not API keys. API keys do not support long-lived streaming connections.
- **Connection limit:** Max 2 concurrent SSE connections per user (multiple browser tabs). Checked via `hub.ConnectionCount()`.
- **Heartbeat:** Every 30 seconds to keep the connection alive through proxies/load balancers.
- **Nginx buffering:** `X-Accel-Buffering: no` header disables nginx response buffering for SSE.
- **Gin SSEvent:** Uses `c.SSEvent(event, data)` which formats as `event: {event}\ndata: {data}\n\n`.
- **Initial unread count:** Sent immediately on connection so the UI can show the badge without a separate REST call.
- **Graceful disconnect:** Deregisters from hub on `c.Request.Context().Done()` (client disconnect) or channel close.

### 10c. Write timeout consideration

The HTTP server has a 30-second write timeout (`WriteTimeout = 30 * time.Second` in `server.go`). For SSE to work, the SSE handler must use `c.Writer` which implements `http.Flusher`. The heartbeat at 30-second intervals keeps the connection alive. If the write timeout causes issues, the SSE route may need a per-handler timeout override using a custom `http.Server` or Gin middleware that extends the deadline on each write. Document this as a known consideration — if SSE connections drop after 30 seconds in testing, add a middleware that calls `http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(60 * time.Second))` before each flush.

### Verification

```bash
go build ./internal/controller/api/customer/...
```

---

## Task 11: Admin In-App Notification REST + SSE Handlers

- [ ] Create REST and SSE handlers for admin notification endpoints

**File:** `internal/controller/api/admin/in_app_notifications.go`

### 11a. Handler struct

Mirror the customer handler structure, using `adminID` instead of `customerID`:

```go
package admin

// AdminInAppNotificationsHandler handles admin in-app notification endpoints.
type AdminInAppNotificationsHandler struct {
	service *services.InAppNotificationService
	hub     *services.SSEHub
	authCfg middleware.AuthConfig
	logger  *slog.Logger
}
```

### 11b. REST endpoints

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| `GET` | `/notifications` | `ListNotifications` | List admin notifications. Same query params as customer. Uses `middleware.GetUserID(c)` to get admin ID. |
| `POST` | `/notifications/:id/read` | `MarkAsRead` | Mark single notification as read for admin. |
| `POST` | `/notifications/read-all` | `MarkAllAsRead` | Mark all admin notifications as read. |
| `GET` | `/notifications/unread-count` | `GetUnreadCount` | Get admin unread count. |
| `GET` | `/notifications/stream` | `StreamNotifications` | SSE endpoint (same pattern as customer, using admin ID). |

### 11c. Implementation

Identical logic to customer handlers, but:
- Uses `adminID` variable: `adminID := middleware.GetUserID(c)`
- Passes `nil` for customerID and `&adminID` for adminID in all service calls
- No API key support (admin always uses JWT)

### Verification

```bash
go build ./internal/controller/api/admin/...
```

---

## Task 12: Customer Route Registration

- [ ] Register customer in-app notification routes

**File:** `internal/controller/api/customer/routes.go`

### 12a. Update `RegisterCustomerRoutes` function signature

Add the `inAppNotifHandler` parameter:

```go
func RegisterCustomerRoutes(
	router *gin.RouterGroup,
	handler *CustomerHandler,
	notifyHandler *NotificationsHandler,
	inAppNotifHandler *InAppNotificationsHandler, // NEW
	apiKeyRepo *repository.CustomerAPIKeyRepository,
	allowSelfRegistration bool,
)
```

### 12b. Register the routes

Add after the existing `RegisterNotificationRoutes` call (around line 87), inside the same `if` block or with its own nil check:

```go
if inAppNotifHandler != nil {
	registerInAppNotificationRoutes(protected, inAppNotifHandler)
}
```

### 12c. Create route registration function

Add at the bottom of `routes.go`:

```go
// registerInAppNotificationRoutes registers in-app notification endpoints.
func registerInAppNotificationRoutes(
	router *gin.RouterGroup,
	handler *InAppNotificationsHandler,
) {
	notifications := router.Group("/notifications")
	notifications.GET("", handler.ListNotifications)
	notifications.POST("/:id/read", handler.MarkAsRead)
	notifications.POST("/read-all", handler.MarkAllAsRead)
	notifications.GET("/unread-count", handler.GetUnreadCount)
	notifications.GET("/stream", handler.StreamNotifications)
}
```

### 12d. Route collision check

The existing `RegisterNotificationRoutes` uses paths under `/notifications/preferences`, `/notifications/events`, and `/notifications/events/types`. The new routes use `/notifications` (list), `/notifications/:id/read`, `/notifications/read-all`, `/notifications/unread-count`, and `/notifications/stream`. There are no collisions because:
- `GET /notifications` (list) vs `GET /notifications/preferences` — different sub-paths
- `GET /notifications/stream` vs `GET /notifications/events` — different sub-paths
- `/notifications/:id/read` uses POST, events use GET

### Verification

```bash
go build ./internal/controller/api/customer/...
```

---

## Task 13: Admin Route Registration

- [ ] Register admin in-app notification routes

**File:** `internal/controller/api/admin/routes.go`

### 13a. Update `RegisterAdminRoutes` signature

Add the `inAppNotifHandler` parameter to `RegisterAdminRoutes`:

```go
func RegisterAdminRoutes(
	router *gin.RouterGroup,
	handler *AdminHandler,
	inAppNotifHandler *AdminInAppNotificationsHandler, // NEW
)
```

### 13b. Register routes in protected group

Add inside the protected group, after other route registrations:

```go
if inAppNotifHandler != nil {
	registerAdminInAppNotificationRoutes(protected, inAppNotifHandler)
}
```

### 13c. Create route registration function

```go
// registerAdminInAppNotificationRoutes registers admin in-app notification endpoints.
func registerAdminInAppNotificationRoutes(
	router *gin.RouterGroup,
	handler *AdminInAppNotificationsHandler,
) {
	notifications := router.Group("/notifications")
	notifications.GET("", handler.ListNotifications)
	notifications.POST("/:id/read", handler.MarkAsRead)
	notifications.POST("/read-all", handler.MarkAllAsRead)
	notifications.GET("/unread-count", handler.GetUnreadCount)
	notifications.GET("/stream", handler.StreamNotifications)
}
```

### 13d. Permission check

Admin notification endpoints do **not** require specific RBAC permissions — every authenticated admin can read their own notifications. The routes are placed inside the protected group which already enforces `RequireRole("admin", "super_admin")` and `AdminLoader`.

### Verification

```bash
go build ./internal/controller/api/admin/...
```

---

## Task 14: Dependencies Wiring

- [ ] Wire `InAppNotificationRepository`, `SSEHub`, `InAppNotificationService`, and handlers in `dependencies.go`

**File:** `internal/controller/dependencies.go`

### 14a. Add SSE hub to Server struct

In `server.go`, add a field to the `Server` struct:

```go
sseHub *services.SSEHub
```

### 14b. Add handler fields to Server struct

In `server.go`, add fields:

```go
customerInAppNotifHandler *customer.InAppNotificationsHandler
adminInAppNotifHandler    *admin.AdminInAppNotificationsHandler
```

### 14c. Wire in `InitializeServices()`

In `dependencies.go`, add after the existing `notifyHandler` wiring (around line 300):

```go
// In-app notification center
inAppNotifRepo := repository.NewInAppNotificationRepository(s.dbPool)

s.sseHub = services.NewSSEHub(s.logger)

inAppNotifService := services.NewInAppNotificationService(services.InAppNotificationServiceConfig{
	Repo:   inAppNotifRepo,
	Hub:    s.sseHub,
	Logger: s.logger,
})

s.customerInAppNotifHandler = customer.NewInAppNotificationsHandler(
	inAppNotifService,
	s.sseHub,
	middleware.AuthConfig{
		JWTSecret: s.config.JWTSecret.Value(),
		Issuer:    "virtuestack",
	},
	s.logger,
)

s.adminInAppNotifHandler = admin.NewAdminInAppNotificationsHandler(
	inAppNotifService,
	s.sseHub,
	middleware.AuthConfig{
		JWTSecret: s.config.JWTSecret.Value(),
		Issuer:    "virtuestack",
	},
	s.logger,
)
```

### 14d. Update `RegisterAPIRoutes` call

In `server.go`, update the calls to `RegisterCustomerRoutes` and `RegisterAdminRoutes` to pass the new handler parameters. Find where these are called and add the new arguments.

### 14e. Store inAppNotifService on Server for scheduler access

Add a `inAppNotifService` field to `Server` and assign it in `InitializeServices()` so the cleanup scheduler can be started in `StartSchedulers()`.

### Verification

```bash
go build ./internal/controller/...
```

---

## Task 15: Cleanup Scheduler Registration

- [ ] Start the notification cleanup scheduler in `StartSchedulers`

**File:** `internal/controller/schedulers.go`

### 15a. Add cleanup scheduler

Add after the `startSessionCleanup` call (around line 48):

```go
if s.inAppNotifService != nil {
	s.logger.Info("starting notification cleanup scheduler")
	go s.inAppNotifService.StartCleanupScheduler(ctx, 6*time.Hour, 90*24*time.Hour)
}
```

This runs every 6 hours and deletes read notifications older than 90 days.

### Verification

```bash
go build ./internal/controller/...
```

---

## Task 16: Customer API Client Updates

- [ ] Add in-app notification API methods to the customer frontend API client

**File:** `webui/customer/lib/api-client.ts`

### 16a. Add TypeScript types

Add near the existing `NotificationEvent` interface:

```typescript
export interface InAppNotification {
  id: string;
  type: string;
  title: string;
  message: string;
  data?: Record<string, unknown>;
  read: boolean;
  created_at: string;
}

export interface InAppNotificationListResponse {
  data: InAppNotification[];
  meta: { next_cursor: string };
}

export interface UnreadCountResponse {
  data: { count: number };
}
```

### 16b. Add API methods

Add an `inAppNotificationApi` export object:

```typescript
export const inAppNotificationApi = {
  async list(params?: {
    unread?: boolean;
    cursor?: string;
    per_page?: number;
  }): Promise<InAppNotificationListResponse> {
    const searchParams = new URLSearchParams();
    if (params?.unread) searchParams.set("unread", "true");
    if (params?.cursor) searchParams.set("cursor", params.cursor);
    if (params?.per_page) searchParams.set("per_page", String(params.per_page));
    const query = searchParams.toString();
    return apiClient.get(`/customer/notifications${query ? `?${query}` : ""}`);
  },

  async markAsRead(id: string): Promise<void> {
    return apiClient.postVoid(`/customer/notifications/${id}/read`, {});
  },

  async markAllAsRead(): Promise<void> {
    return apiClient.postVoid(`/customer/notifications/read-all`, {});
  },

  async getUnreadCount(): Promise<UnreadCountResponse> {
    return apiClient.get("/customer/notifications/unread-count");
  },
};
```

### 16c. Note on `postVoid`

If the customer `apiClient` does not have a `postVoid` method (the admin one does), add it following the admin pattern:

```typescript
async postVoid(endpoint: string, body: unknown): Promise<void> {
  await apiRequest<void>(endpoint, {
    method: "POST",
    body: JSON.stringify(body),
  });
},
```

### Verification

```bash
cd webui/customer && npm run type-check
```

---

## Task 17: Admin API Client Updates

- [ ] Add in-app notification API methods to the admin frontend API client

**File:** `webui/admin/lib/api-client.ts`

### 17a. Add TypeScript types

Same `InAppNotification`, `InAppNotificationListResponse`, and `UnreadCountResponse` interfaces as Task 16.

### 17b. Add API methods

```typescript
export const inAppNotificationApi = {
  async list(params?: {
    unread?: boolean;
    cursor?: string;
    per_page?: number;
  }): Promise<InAppNotificationListResponse> {
    const searchParams = new URLSearchParams();
    if (params?.unread) searchParams.set("unread", "true");
    if (params?.cursor) searchParams.set("cursor", params.cursor);
    if (params?.per_page) searchParams.set("per_page", String(params.per_page));
    const query = searchParams.toString();
    return apiClient.get(`/admin/notifications${query ? `?${query}` : ""}`);
  },

  async markAsRead(id: string): Promise<void> {
    return apiClient.postVoid(`/admin/notifications/${id}/read`, {});
  },

  async markAllAsRead(): Promise<void> {
    return apiClient.postVoid(`/admin/notifications/read-all`, {});
  },

  async getUnreadCount(): Promise<UnreadCountResponse> {
    return apiClient.get("/admin/notifications/unread-count");
  },
};
```

### Verification

```bash
cd webui/admin && npm run type-check
```

---

## Task 18: Customer SSE Hook

- [ ] Create a React hook for SSE notification streaming in the customer portal

**File:** `webui/customer/hooks/use-notifications.ts`

### 18a. Hook implementation

```typescript
"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { InAppNotification } from "@/lib/api-client";
import { inAppNotificationApi } from "@/lib/api-client";
import { useAuth } from "@/lib/auth-context";

interface UseNotificationsReturn {
  unreadCount: number;
  isConnected: boolean;
}

export function useNotifications(): UseNotificationsReturn {
  const [unreadCount, setUnreadCount] = useState(0);
  const [isConnected, setIsConnected] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);
  const retryCountRef = useRef(0);
  const queryClient = useQueryClient();
  const { user } = useAuth();

  const connect = useCallback(() => {
    if (!user) return;

    const apiBase = process.env.NEXT_PUBLIC_API_URL || "/api/v1";
    const url = `${apiBase}/customer/notifications/stream`;
    const es = new EventSource(url, { withCredentials: true });
    eventSourceRef.current = es;

    es.addEventListener("unread_count", (e: MessageEvent) => {
      const data = JSON.parse(e.data);
      setUnreadCount(data.count);
    });

    es.addEventListener("notification", (e: MessageEvent) => {
      const notification: InAppNotification = JSON.parse(e.data);
      setUnreadCount((prev) => prev + 1);
      queryClient.invalidateQueries({ queryKey: ["notifications"] });
    });

    es.addEventListener("unread_count_changed", (e: MessageEvent) => {
      const data = JSON.parse(e.data);
      setUnreadCount(data.count);
      queryClient.invalidateQueries({ queryKey: ["notifications"] });
    });

    es.addEventListener("heartbeat", () => {
      // Keep-alive, no action needed
    });

    es.onopen = () => {
      setIsConnected(true);
      retryCountRef.current = 0;
    };

    es.onerror = () => {
      setIsConnected(false);
      es.close();
      eventSourceRef.current = null;

      // Exponential backoff: 1s, 2s, 4s, 8s, 16s, max 30s
      const delay = Math.min(
        1000 * Math.pow(2, retryCountRef.current),
        30000,
      );
      retryCountRef.current += 1;
      setTimeout(connect, delay);
    };
  }, [user, queryClient]);

  useEffect(() => {
    connect();
    return () => {
      eventSourceRef.current?.close();
      eventSourceRef.current = null;
    };
  }, [connect]);

  // Fetch initial unread count via REST (SSE sends it too, but this is a fallback)
  useEffect(() => {
    if (!user) return;
    inAppNotificationApi
      .getUnreadCount()
      .then((res) => setUnreadCount(res.data.count))
      .catch(() => {});
  }, [user]);

  return { unreadCount, isConnected };
}
```

### 18b. Key design decisions

- **EventSource with credentials:** `withCredentials: true` sends the JWT cookie for authentication.
- **Auto-reconnect:** Exponential backoff from 1s to 30s on connection errors.
- **TanStack Query invalidation:** On new notification, invalidates the `["notifications"]` query key so any rendered notification list re-fetches.
- **Initial count:** Fetched via REST as a fallback; SSE also sends `unread_count` event on connect.
- **Cleanup:** Closes EventSource on component unmount.

### Verification

```bash
cd webui/customer && npm run type-check
```

---

## Task 19: Admin SSE Hook

- [ ] Create a React hook for SSE notification streaming in the admin portal

**File:** `webui/admin/hooks/use-notifications.ts`

### Implementation

Identical structure to Task 18 with these differences:
- SSE URL: `/admin/notifications/stream` instead of `/customer/notifications/stream`
- Uses admin `useAuth()` context
- Calls `inAppNotificationApi` from admin API client

The hook signature and return type are identical:

```typescript
export function useNotifications(): UseNotificationsReturn
```

### Verification

```bash
cd webui/admin && npm run type-check
```

---

## Task 20: Customer Notification Bell Component

- [ ] Create the notification bell dropdown component for the customer portal

**File:** `webui/customer/components/notification-bell.tsx`

### 20a. Component implementation

```typescript
"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Bell, Check, Loader2 } from "lucide-react";
import {
  Button,
  Badge,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  ScrollArea,
} from "@virtuestack/ui";
import { inAppNotificationApi } from "@/lib/api-client";
import type { InAppNotification } from "@/lib/api-client";
import { useNotifications } from "@/hooks/use-notifications";
import { cn } from "@/lib/utils";

export function NotificationBell() {
  const { unreadCount } = useNotifications();
  const [isOpen, setIsOpen] = useState(false);
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ["notifications"],
    queryFn: () => inAppNotificationApi.list({ per_page: 20 }),
    enabled: isOpen,
  });

  const markAllMutation = useMutation({
    mutationFn: () => inAppNotificationApi.markAllAsRead(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["notifications"] });
    },
  });

  const markOneMutation = useMutation({
    mutationFn: (id: string) => inAppNotificationApi.markAsRead(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["notifications"] });
    },
  });

  const notifications = data?.data ?? [];

  return (
    <DropdownMenu open={isOpen} onOpenChange={setIsOpen}>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="relative">
          <Bell className="h-5 w-5" />
          {unreadCount > 0 && (
            <span className="absolute -top-1 -right-1 flex h-4 w-4 items-center justify-center rounded-full bg-destructive text-[10px] font-bold text-destructive-foreground">
              {unreadCount > 99 ? "99+" : unreadCount}
            </span>
          )}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80">
        <DropdownMenuLabel className="flex items-center justify-between">
          <span>Notifications</span>
          {unreadCount > 0 && (
            <Button
              variant="ghost"
              size="sm"
              className="h-auto p-0 text-xs font-normal text-muted-foreground hover:text-foreground"
              onClick={() => markAllMutation.mutate()}
              disabled={markAllMutation.isPending}
            >
              <Check className="mr-1 h-3 w-3" />
              Mark all read
            </Button>
          )}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <ScrollArea className="max-h-80">
          {isLoading ? (
            <div className="flex items-center justify-center p-4">
              <Loader2 className="h-4 w-4 animate-spin" />
            </div>
          ) : notifications.length === 0 ? (
            <div className="p-4 text-center text-sm text-muted-foreground">
              No notifications
            </div>
          ) : (
            notifications.map((notif: InAppNotification) => (
              <DropdownMenuItem
                key={notif.id}
                className={cn(
                  "flex flex-col items-start gap-1 p-3",
                  !notif.read && "bg-muted/50",
                )}
                onClick={() => {
                  if (!notif.read) markOneMutation.mutate(notif.id);
                }}
              >
                <div className="flex w-full items-center justify-between">
                  <span className="text-sm font-medium">{notif.title}</span>
                  {!notif.read && (
                    <span className="h-2 w-2 rounded-full bg-primary" />
                  )}
                </div>
                <span className="text-xs text-muted-foreground line-clamp-2">
                  {notif.message}
                </span>
                <span className="text-[10px] text-muted-foreground">
                  {new Date(notif.created_at).toLocaleString()}
                </span>
              </DropdownMenuItem>
            ))
          )}
        </ScrollArea>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
```

### 20b. Design details

- **Unread badge:** Red circle with count (caps at "99+")
- **Unread dot:** Blue dot on individual unread notifications
- **Lazy fetch:** Notification list only fetched when dropdown opens (`enabled: isOpen`)
- **Mark as read:** Click unread notification → marks as read via mutation
- **Mark all as read:** Button in dropdown header
- **Scroll area:** Max height 320px with scrollbar for long lists
- **Background highlighting:** Unread notifications have subtle `bg-muted/50` background

### Verification

```bash
cd webui/customer && npm run type-check
```

---

## Task 21: Admin Notification Bell Component

- [ ] Create the notification bell dropdown component for the admin portal

**File:** `webui/admin/components/notification-bell.tsx`

### Implementation

Same component as Task 20 with these differences:
- Imports from `@/lib/api-client` (admin version)
- Imports `useNotifications` from `@/hooks/use-notifications` (admin version)
- Uses admin `inAppNotificationApi` (admin endpoints)

The component UI, interactions, and structure are identical.

### Verification

```bash
cd webui/admin && npm run type-check
```

---

## Task 22: Layout Integration — Bell in Both Headers

- [ ] Add NotificationBell to sidebar header in both customer and admin portals

### 22a. Customer sidebar

**File:** `webui/customer/components/sidebar.tsx`

Add the `NotificationBell` component to the sidebar header area, next to the collapse toggle button. Import and place it:

```tsx
import { NotificationBell } from "./notification-bell";

// In the header section (around the VirtueStack logo/toggle area):
<div className="flex items-center gap-1">
  <NotificationBell />
  <Button variant="ghost" size="icon" onClick={onToggle}>
    <ChevronLeft className={cn("h-4 w-4 transition-transform", collapsed && "rotate-180")} />
  </Button>
</div>
```

When the sidebar is collapsed, the bell icon remains visible since it uses `size="icon"` (square button).

### 22b. Admin sidebar

**File:** `webui/admin/components/sidebar.tsx`

Same placement pattern as customer. Import `NotificationBell` from `./notification-bell` and add it next to the collapse toggle in the sidebar header.

### 22c. Mobile navigation

**Files:**
- `webui/customer/components/mobile-nav.tsx`
- `webui/admin/components/mobile-nav.tsx`

Add `NotificationBell` to the mobile nav header as well, so notifications are accessible on mobile. Place it next to the hamburger menu trigger button.

### Verification

```bash
cd webui/customer && npm run type-check && npm run build
cd webui/admin && npm run type-check && npm run build
```

---

## Task 23: Backend Build + Test Verification

- [ ] Verify all backend code compiles and tests pass

### Steps

```bash
# Build controller (includes all new code)
make build-controller

# Run all unit tests with race detector
make test-race

# Run specific new tests
go test -race -run TestInAppNotificationRepo ./internal/controller/repository/...
go test -race -run TestSSEHub ./internal/controller/services/...
go test -race -run TestInAppNotificationService ./internal/controller/services/...

# Lint (if golangci-lint is available)
make lint
```

All tests must pass. No lint errors. No race conditions.

---

## Task 24: Frontend Build Verification

- [ ] Verify both frontends compile without errors

### Steps

```bash
cd webui/customer && npm run type-check && npm run lint && npm run build
cd webui/admin && npm run type-check && npm run lint && npm run build
```

Both portals must:
- Pass TypeScript strict-mode type checking
- Pass ESLint with no errors
- Build successfully for production

---

## Implementation Order & Dependencies

```
Task 1 (migration) ──────────────────────────────┐
Task 2 (model) ──────────────────────────────────┤
                                                  ▼
Task 3 (repository) ─────────────────────────► Task 4 (repo tests)
Task 5 (SSE hub) ────────────────────────────► Task 6 (hub tests)
                                                  │
                                                  ▼
Task 7 (service) ────────────────────────────► Task 8 (service tests)
                                                  │
                                    ┌─────────────┼─────────────┐
                                    ▼             ▼             ▼
                              Task 9+10      Task 11       Task 14
                           (customer API)  (admin API)   (wiring)
                                    │             │             │
                                    ▼             ▼             ▼
                              Task 12        Task 13       Task 15
                           (cust routes)  (admin routes) (scheduler)
                                    │             │
                                    ▼             ▼
                              Task 23 (backend verification)
                                    │
                     ┌──────────────┼──────────────┐
                     ▼              ▼              ▼
               Task 16         Task 17       Task 18+19
            (cust API TS)   (admin API TS)  (SSE hooks)
                     │              │              │
                     ▼              ▼              ▼
               Task 20         Task 21       Task 22
            (cust bell)     (admin bell)   (layout)
                     │              │              │
                     └──────────────┼──────────────┘
                                    ▼
                              Task 24 (frontend verification)
```

**Parallelizable groups:**
- Tasks 1, 2 can run in parallel
- Tasks 3, 5 can run in parallel (after 1+2)
- Tasks 4, 6 can run in parallel
- Tasks 9+10, 11 can run in parallel (after 7)
- Tasks 16, 17, 18, 19 can all run in parallel
- Tasks 20, 21, 22 can all run in parallel
