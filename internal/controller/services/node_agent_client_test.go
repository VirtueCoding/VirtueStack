package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type mockGRPCConnectionPool struct {
	getConnectionFunc func(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error)
	callCount         int32
}

func (m *mockGRPCConnectionPool) GetConnection(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error) {
	atomic.AddInt32(&m.callCount, 1)
	if m.getConnectionFunc != nil {
		return m.getConnectionFunc(ctx, nodeID, address)
	}
	return nil, fmt.Errorf("no mock connection configured")
}

func (m *mockGRPCConnectionPool) Calls() int32 {
	return atomic.LoadInt32(&m.callCount)
}

type testNodeAgentServer struct {
	nodeagentpb.UnimplementedNodeAgentServiceServer
	getNodeHealthFunc func(ctx context.Context, req *nodeagentpb.Empty) (*nodeagentpb.NodeHealthResponse, error)
	pingFunc          func(ctx context.Context, req *nodeagentpb.Empty) (*nodeagentpb.PingResponse, error)
	healthCalls       int32
}

func (s *testNodeAgentServer) GetNodeHealth(ctx context.Context, req *nodeagentpb.Empty) (*nodeagentpb.NodeHealthResponse, error) {
	atomic.AddInt32(&s.healthCalls, 1)
	if s.getNodeHealthFunc != nil {
		return s.getNodeHealthFunc(ctx, req)
	}
	return &nodeagentpb.NodeHealthResponse{
		CpuPercent:    40,
		MemoryPercent: 50,
		DiskPercent:   60,
		VmCount:       3,
	}, nil
}

func (s *testNodeAgentServer) Ping(ctx context.Context, req *nodeagentpb.Empty) (*nodeagentpb.PingResponse, error) {
	if s.pingFunc != nil {
		return s.pingFunc(ctx, req)
	}
	return &nodeagentpb.PingResponse{NodeId: "node-1"}, nil
}

func (s *testNodeAgentServer) HealthCalls() int32 {
	return atomic.LoadInt32(&s.healthCalls)
}

func startTestNodeAgentServer(t *testing.T, srv nodeagentpb.NodeAgentServiceServer) (*grpc.ClientConn, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	nodeagentpb.RegisterNodeAgentServiceServer(grpcServer, srv)

	go func() {
		_ = grpcServer.Serve(lis)
	}()

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	cleanup := func() {
		_ = conn.Close()
		grpcServer.Stop()
		_ = lis.Close()
	}
	return conn, cleanup
}

