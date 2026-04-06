package snapshotpolicy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAllowsRevertRequiresStoppedVM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{
			name:   "allows stopped vm",
			status: "stopped",
			want:   true,
		},
		{
			name:   "rejects running vm",
			status: "running",
			want:   false,
		},
		{
			name:   "rejects paused vm",
			status: "paused",
			want:   false,
		},
		{
			name:   "rejects shutting down vm",
			status: "shutting_down",
			want:   false,
		},
		{
			name:   "rejects crashed vm",
			status: "crashed",
			want:   false,
		},
		{
			name:   "rejects unknown vm state",
			status: "unknown",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, AllowsRevert(tt.status))
		})
	}
}

func TestRevertSnapshot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		vmID              string
		vmStatus          string
		rollbackErr       error
		wantRollbackCalls int
		wantGRPCCode      codes.Code
		wantErrContains   string
		wantSuccess       bool
	}{
		{
			name:              "returns failed precondition for running vm",
			vmID:              "vm-running",
			vmStatus:          "running",
			wantRollbackCalls: 0,
			wantGRPCCode:      codes.FailedPrecondition,
			wantErrContains:   "VM must be stopped before reverting snapshot",
		},
		{
			name:              "returns failed precondition for paused vm",
			vmID:              "vm-paused",
			vmStatus:          "paused",
			wantRollbackCalls: 0,
			wantGRPCCode:      codes.FailedPrecondition,
			wantErrContains:   "VM must be stopped before reverting snapshot",
		},
		{
			name:              "calls rollback and returns success for stopped vm",
			vmID:              "vm-stopped",
			vmStatus:          "stopped",
			wantRollbackCalls: 1,
			wantSuccess:       true,
		},
		{
			name:              "propagates rollback errors after stopped check passes",
			vmID:              "vm-rollback-error",
			vmStatus:          "stopped",
			rollbackErr:       errors.New("rollback failed"),
			wantRollbackCalls: 1,
			wantErrContains:   "rollback failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rollbackCalls := 0
			resp, err := RevertSnapshot(tt.vmID, tt.vmStatus, func() error {
				rollbackCalls++
				return tt.rollbackErr
			})

			assert.Equal(t, tt.wantRollbackCalls, rollbackCalls)

			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)

				if tt.wantGRPCCode != codes.OK {
					st, ok := status.FromError(err)
					require.True(t, ok)
					assert.Equal(t, tt.wantGRPCCode, st.Code())
				}

				assert.Nil(t, resp)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tt.vmID, resp.GetVmId())
			assert.Equal(t, tt.wantSuccess, resp.GetSuccess())
		})
	}
}
