// Package middleware provides HTTP middleware for the VirtueStack Controller.
package middleware

import (
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"unicode"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	googleuuid "github.com/google/uuid"
)

// validate is the package-level singleton validator instance.
// Registering custom validators once is more efficient than creating per-request instances.
var (
	validate     *validator.Validate
	validateOnce sync.Once
)

// getValidator returns the singleton validator, initialising it on first call.
func getValidator() *validator.Validate {
	validateOnce.Do(func() {
		validate = validator.New()
		registerCustomValidations(validate)
	})
	return validate
}

// registerCustomValidations adds VirtueStack-specific validation tags.
func registerCustomValidations(v *validator.Validate) {
	// "uuid4" validates UUID v4 format.
	_ = v.RegisterValidation("uuid4", func(fl validator.FieldLevel) bool {
		return ValidateUUID(fl.Field().String()) == nil
	})

	// "hostname_rfc1123" validates hostnames per RFC 1123.
	_ = v.RegisterValidation("hostname_rfc1123", func(fl validator.FieldLevel) bool {
		return ValidateHostname(fl.Field().String()) == nil
	})
}

// BindAndValidate binds the request body into dst (must be a pointer to a struct)
// and then runs go-playground/validator validation against it.
// On validation failure, returns a *sharederrors.APIError with field-level details
// so the caller can forward them directly to the client.
func BindAndValidate(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return &sharederrors.APIError{
			Code:       "INVALID_REQUEST_BODY",
			Message:    "request body could not be parsed as JSON",
			HTTPStatus: http.StatusBadRequest,
			Details: []sharederrors.ValidationDetail{
				{Field: "body", Issue: err.Error()},
			},
		}
	}

	if err := getValidator().Struct(dst); err != nil {
		return buildValidationError(err)
	}

	return nil
}

// ValidateUUID validates that id is a well-formed UUID v4.
// Returns a *sharederrors.ValidationError on failure.
func ValidateUUID(id string) error {
	parsed, err := googleuuid.Parse(id)
	if err != nil {
		return &sharederrors.ValidationError{
			Field: "id",
			Issue: fmt.Sprintf("value %q is not a valid UUID: %v", id, err),
		}
	}

	// Enforce version 4.
	if parsed.Version() != 4 {
		return &sharederrors.ValidationError{
			Field: "id",
			Issue: fmt.Sprintf("UUID must be version 4, got version %d", parsed.Version()),
		}
	}

	return nil
}

// hostnameRegex matches valid RFC 1123 hostnames.
// Labels may contain letters, digits, and hyphens; they must not start or end with a hyphen.
var hostnameRegex = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)

// ValidateHostname validates hostname per RFC 1123.
// Returns a *sharederrors.ValidationError on failure.
func ValidateHostname(hostname string) error {
	if len(hostname) == 0 || len(hostname) > 253 {
		return &sharederrors.ValidationError{
			Field: "hostname",
			Issue: "hostname must be between 1 and 253 characters",
		}
	}

	// Hostnames must not contain non-ASCII characters.
	for _, r := range hostname {
		if r > unicode.MaxASCII {
			return &sharederrors.ValidationError{
				Field: "hostname",
				Issue: "hostname must only contain ASCII characters",
			}
		}
	}

	if !hostnameRegex.MatchString(hostname) {
		return &sharederrors.ValidationError{
			Field: "hostname",
			Issue: fmt.Sprintf("value %q is not a valid RFC 1123 hostname", hostname),
		}
	}

	return nil
}

// ─── internal helpers ────────────────────────────────────────────────────────

// buildValidationError converts a go-playground/validator error into a structured
// *sharederrors.APIError with field-level details.
func buildValidationError(err error) *sharederrors.APIError {
	var details []sharederrors.ValidationDetail

	var validationErrs validator.ValidationErrors
	if ok := sharederrors.As(err, &validationErrs); ok {
		for _, fe := range validationErrs {
			details = append(details, sharederrors.ValidationDetail{
				Field: fieldName(fe),
				Issue: fieldIssue(fe),
			})
		}
	} else {
		details = []sharederrors.ValidationDetail{
			{Field: "unknown", Issue: err.Error()},
		}
	}

	return &sharederrors.APIError{
		Code:       "VALIDATION_ERROR",
		Message:    "one or more fields failed validation",
		HTTPStatus: http.StatusBadRequest,
		Details:    details,
	}
}

// fieldName returns the JSON-friendly field name from a validator.FieldError.
func fieldName(fe validator.FieldError) string {
	// fe.Field() returns the struct field name; fe.Namespace() includes parent path.
	ns := fe.Namespace()
	// Strip the top-level struct name (everything before and including the first dot).
	if idx := indexOf(ns, '.'); idx >= 0 {
		return ns[idx+1:]
	}
	return fe.Field()
}

// fieldIssue constructs a human-readable validation message.
func fieldIssue(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "field is required"
	case "min":
		return fmt.Sprintf("must be at least %s characters", fe.Param())
	case "max":
		return fmt.Sprintf("must be at most %s characters", fe.Param())
	case "email":
		return "must be a valid email address"
	case "uuid4":
		return "must be a valid UUID v4"
	case "hostname_rfc1123":
		return "must be a valid RFC 1123 hostname"
	case "oneof":
		return fmt.Sprintf("must be one of: %s", fe.Param())
	case "gt":
		return fmt.Sprintf("must be greater than %s", fe.Param())
	case "gte":
		return fmt.Sprintf("must be greater than or equal to %s", fe.Param())
	case "lt":
		return fmt.Sprintf("must be less than %s", fe.Param())
	case "lte":
		return fmt.Sprintf("must be less than or equal to %s", fe.Param())
	case "url":
		return "must be a valid URL"
	default:
		return fmt.Sprintf("failed validation rule %q", fe.Tag())
	}
}

// indexOf returns the index of the first occurrence of sep in s, or -1.
func indexOf(s string, sep byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return i
		}
	}
	return -1
}
