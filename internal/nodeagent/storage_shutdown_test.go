package nodeagent

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

type storageShutdownTestCloser struct {
	closeCount int
	closeErr   error
}

func (c *storageShutdownTestCloser) Close() error {
	c.closeCount++
	return c.closeErr
}

type storageShutdownTestNoErrCloser struct {
	closeCount int
}

func (c *storageShutdownTestNoErrCloser) Close() {
	c.closeCount++
}

func TestCloseManagedComponents(t *testing.T) {
	tests := []struct {
		name               string
		errorCloserErr     error
		includeNonCloser   bool
		wantErrorCloseCount int
		wantPlainCloseCount int
		wantLogFragment    string
	}{
		{
			name:                "closes all supported closer shapes and skips non-closers",
			includeNonCloser:    true,
			wantErrorCloseCount: 1,
			wantPlainCloseCount: 1,
		},
		{
			name:                "continues closing remaining components after a close error",
			errorCloserErr:      errors.New("template close failed"),
			wantErrorCloseCount: 1,
			wantPlainCloseCount: 1,
			wantLogFragment:     "template close failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errCloser := &storageShutdownTestCloser{closeErr: tt.errorCloserErr}
			plainCloser := &storageShutdownTestNoErrCloser{}
			components := []any{errCloser, plainCloser}
			if tt.includeNonCloser {
				components = append(components, struct{}{}, nil)
			}

			var logBuffer bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logBuffer, nil))

			closeManagedComponents(logger, "error closing storage resources", components...)

			assert.Equal(t, tt.wantErrorCloseCount, errCloser.closeCount)
			assert.Equal(t, tt.wantPlainCloseCount, plainCloser.closeCount)
			if tt.wantLogFragment == "" {
				assert.NotContains(t, logBuffer.String(), "error closing storage resources")
				return
			}

			assert.Contains(t, logBuffer.String(), tt.wantLogFragment)
		})
	}
}
