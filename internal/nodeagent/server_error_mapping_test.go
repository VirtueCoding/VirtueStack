package nodeagent

import (
	"context"
	"fmt"
	"testing"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGRPCStatusForNodeErrorMapsTypedErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode codes.Code
	}{
		{
			name:     "wrapped not found maps to not found",
			err:      fmt.Errorf("lookup vm: %w", sharederrors.ErrNotFound),
			wantCode: codes.NotFound,
		},
		{
			name:     "wrapped timeout maps to deadline exceeded",
			err:      fmt.Errorf("shutdown vm: %w", sharederrors.ErrTimeout),
			wantCode: codes.DeadlineExceeded,
		},
		{
			name:     "wrapped timeout-like error maps to deadline exceeded",
			err:      fmt.Errorf("download template: %w", timeoutLikeTestError{}),
			wantCode: codes.DeadlineExceeded,
		},
		{
			name:     "context cancellation maps to canceled",
			err:      context.Canceled,
			wantCode: codes.Canceled,
		},
		{
			name:     "unknown errors remain internal",
			err:      fmt.Errorf("boom"),
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := grpcStatusForNodeError("test operation", tt.err)
			require.Error(t, err)

			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, st.Code())
		})
	}
}

type timeoutLikeTestError struct{}

func (timeoutLikeTestError) Error() string {
	return "timeout"
}

func (timeoutLikeTestError) Timeout() bool {
	return true
}
