package nodeagent

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGuestRPCsRequireGuestOpToken(t *testing.T) {
	t.Parallel()

	handler := &grpcHandler{
		server: &Server{
			config: &config.NodeAgentConfig{
				GuestOpHMACSecret: config.Secret("0123456789abcdef0123456789abcdef"),
			},
			logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
	}

	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{})

	tests := []struct {
		name string
		call func(context.Context) error
	}{
		{
			name: "guest exec command",
			call: func(ctx context.Context) error {
				_, err := handler.GuestExecCommand(ctx, &nodeagentpb.GuestExecRequest{
					VmId:    "vm-123",
					Command: "/bin/df",
				})
				return err
			},
		},
		{
			name: "guest set password",
			call: func(ctx context.Context) error {
				_, err := handler.GuestSetPassword(ctx, &nodeagentpb.GuestPasswordRequest{
					VmId:         "vm-123",
					Username:     "root",
					PasswordHash: "hash",
				})
				return err
			},
		},
		{
			name: "guest freeze filesystems",
			call: func(ctx context.Context) error {
				_, err := handler.GuestFreezeFilesystems(ctx, &nodeagentpb.VMIdentifier{VmId: "vm-123"})
				return err
			},
		},
		{
			name: "guest thaw filesystems",
			call: func(ctx context.Context) error {
				_, err := handler.GuestThawFilesystems(ctx, &nodeagentpb.VMIdentifier{VmId: "vm-123"})
				return err
			},
		},
		{
			name: "guest get network interfaces",
			call: func(ctx context.Context) error {
				_, err := handler.GuestGetNetworkInterfaces(ctx, &nodeagentpb.VMIdentifier{VmId: "vm-123"})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.call(ctx)
			require.Error(t, err)

			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, codes.Unauthenticated, st.Code())
			assert.Equal(t, "missing x-guest-op-token metadata", st.Message())
		})
	}
}

func TestVerifyGuestOpToken_FailsClosedWhenSecretMissing(t *testing.T) {
	t.Parallel()

	handler := &grpcHandler{
		server: &Server{
			config: &config.NodeAgentConfig{},
			logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
	}

	err := handler.verifyGuestOpToken(context.Background(), "vm-123")
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.FailedPrecondition, st.Code())
	assert.Equal(t, "guest operation HMAC secret is not configured", st.Message())
}
