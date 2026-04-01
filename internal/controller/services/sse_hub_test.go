package services

import (
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHub() *SSEHub {
	return NewSSEHub(slog.Default())
}

func TestSSEHub_RegisterUnregister(t *testing.T) {
	hub := newTestHub()
	ch := make(chan SSEEvent, 1)

	hub.Register("user-1", ch)
	assert.Equal(t, 1, hub.ConnectionCount("user-1"))

	hub.Unregister("user-1", ch)
	assert.Equal(t, 0, hub.ConnectionCount("user-1"))
}

func TestSSEHub_BroadcastSingleClient(t *testing.T) {
	hub := newTestHub()
	ch := make(chan SSEEvent, 1)
	hub.Register("user-1", ch)
	defer hub.Unregister("user-1", ch)

	event := SSEEvent{Type: "test", Data: json.RawMessage(`{"key":"value"}`)}
	hub.Broadcast("user-1", event)

	select {
	case received := <-ch:
		assert.Equal(t, "test", received.Type)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestSSEHub_BroadcastMultipleClients(t *testing.T) {
	hub := newTestHub()
	ch1 := make(chan SSEEvent, 1)
	ch2 := make(chan SSEEvent, 1)
	hub.Register("user-1", ch1)
	hub.Register("user-1", ch2)
	defer hub.Unregister("user-1", ch1)
	defer hub.Unregister("user-1", ch2)

	assert.Equal(t, 2, hub.ConnectionCount("user-1"))

	event := SSEEvent{Type: "multi", Data: json.RawMessage(`{}`)}
	hub.Broadcast("user-1", event)

	require.Equal(t, "multi", (<-ch1).Type)
	require.Equal(t, "multi", (<-ch2).Type)
}

func TestSSEHub_BroadcastNoClients(t *testing.T) {
	hub := newTestHub()
	// Should not panic when broadcasting to nonexistent user
	hub.Broadcast("nobody", SSEEvent{Type: "test", Data: json.RawMessage(`{}`)})
}

func TestSSEHub_SlowClientDropsEvent(t *testing.T) {
	hub := newTestHub()
	// Unbuffered channel — will block
	ch := make(chan SSEEvent)
	hub.Register("user-1", ch)
	defer hub.Unregister("user-1", ch)

	// Should not block — event is dropped
	hub.Broadcast("user-1", SSEEvent{Type: "dropped", Data: json.RawMessage(`{}`)})
}

func TestSSEHub_MultiUserIsolation(t *testing.T) {
	hub := newTestHub()
	ch1 := make(chan SSEEvent, 1)
	ch2 := make(chan SSEEvent, 1)
	hub.Register("user-1", ch1)
	hub.Register("user-2", ch2)
	defer hub.Unregister("user-1", ch1)
	defer hub.Unregister("user-2", ch2)

	hub.Broadcast("user-1", SSEEvent{Type: "only-user1", Data: json.RawMessage(`{}`)})

	select {
	case received := <-ch1:
		assert.Equal(t, "only-user1", received.Type)
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	select {
	case <-ch2:
		t.Fatal("user-2 should not receive user-1 event")
	default:
		// Expected: no event for user-2
	}
}

func TestSSEHub_ConcurrentAccess(t *testing.T) {
	hub := newTestHub()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			userID := "concurrent-user"
			ch := make(chan SSEEvent, 10)
			hub.Register(userID, ch)
			hub.Broadcast(userID, SSEEvent{Type: "concurrent", Data: json.RawMessage(`{}`)})
			hub.ConnectionCount(userID)
			hub.Unregister(userID, ch)
		}(i)
	}

	wg.Wait()
	assert.Equal(t, 0, hub.ConnectionCount("concurrent-user"))
}

func TestSSEHub_UnregisterNonexistent(t *testing.T) {
	hub := newTestHub()
	ch := make(chan SSEEvent, 1)
	// Should not panic
	hub.Unregister("nonexistent", ch)
}
