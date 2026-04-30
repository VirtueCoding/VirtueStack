// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrCircuitBreakerOpen     = errors.New("circuit breaker is open")
	ErrCircuitBreakerCooldown = errors.New("circuit breaker is in cooldown period")
	ErrMaxRetriesExceeded     = errors.New("maximum retry count exceeded")
)

// CircuitBreakerState represents the current state of a circuit breaker.
type CircuitBreakerState string

const (
	CircuitBreakerClosed   CircuitBreakerState = "closed"
	CircuitBreakerOpen     CircuitBreakerState = "open"
	CircuitBreakerHalfOpen CircuitBreakerState = "half-open"
)

// CircuitBreakerConfig holds configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures before opening
	FailureThreshold int
	// SuccessThreshold is the number of consecutive successes to close from half-open
	SuccessThreshold int
	// CooldownPeriod is how long the circuit stays open before transitioning to half-open
	CooldownPeriod time.Duration
	// MaxRetries is the maximum number of retry attempts per operation
	MaxRetries int
	// Timeout is the timeout for individual operations
	Timeout time.Duration
}

// DefaultCircuitBreakerConfig returns a sensible default configuration.
// Values align with CODING_STANDARD: FailureThreshold=5, CooldownPeriod=30s.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		CooldownPeriod:   30 * time.Second,
		MaxRetries:       3,
		Timeout:          30 * time.Second,
	}
}

// CircuitBreakerEntry holds the state for a single circuit breaker instance.
type CircuitBreakerEntry struct {
	State         CircuitBreakerState
	FailureCount  int
	SuccessCount  int
	LastFailureAt time.Time
	LastAttemptAt time.Time
	RetryCount    int
	LastError     error
}

// CircuitBreaker implements the circuit breaker pattern to prevent flapping
// and cascading failures in distributed systems.
type CircuitBreaker struct {
	config  CircuitBreakerConfig
	mu      sync.RWMutex
	entries map[string]*CircuitBreakerEntry
}

// NewCircuitBreaker creates a new CircuitBreaker with the given configuration.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config:  config,
		entries: make(map[string]*CircuitBreakerEntry),
	}
}

// getEntry returns the circuit breaker entry for the given key, creating one if needed.
func (cb *CircuitBreaker) getEntry(key string) *CircuitBreakerEntry {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	entry, exists := cb.entries[key]
	if !exists {
		entry = &CircuitBreakerEntry{
			State: CircuitBreakerClosed,
		}
		cb.entries[key] = entry
	}
	return entry
}

// CanAttempt checks if an operation can be attempted for the given key.
// Returns an error if the circuit is open or in cooldown.
func (cb *CircuitBreaker) CanAttempt(key string) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	entry, exists := cb.entries[key]
	if !exists {
		entry = &CircuitBreakerEntry{State: CircuitBreakerClosed}
		cb.entries[key] = entry
	}

	switch entry.State {
	case CircuitBreakerOpen:
		timeSinceFailure := time.Since(entry.LastFailureAt)
		if timeSinceFailure < cb.config.CooldownPeriod {
			remaining := cb.config.CooldownPeriod - timeSinceFailure
			return errors.New("circuit breaker is in cooldown period, retry after " + remaining.String())
		}
		// Cooldown has elapsed: transition to HalfOpen and allow a probe attempt (F-183).
		entry.State = CircuitBreakerHalfOpen
		entry.SuccessCount = 0
		entry.RetryCount = 0
		return nil

	case CircuitBreakerHalfOpen:
		if entry.RetryCount >= cb.config.MaxRetries {
			return ErrMaxRetriesExceeded
		}
		return nil

	case CircuitBreakerClosed:
		if entry.RetryCount >= cb.config.MaxRetries {
			return ErrMaxRetriesExceeded
		}
		return nil

	default:
		return nil
	}
}

