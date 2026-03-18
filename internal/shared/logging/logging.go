// Package logging provides structured logging setup for VirtueStack using Go's slog.
// It configures JSON output by default and provides helpers for adding context fields.
package logging

import (
	"context"
	"log/slog"
	"os"
)

// contextKey is the type for context keys used by this package.
type contextKey string

const (
	// loggerKey is the context key for storing a logger.
	loggerKey contextKey = "logger"
)

// ParseLevel converts a string level to slog.Level.
// Supported levels: "debug", "info", "warn", "error".
// Defaults to slog.LevelInfo for unrecognized values.
func ParseLevel(level string) slog.Level {
	switch level {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "info", "INFO":
		return slog.LevelInfo
	case "warn", "WARN", "warning", "WARNING":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Setup configures the global slog logger with JSON output.
// The level parameter specifies the minimum log level.
// Supported levels: "debug", "info", "warn", "error".
func Setup(level string) {
	logLevel := ParseLevel(level)

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)

	slog.SetDefault(logger)
}

// NewLogger creates a new logger with JSON output at the specified level.
// This is useful when you need a separate logger instance rather than the global one.
func NewLogger(level string) *slog.Logger {
	logLevel := ParseLevel(level)

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(handler)
}

// WithCorrelation returns a logger with correlation_id attribute.
// This is used to track requests across service boundaries.
func WithCorrelation(logger *slog.Logger, correlationID string) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return logger.With(slog.String("correlation_id", correlationID))
}

// WithComponent returns a logger with component attribute.
// Common values: "node-agent", "controller", "api", "scheduler".
func WithComponent(logger *slog.Logger, component string) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return logger.With(slog.String("component", component))
}

// WithVM returns a logger with vm_id attribute.
// This is used to correlate log entries with a specific virtual machine.
func WithVM(logger *slog.Logger, vmID string) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return logger.With(slog.String("vm_id", vmID))
}

// WithNode returns a logger with node_id attribute.
// This is used to correlate log entries with a specific node.
func WithNode(logger *slog.Logger, nodeID string) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return logger.With(slog.String("node_id", nodeID))
}

// WithUser returns a logger with user_id attribute.
// This is used to correlate log entries with a specific user.
func WithUser(logger *slog.Logger, userID string) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return logger.With(slog.String("user_id", userID))
}

// WithRequest returns a logger with request_id attribute.
// This is an alias for WithCorrelation for semantic clarity in HTTP contexts.
func WithRequest(logger *slog.Logger, requestID string) *slog.Logger {
	return WithCorrelation(logger, requestID)
}

// WithError returns a logger with an error attribute.
// This is useful for logging errors alongside other context.
func WithError(logger *slog.Logger, err error) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return logger.With(slog.String("error", err.Error()))
}

// WithFields returns a logger with multiple attributes.
// This provides a convenient way to add multiple fields at once.
//
// The map[string]any parameter type is intentional: it mirrors the slog API's
// variadic any args and allows callers to pass heterogeneous log field values
// without defining a concrete struct. This is an accepted exception to the
// project's general preference for typed parameters.
func WithFields(logger *slog.Logger, fields map[string]any) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}

	attrs := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}

	return logger.With(attrs...)
}

// LoggerFromContext retrieves a logger from the context.
// If no logger is found, returns the default logger.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}

// LoggerWithContext stores a logger in the context.
// This allows propagating loggers through the call chain.
func LoggerWithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// WithContext returns a new context containing the provided logger.
// This is a convenience method for adding a logger to a context.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return LoggerWithContext(ctx, logger)
}