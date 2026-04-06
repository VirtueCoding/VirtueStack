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
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type consoleTestStream[Req any, Res any] struct {
	ctx      context.Context
	recvMsgs []*Req
}

func (s *consoleTestStream[Req, Res]) Recv() (*Req, error) {
	if len(s.recvMsgs) == 0 {
		return nil, io.EOF
	}

	msg := s.recvMsgs[0]
	s.recvMsgs = s.recvMsgs[1:]
	return msg, nil
}

func (s *consoleTestStream[Req, Res]) Send(*Res) error {
	return nil
}

func (s *consoleTestStream[Req, Res]) SetHeader(metadata.MD) error {
	return nil
}

func (s *consoleTestStream[Req, Res]) SendHeader(metadata.MD) error {
	return nil
}

func (s *consoleTestStream[Req, Res]) SetTrailer(metadata.MD) {}

func (s *consoleTestStream[Req, Res]) Context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *consoleTestStream[Req, Res]) SendMsg(any) error {
	return nil
}

func (s *consoleTestStream[Req, Res]) RecvMsg(any) error {
	return io.EOF
}

func TestConsoleRPCsRequireGuestOpToken(t *testing.T) {
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
	const vmID = "550e8400-e29b-41d4-a716-446655440000"

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "vnc console stream",
			call: func() error {
				return handler.StreamVNCConsole(&consoleTestStream[nodeagentpb.VNCFrame, nodeagentpb.VNCFrame]{
					ctx: ctx,
					recvMsgs: []*nodeagentpb.VNCFrame{
						{Data: []byte(vmID)},
					},
				})
			},
		},
		{
			name: "serial console stream",
			call: func() error {
				return handler.StreamSerialConsole(&consoleTestStream[nodeagentpb.SerialData, nodeagentpb.SerialData]{
					ctx: ctx,
					recvMsgs: []*nodeagentpb.SerialData{
						{Data: []byte(vmID)},
					},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var err error
			require.NotPanics(t, func() {
				err = tt.call()
			})
			require.Error(t, err)

			st, ok := status.FromError(err)
			require.True(t, ok)
			assert.Equal(t, "missing x-guest-op-token metadata", st.Message())
		})
	}
}