// RecordSuccess records a successful operation for the given key.
func (cb *CircuitBreaker) RecordSuccess(key string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	entry, exists := cb.entries[key]
	if !exists {
		entry = &CircuitBreakerEntry{State: CircuitBreakerClosed}
		cb.entries[key] = entry
	}

	entry.SuccessCount++
	entry.LastAttemptAt = time.Now()
	entry.RetryCount = 0
	entry.LastError = nil

	switch entry.State {
	case CircuitBreakerHalfOpen:
		if entry.SuccessCount >= cb.config.SuccessThreshold {
			entry.State = CircuitBreakerClosed
			entry.FailureCount = 0
			entry.SuccessCount = 0
		}

	case CircuitBreakerOpen:
		entry.State = CircuitBreakerHalfOpen
		entry.SuccessCount = 1

	case CircuitBreakerClosed:
		entry.FailureCount = 0
	}
}

// RecordFailure records a failed operation for the given key.
func (cb *CircuitBreaker) RecordFailure(key string, err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	entry, exists := cb.entries[key]
	if !exists {
		entry = &CircuitBreakerEntry{State: CircuitBreakerClosed}
		cb.entries[key] = entry
	}

	entry.FailureCount++
	entry.LastFailureAt = time.Now()
	entry.LastAttemptAt = time.Now()
	entry.RetryCount++
	entry.LastError = err

	switch entry.State {
	case CircuitBreakerClosed:
		if entry.FailureCount >= cb.config.FailureThreshold {
			entry.State = CircuitBreakerOpen
			entry.SuccessCount = 0
		}

	case CircuitBreakerHalfOpen:
		entry.State = CircuitBreakerOpen
		entry.SuccessCount = 0

	case CircuitBreakerOpen:
		// Already open, failure just extends cooldown
		entry.SuccessCount = 0
	}
}

// getEntryLocked returns the circuit breaker entry for the given key under an
// already-held write lock. Creates a new entry if one does not exist.
// Callers must hold cb.mu (write lock) before calling this method.
func (cb *CircuitBreaker) getEntryLocked(key string) *CircuitBreakerEntry {
	entry, exists := cb.entries[key]
	if !exists {
		entry = &CircuitBreakerEntry{
			State: CircuitBreakerClosed,
		}
		cb.entries[key] = entry
	}
	return entry
}

// GetState returns the current state of the circuit breaker for the given key.
// Uses a single write lock via getEntryLocked to avoid the TOCTOU race between
// getEntry (write lock) and the subsequent RLock (F-033).
func (cb *CircuitBreaker) GetState(key string) CircuitBreakerState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.getEntryLocked(key).State
}

// GetRetryCount returns the current retry count for the given key.
// Uses a single write lock via getEntryLocked to avoid the TOCTOU race (F-033).
func (cb *CircuitBreaker) GetRetryCount(key string) int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.getEntryLocked(key).RetryCount
}

// Reset resets the circuit breaker for the given key.
func (cb *CircuitBreaker) Reset(key string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	delete(cb.entries, key)
}

// ResetAll resets all circuit breaker entries.
func (cb *CircuitBreaker) ResetAll() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.entries = make(map[string]*CircuitBreakerEntry)
}

