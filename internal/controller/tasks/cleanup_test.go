package tasks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompensationStackRollback(t *testing.T) {
	tests := []struct {
		name             string
		setupStack       func(*CompensationStack, *[]string, *int)
		wantOrder        []string
		wantErrorLogHits int
	}{
		{
			name: "rollback executes LIFO order",
			setupStack: func(cs *CompensationStack, calls *[]string, errorHits *int) {
				cs.Push("step-1", func(ctx context.Context) error {
					*calls = append(*calls, "step-1")
					return nil
				})
				cs.Push("step-2", func(ctx context.Context) error {
					*calls = append(*calls, "step-2")
					return nil
				})
				cs.Push("step-3", func(ctx context.Context) error {
					*calls = append(*calls, "step-3")
					return nil
				})
			},
			wantOrder: []string{"step-3", "step-2", "step-1"},
		},
		{
			name: "rollback continues when cleanup errors occur",
			setupStack: func(cs *CompensationStack, calls *[]string, errorHits *int) {
				cs.Push("first", func(ctx context.Context) error {
					*calls = append(*calls, "first")
					return nil
				})
				cs.Push("failing", func(ctx context.Context) error {
					*calls = append(*calls, "failing")
					*errorHits = *errorHits + 1
					return errors.New("cleanup failed")
				})
				cs.Push("last", func(ctx context.Context) error {
					*calls = append(*calls, "last")
					return nil
				})
			},
			wantOrder:        []string{"last", "failing", "first"},
			wantErrorLogHits: 1,
		},
		{
			name: "rollback on empty stack is no-op",
			setupStack: func(cs *CompensationStack, calls *[]string, errorHits *int) {
			},
			wantOrder: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := make([]string, 0)
			errorHits := 0
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			stack := NewCompensationStack(logger)
			tt.setupStack(stack, &calls, &errorHits)

			stack.Rollback(context.Background())

			require.True(t, slices.Equal(tt.wantOrder, calls), "unexpected rollback order")
			assert.Equal(t, tt.wantErrorLogHits, errorHits)
		})
	}
}
