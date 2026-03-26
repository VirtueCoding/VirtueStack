package services

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCircuitBreaker(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config)

	require.NotNil(t, cb)
	assert.Equal(t, CircuitBreakerClosed, cb.GetState("any-key"))
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	assert.Equal(t, 5, config.FailureThreshold)
	assert.Equal(t, 2, config.SuccessThreshold)
	assert.Equal(t, 30*time.Second, config.CooldownPeriod)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 30*time.Second, config.Timeout)
}

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	t.Run("new key starts closed", func(t *testing.T) {
		assert.Equal(t, CircuitBreakerClosed, cb.GetState("node-1"))
	})

	t.Run("can attempt when closed", func(t *testing.T) {
		err := cb.CanAttempt("node-1")
		assert.NoError(t, err)
	})

	t.Run("success resets failure count", func(t *testing.T) {
		cb.RecordFailure("node-1", errors.New("timeout"))
		cb.RecordSuccess("node-1")
		assert.Equal(t, CircuitBreakerClosed, cb.GetState("node-1"))
		assert.Equal(t, 0, cb.GetRetryCount("node-1"))
	})
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		CooldownPeriod:   1 * time.Hour,
		MaxRetries:       5,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Record failures below threshold
	cb.RecordFailure("node-1", errors.New("err1"))
	cb.RecordFailure("node-1", errors.New("err2"))
	assert.Equal(t, CircuitBreakerClosed, cb.GetState("node-1"))

	// Third failure crosses threshold
	cb.RecordFailure("node-1", errors.New("err3"))
	assert.Equal(t, CircuitBreakerOpen, cb.GetState("node-1"))
}

func TestCircuitBreaker_OpenState_BlocksAttempts(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		CooldownPeriod:   1 * time.Hour, // Long cooldown so it stays open
		MaxRetries:       5,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	cb.RecordFailure("node-1", errors.New("down"))
	assert.Equal(t, CircuitBreakerOpen, cb.GetState("node-1"))

	err := cb.CanAttempt("node-1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cooldown")
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		CooldownPeriod:   1 * time.Millisecond, // Very short cooldown
		MaxRetries:       5,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	cb.RecordFailure("node-1", errors.New("down"))
	assert.Equal(t, CircuitBreakerOpen, cb.GetState("node-1"))

	// Wait for cooldown to elapse
	time.Sleep(10 * time.Millisecond)

	// CanAttempt should transition to half-open
	err := cb.CanAttempt("node-1")
	assert.NoError(t, err)
	assert.Equal(t, CircuitBreakerHalfOpen, cb.GetState("node-1"))
}

func TestCircuitBreaker_HalfOpen_SuccessCloses(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		CooldownPeriod:   1 * time.Millisecond,
		MaxRetries:       5,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure("node-1", errors.New("down"))
	time.Sleep(10 * time.Millisecond)
	_ = cb.CanAttempt("node-1") // transitions to half-open

	assert.Equal(t, CircuitBreakerHalfOpen, cb.GetState("node-1"))

	// First success in half-open
	cb.RecordSuccess("node-1")
	assert.Equal(t, CircuitBreakerHalfOpen, cb.GetState("node-1"))

	// Second success closes the circuit
	cb.RecordSuccess("node-1")
	assert.Equal(t, CircuitBreakerClosed, cb.GetState("node-1"))
}

func TestCircuitBreaker_HalfOpen_FailureReopens(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		CooldownPeriod:   1 * time.Millisecond,
		MaxRetries:       5,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Open the circuit
	cb.RecordFailure("node-1", errors.New("down"))
	time.Sleep(10 * time.Millisecond)
	_ = cb.CanAttempt("node-1") // transitions to half-open

	assert.Equal(t, CircuitBreakerHalfOpen, cb.GetState("node-1"))

	// Failure in half-open reopens
	cb.RecordFailure("node-1", errors.New("still down"))
	assert.Equal(t, CircuitBreakerOpen, cb.GetState("node-1"))
}

func TestCircuitBreaker_MaxRetries(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 10, // High threshold so it stays closed
		SuccessThreshold: 1,
		CooldownPeriod:   1 * time.Hour,
		MaxRetries:       2,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Record failures to increment retry count
	cb.RecordFailure("node-1", errors.New("err1"))
	cb.RecordFailure("node-1", errors.New("err2"))

	err := cb.CanAttempt("node-1")
	assert.ErrorIs(t, err, ErrMaxRetriesExceeded)
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		CooldownPeriod:   1 * time.Hour,
		MaxRetries:       5,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	cb.RecordFailure("node-1", errors.New("down"))
	assert.Equal(t, CircuitBreakerOpen, cb.GetState("node-1"))

	cb.Reset("node-1")
	assert.Equal(t, CircuitBreakerClosed, cb.GetState("node-1"))
	assert.NoError(t, cb.CanAttempt("node-1"))
}

