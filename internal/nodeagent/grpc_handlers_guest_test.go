package nodeagent

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestVerifyGuestOpTokenRejectsMissingSecret(t *testing.T) {
	tests := []struct {
		name     string
		secret   config.Secret
		wantCode codes.Code
	}{
		{
			name:     "empty guest operation secret fails closed",
			secret:   "",
			wantCode: codes.FailedPrecondition,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &grpcHandler{
				server: &Server{
					config: &config.NodeAgentConfig{
						GuestOpHMACSecret: tt.secret,
					},
					logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				},
			}

			err := handler.verifyGuestOpToken(context.Background(), "vm-123")

			require.Error(t, err)
			require.Equal(t, tt.wantCode, status.Code(err))
		})
	}
}
