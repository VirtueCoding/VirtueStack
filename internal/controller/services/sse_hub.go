package services

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// SSEEvent represents a server-sent event payload.
type SSEEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// SSEHub manages SSE client connections and broadcasts events to connected users.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[string]map[chan SSEEvent]struct{}
	logger  *slog.Logger
}

// NewSSEHub creates a new SSEHub.
func NewSSEHub(logger *slog.Logger) *SSEHub {
	return &SSEHub{
		clients: make(map[string]map[chan SSEEvent]struct{}),
		logger:  logger.With("component", "sse-hub"),
	}
}

// Register adds a client channel to a user's connection set.
func (h *SSEHub) Register(userID string, ch chan SSEEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[chan SSEEvent]struct{})
	}
	h.clients[userID][ch] = struct{}{}
}

// Unregister removes a client channel from a user's connection set.
func (h *SSEHub) Unregister(userID string, ch chan SSEEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conns, ok := h.clients[userID]; ok {
		delete(conns, ch)
		if len(conns) == 0 {
			delete(h.clients, userID)
		}
	}
}

// Broadcast sends an event to all connected clients of a user (non-blocking).
func (h *SSEHub) Broadcast(userID string, event SSEEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns, ok := h.clients[userID]
	if !ok {
		return
	}
	for ch := range conns {
		select {
		case ch <- event:
		default:
			h.logger.Debug("dropping SSE event for slow client", "user_id", userID)
		}
	}
}

// ConnectionCount returns the number of active SSE connections for a user.
func (h *SSEHub) ConnectionCount(userID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID])
}