// CircuitBreakerStats holds statistics about a circuit breaker instance.
type CircuitBreakerStats struct {
	State            string    `json:"state"`
	FailureCount     int       `json:"failure_count"`
	SuccessCount     int       `json:"success_count"`
	RetryCount       int       `json:"retry_count"`
	LastAttemptAt    time.Time `json:"last_attempt_at"`
	LastFailureAt    time.Time `json:"last_failure_at,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
	CooldownRemaining string   `json:"cooldown_remaining,omitempty"`
}

// GetStats returns statistics about the circuit breaker for the given key.
// Uses a single write lock via getEntryLocked to avoid the TOCTOU race (F-033).
func (cb *CircuitBreaker) GetStats(key string) CircuitBreakerStats {
	cb.mu.Lock()
	entry := cb.getEntryLocked(key)

	stats := CircuitBreakerStats{
		State:         string(entry.State),
		FailureCount:  entry.FailureCount,
		SuccessCount:  entry.SuccessCount,
		RetryCount:    entry.RetryCount,
		LastAttemptAt: entry.LastAttemptAt,
	}

	if !entry.LastFailureAt.IsZero() {
		stats.LastFailureAt = entry.LastFailureAt
	}

	if entry.LastError != nil {
		stats.LastError = entry.LastError.Error()
	}

	if entry.State == CircuitBreakerOpen {
		timeSinceFailure := time.Since(entry.LastFailureAt)
		if timeSinceFailure < cb.config.CooldownPeriod {
			stats.CooldownRemaining = (cb.config.CooldownPeriod - timeSinceFailure).String()
		}
	}

	cb.mu.Unlock()
	return stats
}

// IsInCooldown checks if the circuit breaker is in cooldown for the given key.
// Uses a single write lock via getEntryLocked to avoid the TOCTOU race (F-033).
func (cb *CircuitBreaker) IsInCooldown(key string) bool {
	cb.mu.Lock()
	entry := cb.getEntryLocked(key)
	state := entry.State
	lastFailureAt := entry.LastFailureAt
	cb.mu.Unlock()

	if state != CircuitBreakerOpen {
		return false
	}

	return time.Since(lastFailureAt) < cb.config.CooldownPeriod
}

// TimeUntilRetry returns the duration until the next retry attempt is allowed.
// Returns 0 if retry is allowed immediately.
// Uses a single write lock via getEntryLocked to avoid the TOCTOU race (F-033).
func (cb *CircuitBreaker) TimeUntilRetry(key string) time.Duration {
	cb.mu.Lock()
	entry := cb.getEntryLocked(key)
	state := entry.State
	lastFailureAt := entry.LastFailureAt
	cb.mu.Unlock()

	if state != CircuitBreakerOpen {
		return 0
	}

	elapsed := time.Since(lastFailureAt)
	if elapsed >= cb.config.CooldownPeriod {
		return 0
	}

	return cb.config.CooldownPeriod - elapsed
}

// FailoverCircuitBreaker is a specialized circuit breaker for node failover operations.
type FailoverCircuitBreaker struct {
	*innerBreaker
}

type innerBreaker = CircuitBreaker

// NewFailoverCircuitBreaker creates a circuit breaker configured for failover operations.
func NewFailoverCircuitBreaker() *FailoverCircuitBreaker {
	config := CircuitBreakerConfig{
		FailureThreshold: 2,
		SuccessThreshold: 1,
		CooldownPeriod:   10 * time.Minute,
		MaxRetries:       5,
		Timeout:          60 * time.Second,
	}
	return &FailoverCircuitBreaker{
		innerBreaker: NewCircuitBreaker(config),
	}
}

// CanAttemptFailover checks if a failover can be attempted for the given node.
func (fb *FailoverCircuitBreaker) CanAttemptFailover(nodeID string) error {
	return fb.CanAttempt(nodeID)
}

// RecordFailoverSuccess records a successful failover for the given node.
func (fb *FailoverCircuitBreaker) RecordFailoverSuccess(nodeID string) {
	fb.RecordSuccess(nodeID)
}

// RecordFailoverFailure records a failed failover for the given node.
func (fb *FailoverCircuitBreaker) RecordFailoverFailure(nodeID string, err error) {
	fb.RecordFailure(nodeID, err)
}

// GetFailoverStats returns statistics about failover attempts for a node.
func (fb *FailoverCircuitBreaker) GetFailoverStats(nodeID string) CircuitBreakerStats {
	return fb.GetStats(nodeID)
}

// ResetNode resets the failover circuit breaker for a specific node.
func (fb *FailoverCircuitBreaker) ResetNode(nodeID string) {
	fb.Reset(nodeID)
}