func TestCircuitBreaker_ResetAll(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		CooldownPeriod:   1 * time.Hour,
		MaxRetries:       5,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	cb.RecordFailure("node-1", errors.New("down"))
	cb.RecordFailure("node-2", errors.New("down"))

	cb.ResetAll()
	assert.Equal(t, CircuitBreakerClosed, cb.GetState("node-1"))
	assert.Equal(t, CircuitBreakerClosed, cb.GetState("node-2"))
}

func TestCircuitBreaker_GetStats(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		CooldownPeriod:   1 * time.Hour,
		MaxRetries:       3,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Record some activity
	cb.RecordSuccess("node-1")
	cb.RecordFailure("node-1", errors.New("timeout"))

	stats := cb.GetStats("node-1")
	assert.Equal(t, "closed", stats.State)
	assert.Equal(t, 1, stats.FailureCount)
	assert.Equal(t, 1, stats.SuccessCount)
	assert.Equal(t, "timeout", stats.LastError)
	assert.False(t, stats.LastFailureAt.IsZero())
}

func TestCircuitBreaker_GetStats_OpenWithCooldown(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		CooldownPeriod:   1 * time.Hour,
		MaxRetries:       5,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	cb.RecordFailure("node-1", errors.New("down"))
	stats := cb.GetStats("node-1")

	assert.Equal(t, "open", stats.State)
	assert.NotEmpty(t, stats.CooldownRemaining)
}

func TestCircuitBreaker_IsInCooldown(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		CooldownPeriod:   1 * time.Hour,
		MaxRetries:       5,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	t.Run("not in cooldown when closed", func(t *testing.T) {
		assert.False(t, cb.IsInCooldown("node-1"))
	})

	t.Run("in cooldown when open", func(t *testing.T) {
		cb.RecordFailure("node-2", errors.New("down"))
		assert.True(t, cb.IsInCooldown("node-2"))
	})
}

func TestCircuitBreaker_TimeUntilRetry(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		CooldownPeriod:   1 * time.Hour,
		MaxRetries:       5,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	t.Run("zero for closed circuit", func(t *testing.T) {
		assert.Equal(t, time.Duration(0), cb.TimeUntilRetry("node-1"))
	})

	t.Run("positive for open circuit in cooldown", func(t *testing.T) {
		cb.RecordFailure("node-2", errors.New("down"))
		d := cb.TimeUntilRetry("node-2")
		assert.Greater(t, d, time.Duration(0))
	})
}

func TestCircuitBreaker_MultipleKeys(t *testing.T) {
	config := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		CooldownPeriod:   1 * time.Hour,
		MaxRetries:       5,
		Timeout:          30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Break node-1 but not node-2
	cb.RecordFailure("node-1", errors.New("down"))
	cb.RecordSuccess("node-2")

	assert.Equal(t, CircuitBreakerOpen, cb.GetState("node-1"))
	assert.Equal(t, CircuitBreakerClosed, cb.GetState("node-2"))
}

func TestNewFailoverCircuitBreaker(t *testing.T) {
	fb := NewFailoverCircuitBreaker()
	require.NotNil(t, fb)

	t.Run("can attempt failover initially", func(t *testing.T) {
		err := fb.CanAttemptFailover("node-1")
		assert.NoError(t, err)
	})

	t.Run("records failover failure", func(t *testing.T) {
		fb.RecordFailoverFailure("node-1", errors.New("failover failed"))
		fb.RecordFailoverFailure("node-1", errors.New("failover failed again"))
		// Threshold is 2, so should be open now
		stats := fb.GetFailoverStats("node-1")
		assert.Equal(t, "open", stats.State)
	})

	t.Run("reset node", func(t *testing.T) {
		fb.ResetNode("node-1")
		err := fb.CanAttemptFailover("node-1")
		assert.NoError(t, err)
	})

	t.Run("records failover success", func(t *testing.T) {
		fb.RecordFailoverSuccess("node-2")
		stats := fb.GetFailoverStats("node-2")
		assert.Equal(t, "closed", stats.State)
	})
}
