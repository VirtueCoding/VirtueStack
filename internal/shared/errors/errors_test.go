package errors

import (
	stderrors "errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationError(t *testing.T) {
	t.Run("Error method", func(t *testing.T) {
		err := &ValidationError{Field: "hostname", Issue: "invalid format"}
		assert.Contains(t, err.Error(), "hostname")
		assert.Contains(t, err.Error(), "invalid format")
	})

	t.Run("Unwrap returns ErrValidation", func(t *testing.T) {
		err := &ValidationError{Field: "email", Issue: "required"}
		assert.True(t, stderrors.Is(err, ErrValidation))
	})
}

func TestNewValidationError(t *testing.T) {
	err := NewValidationError("name", "too short")
	assert.Equal(t, "name", err.Field)
	assert.Equal(t, "too short", err.Issue)
	assert.True(t, stderrors.Is(err, ErrValidation))
}

func TestOperationError(t *testing.T) {
	t.Run("Error with step", func(t *testing.T) {
		inner := stderrors.New("disk full")
		err := &OperationError{Operation: "vm.create", Step: "allocate_disk", Err: inner}
		msg := err.Error()
		assert.Contains(t, msg, "vm.create")
		assert.Contains(t, msg, "allocate_disk")
		assert.Contains(t, msg, "disk full")
	})

	t.Run("Error without step", func(t *testing.T) {
		inner := stderrors.New("network error")
		err := &OperationError{Operation: "vm.migrate", Err: inner}
		msg := err.Error()
		assert.Contains(t, msg, "vm.migrate")
		assert.NotContains(t, msg, "step")
	})

	t.Run("Unwrap returns inner error", func(t *testing.T) {
		inner := stderrors.New("timeout")
		err := &OperationError{Operation: "test", Step: "connect", Err: inner}
		assert.True(t, stderrors.Is(err, inner))
	})

	t.Run("Unwrap preserves error chain", func(t *testing.T) {
		err := NewOperationError("vm.create", "allocate_ip", ErrNotFound)
		assert.True(t, stderrors.Is(err, ErrNotFound))
	})
}

func TestNewOperationError(t *testing.T) {
	inner := stderrors.New("connection refused")
	err := NewOperationError("grpc.call", "connect", inner)
	assert.Equal(t, "grpc.call", err.Operation)
	assert.Equal(t, "connect", err.Step)
	assert.Equal(t, inner, err.Err)
}

func TestAPIError(t *testing.T) {
	t.Run("Error method", func(t *testing.T) {
		err := &APIError{Code: "NOT_FOUND", Message: "VM not found", HTTPStatus: 404}
		assert.Equal(t, "NOT_FOUND: VM not found", err.Error())
	})

	t.Run("ToJSON success", func(t *testing.T) {
		err := &APIError{
			Code:       "VALIDATION_ERROR",
			Message:    "Invalid hostname",
			HTTPStatus: http.StatusBadRequest,
			Details: []ValidationDetail{
				{Field: "hostname", Issue: "must be RFC 1123"},
			},
		}
		data := err.ToJSON()
		assert.Contains(t, string(data), "VALIDATION_ERROR")
		assert.Contains(t, string(data), "hostname")
	})
}

func TestNewAPIError(t *testing.T) {
	err := NewAPIError("CONFLICT", "Resource already exists", http.StatusConflict)
	assert.Equal(t, "CONFLICT", err.Code)
	assert.Equal(t, "Resource already exists", err.Message)
	assert.Equal(t, http.StatusConflict, err.HTTPStatus)
	assert.Nil(t, err.Details)
}

func TestNewAPIValidationError(t *testing.T) {
	details := []ValidationDetail{
		{Field: "email", Issue: "invalid format"},
		{Field: "name", Issue: "required"},
	}
	err := NewAPIValidationError("VALIDATION_ERROR", "Input validation failed", details)
	assert.Equal(t, "VALIDATION_ERROR", err.Code)
	assert.Equal(t, http.StatusBadRequest, err.HTTPStatus)
	assert.Len(t, err.Details, 2)
}

func TestSentinelErrors(t *testing.T) {
	// Verify all sentinel errors are distinct
	sentinels := []error{
		ErrNotFound, ErrAlreadyExists, ErrUnauthorized, ErrForbidden,
		ErrValidation, ErrConflict, ErrRateLimited, ErrServiceDown,
		ErrTimeout, ErrNoIPMIConfigured, ErrTwoFAAlreadyEnabled,
		ErrTwoFANotEnabled, ErrTwoFASetupNotInitiated, ErrInvalidVMState,
		ErrVMNotRunning, ErrPlanHasExistingVMs, ErrAccountLocked,
		ErrNoRowsAffected, ErrLimitExceeded,
	}

	// Verify each sentinel has a meaningful message
	for _, s := range sentinels {
		assert.NotEmpty(t, s.Error(), "sentinel error should have a message")
	}

	// Verify sentinels are distinct from each other
	for i := range sentinels {
		for j := range sentinels {
			if i != j {
				assert.False(t, stderrors.Is(sentinels[i], sentinels[j]),
					"%v should not match %v", sentinels[i], sentinels[j])
			}
		}
	}
}

func TestIs_Convenience(t *testing.T) {
	err := &ValidationError{Field: "x", Issue: "y"}
	assert.True(t, Is(err, ErrValidation))
	assert.False(t, Is(err, ErrNotFound))
}

func TestAs_Convenience(t *testing.T) {
	err := &OperationError{Operation: "test", Err: stderrors.New("inner")}
	var opErr *OperationError
	require.True(t, As(err, &opErr))
	assert.Equal(t, "test", opErr.Operation)
}

func TestValidationError_IsNotErrNotFound(t *testing.T) {
	err := &ValidationError{Field: "x", Issue: "y"}
	assert.False(t, stderrors.Is(err, ErrNotFound))
}

func TestOperationError_WrapsCustomError(t *testing.T) {
	inner := NewValidationError("field", "issue")
	outer := NewOperationError("op", "step", inner)

	// Should be able to unwrap to the ValidationError
	var ve *ValidationError
	assert.True(t, stderrors.As(outer, &ve))
	assert.Equal(t, "field", ve.Field)

	// Should also match ErrValidation sentinel
	assert.True(t, stderrors.Is(outer, ErrValidation))
}
