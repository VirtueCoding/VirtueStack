// Package logging provides structured logging setup for VirtueStack using Go's slog.
// It configures JSON output by default and provides helpers for adding context fields.
package logging

import (
	"log/slog"
	"os"
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

// WithComponent returns a logger with component attribute.
// Common values: "node-agent", "controller", "api", "scheduler".
func WithComponent(logger *slog.Logger, component string) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return logger.With(slog.String("component", component))
}