func TestNodeAgentGRPCClient_GetNodeMetrics(t *testing.T) {
	t.Run("grpc connection failure returns wrapped error", func(t *testing.T) {
		db := &fakeDB{
			queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
				if strings.Contains(sql, "FROM nodes WHERE id = $1") {
					return &fakeRow{values: nodeRow("node-1", models.NodeStatusOnline, 8, 16384, 2, 2048)}
				}
				return &fakeRow{scanErr: pgx.ErrNoRows}
			},
		}
		pool := &mockGRPCConnectionPool{
			getConnectionFunc: func(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error) {
				return nil, fmt.Errorf("dial failed")
			},
		}

		client := NewNodeAgentGRPCClient(
			repository.NewNodeRepository(db),
			repository.NewVMRepository(db),
			pool,
			nil,
			testVMServiceLogger(),
		)

		metrics, err := client.GetNodeMetrics(context.Background(), "node-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connecting to node node-1")
		assert.Contains(t, err.Error(), "dial failed")
		assert.Nil(t, metrics)
	})

	t.Run("grpc timeout returns context deadline exceeded", func(t *testing.T) {
		db := &fakeDB{
			queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
				if strings.Contains(sql, "FROM nodes WHERE id = $1") {
					return &fakeRow{values: nodeRow("node-1", models.NodeStatusOnline, 8, 16384, 2, 2048)}
				}
				return &fakeRow{scanErr: pgx.ErrNoRows}
			},
		}

		server := &testNodeAgentServer{
			getNodeHealthFunc: func(ctx context.Context, req *nodeagentpb.Empty) (*nodeagentpb.NodeHealthResponse, error) {
				select {
				case <-time.After(200 * time.Millisecond):
					return &nodeagentpb.NodeHealthResponse{}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		}
		conn, cleanup := startTestNodeAgentServer(t, server)
		defer cleanup()

		pool := &mockGRPCConnectionPool{
			getConnectionFunc: func(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error) {
				return conn, nil
			},
		}

		client := NewNodeAgentGRPCClient(
			repository.NewNodeRepository(db),
			repository.NewVMRepository(db),
			pool,
			nil,
			testVMServiceLogger(),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		metrics, err := client.GetNodeMetrics(ctx, "node-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context deadline exceeded")
		assert.Nil(t, metrics)
	})

	t.Run("metrics cache miss fetches and caches", func(t *testing.T) {
		db := &fakeDB{
			queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
				if strings.Contains(sql, "FROM nodes WHERE id = $1") {
					return &fakeRow{values: nodeRow("node-1", models.NodeStatusOnline, 8, 16384, 2, 2048)}
				}
				return &fakeRow{scanErr: pgx.ErrNoRows}
			},
		}

		server := &testNodeAgentServer{}
		conn, cleanup := startTestNodeAgentServer(t, server)
		defer cleanup()

		pool := &mockGRPCConnectionPool{
			getConnectionFunc: func(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error) {
				return conn, nil
			},
		}

		client := NewNodeAgentGRPCClient(
			repository.NewNodeRepository(db),
			repository.NewVMRepository(db),
			pool,
			nil,
			testVMServiceLogger(),
		)

		metrics, err := client.GetNodeMetrics(context.Background(), "node-1")
		require.NoError(t, err)
		require.NotNil(t, metrics)
		assert.Equal(t, float32(40), metrics.CPUPercent)
		assert.Equal(t, int32(1), pool.Calls())
		assert.Equal(t, int32(1), server.HealthCalls())
	})

	t.Run("metrics cache hit returns cached data without grpc call", func(t *testing.T) {
		db := &fakeDB{
			queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
				return &fakeRow{scanErr: pgx.ErrNoRows}
			},
		}
		pool := &mockGRPCConnectionPool{
			getConnectionFunc: func(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error) {
				return nil, fmt.Errorf("should not be called")
			},
		}

		client := NewNodeAgentGRPCClient(
			repository.NewNodeRepository(db),
			repository.NewVMRepository(db),
			pool,
			nil,
			testVMServiceLogger(),
		)

		expected := &models.NodeHeartbeat{CPUPercent: 77}
		client.cache.set("node-1", expected)

		metrics, err := client.GetNodeMetrics(context.Background(), "node-1")
		require.NoError(t, err)
		require.NotNil(t, metrics)
		assert.Equal(t, float32(77), metrics.CPUPercent)
		assert.Equal(t, int32(0), pool.Calls())
	})

	t.Run("node not found returns not found error", func(t *testing.T) {
		db := &fakeDB{
			queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
				if strings.Contains(sql, "FROM nodes WHERE id = $1") {
					return &fakeRow{scanErr: pgx.ErrNoRows}
				}
				return &fakeRow{scanErr: pgx.ErrNoRows}
			},
		}
		pool := &mockGRPCConnectionPool{
			getConnectionFunc: func(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error) {
				return nil, fmt.Errorf("unexpected call")
			},
		}

		client := NewNodeAgentGRPCClient(
			repository.NewNodeRepository(db),
			repository.NewVMRepository(db),
			pool,
			nil,
			testVMServiceLogger(),
		)

		metrics, err := client.GetNodeMetrics(context.Background(), "missing-node")
		require.Error(t, err)
		assert.True(t, errors.Is(err, sharederrors.ErrNotFound))
		assert.Contains(t, err.Error(), "getting node missing-node")
		assert.Nil(t, metrics)
		assert.Equal(t, int32(0), pool.Calls())
	})
}
