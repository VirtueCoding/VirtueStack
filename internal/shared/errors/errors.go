// Package errors provides custom error types for VirtueStack.
// These error types support structured error handling with contextual information
// and proper error wrapping for use with errors.Is() and errors.As().
package errors

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
)

// Sentinel errors for expected conditions.
// These errors represent common failure modes that can be checked using errors.Is().
var (
	// ErrNotFound indicates that a requested resource was not found.
	ErrNotFound = stderrors.New("resource not found")

	// ErrAlreadyExists indicates that a resource already exists when trying to create it.
	ErrAlreadyExists = stderrors.New("resource already exists")

	// ErrUnauthorized indicates missing or invalid authentication credentials.
	ErrUnauthorized = stderrors.New("unauthorized")

	// ErrForbidden indicates that the authenticated user lacks permission for the operation.
	ErrForbidden = stderrors.New("forbidden")

	// ErrValidation indicates that input validation failed.
	ErrValidation = stderrors.New("validation error")

	// ErrConflict indicates a resource conflict, such as a version mismatch.
	ErrConflict = stderrors.New("resource conflict")

	// ErrRateLimited indicates that the request was rate limited.
	ErrRateLimited = stderrors.New("rate limit exceeded")

	// ErrServiceDown indicates that a required service is unavailable.
	ErrServiceDown = stderrors.New("service unavailable")

	// ErrTimeout indicates that an operation timed out.
	ErrTimeout = stderrors.New("operation timed out")

	// ErrNoIPMIConfigured indicates that IPMI is not configured for a node.
	ErrNoIPMIConfigured = stderrors.New("IPMI not configured for this node")
)

// ValidationDetail represents a single validation error detail.
type ValidationDetail struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

// ValidationError carries field-level validation error details.
// It wraps ErrValidation and provides structured information about
// which field failed validation and why.
type ValidationError struct {
	Field string
	Issue string
}

// Error implements the error interface for ValidationError.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: field %q - %s", e.Field, e.Issue)
}

// Unwrap returns ErrValidation to support errors.Is() checks.
func (e *ValidationError) Unwrap() error {
	return ErrValidation
}

// NewValidationError creates a new ValidationError with the given field and issue.
func NewValidationError(field, issue string) *ValidationError {
	return &ValidationError{
		Field: field,
		Issue: issue,
	}
}

// OperationError represents a failure in a multi-step operation.
// It captures which operation and step failed, along with the underlying error.
type OperationError struct {
	Operation string
	Step      string
	Err       error
}

// Error implements the error interface for OperationError.
func (e *OperationError) Error() string {
	if e.Step != "" {
		return fmt.Sprintf("operation %q failed at step %q: %v", e.Operation, e.Step, e.Err)
	}
	return fmt.Sprintf("operation %q failed: %v", e.Operation, e.Err)
}

// Unwrap returns the underlying error to support errors.Is() and errors.As().
func (e *OperationError) Unwrap() error {
	return e.Err
}

// NewOperationError creates a new OperationError with the given details.
func NewOperationError(operation, step string, err error) *OperationError {
	return &OperationError{
		Operation: operation,
		Step:      step,
		Err:       err,
	}
}

// APIError represents an error suitable for HTTP API responses.
// It includes a machine-readable code, human-readable message,
// optional validation details, and the appropriate HTTP status code.
type APIError struct {
	Code       string            `json:"code"`
	Message    string            `json:"message"`
	Details    []ValidationDetail `json:"details,omitempty"`
	HTTPStatus int               `json:"-"`
}

// Error implements the error interface for APIError.
func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ToJSON returns the JSON representation of the APIError.
// This is the standard error response format for VirtueStack APIs.
func (e *APIError) ToJSON() []byte {
	data, err := json.Marshal(e)
	if err != nil {
		// Fallback to a simple error message if marshaling fails
		return []byte(fmt.Sprintf(`{"code":"INTERNAL_ERROR","message":"%s"}`, err.Error()))
	}
	return data
}

// NewAPIError creates a new APIError with the given code, message, and HTTP status.
func NewAPIError(code string, message string, httpStatus int) *APIError {
	return &APIError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// NewAPIValidationError creates an APIError with validation details.
func NewAPIValidationError(code string, message string, details []ValidationDetail) *APIError {
	return &APIError{
		Code:       code,
		Message:    message,
		Details:    details,
		HTTPStatus: 400,
	}
}

// Is re-exports stdlib errors.Is for convenience.
// It checks if err wraps target using errors.Is semantics.
func Is(err, target error) bool {
	return stderrors.Is(err, target)
}

// As re-exports stdlib errors.As for convenience.
// It attempts to cast err to target using errors.As semantics.
func As(err error, target any) bool {
	return stderrors.As(err, target)
}