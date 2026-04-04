package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
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
	getNodeHealthFunc          func(ctx context.Context, req *nodeagentpb.Empty) (*nodeagentpb.NodeHealthResponse, error)
	pingFunc                   func(ctx context.Context, req *nodeagentpb.Empty) (*nodeagentpb.PingResponse, error)
	guestFreezeFilesystemsFunc func(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error)
	guestThawFilesystemsFunc   func(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error)
	migrateVMFunc              func(ctx context.Context, req *nodeagentpb.MigrateVMRequest) (*nodeagentpb.MigrateVMResponse, error)
	transferDiskFunc           func(req *nodeagentpb.TransferDiskRequest, stream nodeagentpb.NodeAgentService_TransferDiskServer) error
	receiveDiskFunc            func(stream nodeagentpb.NodeAgentService_ReceiveDiskServer) error
	healthCalls                int32
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

func (s *testNodeAgentServer) GuestFreezeFilesystems(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if s.guestFreezeFilesystemsFunc != nil {
		return s.guestFreezeFilesystemsFunc(ctx, req)
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

func (s *testNodeAgentServer) GuestThawFilesystems(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if s.guestThawFilesystemsFunc != nil {
		return s.guestThawFilesystemsFunc(ctx, req)
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

func (s *testNodeAgentServer) MigrateVM(ctx context.Context, req *nodeagentpb.MigrateVMRequest) (*nodeagentpb.MigrateVMResponse, error) {
	if s.migrateVMFunc != nil {
		return s.migrateVMFunc(ctx, req)
	}

	return &nodeagentpb.MigrateVMResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

func (s *testNodeAgentServer) TransferDisk(req *nodeagentpb.TransferDiskRequest, stream nodeagentpb.NodeAgentService_TransferDiskServer) error {
	if s.transferDiskFunc != nil {
		return s.transferDiskFunc(req, stream)
	}

	return nil
}

func (s *testNodeAgentServer) ReceiveDisk(stream nodeagentpb.NodeAgentService_ReceiveDiskServer) error {
	if s.receiveDiskFunc != nil {
		return s.receiveDiskFunc(stream)
	}

	for {
		_, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&nodeagentpb.ReceiveDiskResponse{Success: true})
		}
		if err != nil {
			return err
		}
	}
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
			"",
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
			"",
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
			"",
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
			"",
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
			"",
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

func TestNodeAgentGRPCClient_GuestFilesystemOpsAttachGuestOpToken(t *testing.T) {
	t.Parallel()

	const secret = "0123456789abcdef0123456789abcdef"
	const nodeID = "node-1"
	const vmID = "vm-123"

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM nodes WHERE id = $1") {
				return &fakeRow{values: nodeRow(nodeID, models.NodeStatusOnline, 8, 16384, 2, 2048)}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
	}

	validateGuestToken := func(ctx context.Context) error {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return fmt.Errorf("missing incoming metadata")
		}

		vals := md.Get("x-guest-op-token")
		if len(vals) != 1 {
			return fmt.Errorf("expected one guest op token, got %d", len(vals))
		}

		parts := strings.SplitN(vals[0], ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("malformed guest op token: %q", vals[0])
		}

		message := fmt.Sprintf("%s:%s", vmID, parts[0])
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(message))
		expected := hex.EncodeToString(mac.Sum(nil))
		if parts[1] != expected {
			return fmt.Errorf("unexpected guest op token signature")
		}

		return nil
	}

	server := &testNodeAgentServer{
		guestFreezeFilesystemsFunc: func(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
			if err := validateGuestToken(ctx); err != nil {
				return nil, err
			}

			return &nodeagentpb.VMOperationResponse{
				VmId:    req.GetVmId(),
				Success: true,
			}, nil
		},
		guestThawFilesystemsFunc: func(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
			if err := validateGuestToken(ctx); err != nil {
				return nil, err
			}

			return &nodeagentpb.VMOperationResponse{
				VmId:    req.GetVmId(),
				Success: true,
			}, nil
		},
	}

	conn, cleanup := startTestNodeAgentServer(t, server)
	defer cleanup()

	pool := &mockGRPCConnectionPool{
		getConnectionFunc: func(ctx context.Context, gotNodeID, address string) (*grpc.ClientConn, error) {
			assert.Equal(t, nodeID, gotNodeID)
			return conn, nil
		},
	}

	client := NewNodeAgentGRPCClient(
		repository.NewNodeRepository(db),
		repository.NewVMRepository(db),
		pool,
		nil,
		secret,
		testVMServiceLogger(),
	)

	ctx := context.Background()

	frozenCount, err := client.GuestFreezeFilesystems(ctx, nodeID, vmID)
	require.NoError(t, err)
	assert.Equal(t, 0, frozenCount)

	thawedCount, err := client.GuestThawFilesystems(ctx, nodeID, vmID)
	require.NoError(t, err)
	assert.Equal(t, 0, thawedCount)
}

func TestNodeAgentGRPCClient_GuestFilesystemOpsRequireGuestOpSecret(t *testing.T) {
	t.Parallel()

	const nodeID = "node-1"
	const vmID = "vm-123"

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM nodes WHERE id = $1") {
				return &fakeRow{values: nodeRow(nodeID, models.NodeStatusOnline, 8, 16384, 2, 2048)}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
	}

	server := &testNodeAgentServer{
		guestFreezeFilesystemsFunc: func(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
			md, ok := metadata.FromIncomingContext(ctx)
			if !ok {
				return nil, fmt.Errorf("missing incoming metadata")
			}
			if vals := md.Get("x-guest-op-token"); len(vals) != 1 {
				return nil, fmt.Errorf("expected one guest op token, got %d", len(vals))
			}
			return &nodeagentpb.VMOperationResponse{VmId: req.GetVmId(), Success: true}, nil
		},
	}

	conn, cleanup := startTestNodeAgentServer(t, server)
	defer cleanup()

	pool := &mockGRPCConnectionPool{
		getConnectionFunc: func(ctx context.Context, gotNodeID, address string) (*grpc.ClientConn, error) {
			assert.Equal(t, nodeID, gotNodeID)
			return conn, nil
		},
	}

	client := NewNodeAgentGRPCClient(
		repository.NewNodeRepository(db),
		repository.NewVMRepository(db),
		pool,
		nil,
		"",
		testVMServiceLogger(),
	)

	_, err := client.GuestFreezeFilesystems(context.Background(), nodeID, vmID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "guest operation HMAC secret is required")
}

func TestNodeAgentGRPCClient_MigrateVMAlwaysUsesLiveFlag(t *testing.T) {
	t.Parallel()

	const sourceNodeID = "source-node"
	const targetNodeID = "target-node"
	const vmID = "vm-123"

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM nodes WHERE id = $1") {
				switch args[0] {
				case sourceNodeID:
					row := nodeRow(sourceNodeID, models.NodeStatusOnline, 8, 16384, 2, 2048)
					row[2] = "source-node:8443"
					return &fakeRow{values: row}
				case targetNodeID:
					row := nodeRow(targetNodeID, models.NodeStatusOnline, 8, 16384, 2, 2048)
					row[2] = "target-node:8443"
					return &fakeRow{values: row}
				}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
	}

	tests := []struct {
		name string
		opts *tasks.MigrateVMOptions
	}{
		{
			name: "nil options still uses live migration",
			opts: nil,
		},
		{
			name: "target address option does not affect live flag",
			opts: &tasks.MigrateVMOptions{
				TargetNodeAddress: "10.0.0.2:8443",
			},
		},
		{
			name: "empty target address does not force cold request",
			opts: &tasks.MigrateVMOptions{
				TargetNodeAddress: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &testNodeAgentServer{
				migrateVMFunc: func(ctx context.Context, req *nodeagentpb.MigrateVMRequest) (*nodeagentpb.MigrateVMResponse, error) {
					assert.Equal(t, vmID, req.GetVmId())
					assert.Equal(t, "target-node:8443", req.GetDestinationNodeAddress())
					assert.True(t, req.GetLive())
					return &nodeagentpb.MigrateVMResponse{
						VmId:    req.GetVmId(),
						Success: true,
					}, nil
				},
			}
			conn, cleanup := startTestNodeAgentServer(t, server)
			defer cleanup()

			pool := &mockGRPCConnectionPool{
				getConnectionFunc: func(ctx context.Context, gotNodeID, address string) (*grpc.ClientConn, error) {
					assert.Equal(t, sourceNodeID, gotNodeID)
					return conn, nil
				},
			}

			client := NewNodeAgentGRPCClient(
				repository.NewNodeRepository(db),
				repository.NewVMRepository(db),
				pool,
				nil,
				"",
				testVMServiceLogger(),
			)

			err := client.MigrateVM(context.Background(), sourceNodeID, targetNodeID, vmID, tt.opts)
			require.NoError(t, err)
		})
	}
}

func TestNodeAgentGRPCClient_TransferDiskRelaysToReceiveDisk(t *testing.T) {
	t.Parallel()

	const sourceNodeID = "source-node"
	const targetNodeID = "target-node"

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM nodes WHERE id = $1") {
				switch args[0] {
				case sourceNodeID:
					row := nodeRow(sourceNodeID, models.NodeStatusOnline, 8, 16384, 2, 2048)
					row[2] = "source-node:8443"
					return &fakeRow{values: row}
				case targetNodeID:
					row := nodeRow(targetNodeID, models.NodeStatusOnline, 8, 16384, 2, 2048)
					row[2] = "target-node:8443"
					return &fakeRow{values: row}
				}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
	}

	expectedChunks := []*nodeagentpb.DiskChunk{
		{
			TargetDiskPath:       "/target/disk.qcow2",
			StorageBackend:       "lvm",
			DiskSizeGb:           50,
			TargetLvmVolumeGroup: "vg-target",
			TargetLvmThinPool:    "thin-target",
			Total:                11,
		},
		{
			Data:   []byte("hello "),
			Offset: 0,
			Total:  11,
		},
		{
			Data:   []byte("world"),
			Offset: 6,
			Total:  11,
		},
	}

	var receivedChunks []*nodeagentpb.DiskChunk

	server := &testNodeAgentServer{
		transferDiskFunc: func(req *nodeagentpb.TransferDiskRequest, stream nodeagentpb.NodeAgentService_TransferDiskServer) error {
			assert.Equal(t, "/source/disk.qcow2", req.GetSourceDiskPath())
			assert.Equal(t, "target-node:8443", req.GetTargetNodeAddress())
			assert.Equal(t, "/target/disk.qcow2", req.GetTargetDiskPath())
			assert.Equal(t, "snap-1", req.GetSnapshotName())
			assert.True(t, req.GetCompress())
			assert.Equal(t, "lvm", req.GetStorageBackend())
			assert.Equal(t, int32(50), req.GetDiskSizeGb())
			assert.Equal(t, "vg-source", req.GetSourceLvmVolumeGroup())
			assert.Equal(t, "thin-source", req.GetSourceLvmThinPool())
			assert.Equal(t, "vg-target", req.GetTargetLvmVolumeGroup())
			assert.Equal(t, "thin-target", req.GetTargetLvmThinPool())
			assert.True(t, req.GetIsDeltaSync())

			for _, chunk := range expectedChunks {
				if err := stream.Send(chunk); err != nil {
					return err
				}
			}

			return nil
		},
		receiveDiskFunc: func(stream nodeagentpb.NodeAgentService_ReceiveDiskServer) error {
			for {
				chunk, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return stream.SendAndClose(&nodeagentpb.ReceiveDiskResponse{
						TargetDiskPath: "/target/disk.qcow2",
						BytesReceived:  11,
						Success:        true,
					})
				}
				if err != nil {
					return err
				}

				receivedChunks = append(receivedChunks, chunk)
			}
		},
	}
	conn, cleanup := startTestNodeAgentServer(t, server)
	defer cleanup()

	pool := &mockGRPCConnectionPool{
		getConnectionFunc: func(ctx context.Context, gotNodeID, address string) (*grpc.ClientConn, error) {
			switch gotNodeID {
			case sourceNodeID:
				assert.Equal(t, "source-node:8443", address)
			case targetNodeID:
				assert.Equal(t, "target-node:8443", address)
			default:
				t.Fatalf("unexpected nodeID %q", gotNodeID)
			}
			return conn, nil
		},
	}

	client := NewNodeAgentGRPCClient(
		repository.NewNodeRepository(db),
		repository.NewVMRepository(db),
		pool,
		nil,
		"",
		testVMServiceLogger(),
	)

	err := client.TransferDisk(context.Background(), &tasks.DiskTransferOptions{
		SourceNodeID:         sourceNodeID,
		TargetNodeID:         targetNodeID,
		SourceDiskPath:       "/source/disk.qcow2",
		TargetDiskPath:       "/target/disk.qcow2",
		SnapshotName:         "snap-1",
		DiskSizeGB:           50,
		SourceStorageBackend: "lvm",
		TargetStorageBackend: "lvm",
		SourceLVMVolumeGroup: "vg-source",
		SourceLVMThinPool:    "thin-source",
		TargetLVMVolumeGroup: "vg-target",
		TargetLVMThinPool:    "thin-target",
		Compress:             true,
		IsDeltaSync:          true,
	})
	require.NoError(t, err)
	require.Len(t, receivedChunks, len(expectedChunks))

	for i, chunk := range receivedChunks {
		assert.Equal(t, expectedChunks[i].GetTargetDiskPath(), chunk.GetTargetDiskPath())
		assert.Equal(t, expectedChunks[i].GetStorageBackend(), chunk.GetStorageBackend())
		assert.Equal(t, expectedChunks[i].GetDiskSizeGb(), chunk.GetDiskSizeGb())
		assert.Equal(t, expectedChunks[i].GetTargetLvmVolumeGroup(), chunk.GetTargetLvmVolumeGroup())
		assert.Equal(t, expectedChunks[i].GetTargetLvmThinPool(), chunk.GetTargetLvmThinPool())
		assert.Equal(t, expectedChunks[i].GetOffset(), chunk.GetOffset())
		assert.Equal(t, expectedChunks[i].GetTotal(), chunk.GetTotal())
		assert.Equal(t, expectedChunks[i].GetData(), chunk.GetData())
	}
	assert.Equal(t, int32(2), pool.Calls())
}

func TestNodeAgentGRPCClient_TransferDiskClosesSourceStreamWhenForwardingFails(t *testing.T) {
	t.Parallel()

	const sourceNodeID = "source-node"
	const targetNodeID = "target-node"

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM nodes WHERE id = $1") {
				switch args[0] {
				case sourceNodeID:
					row := nodeRow(sourceNodeID, models.NodeStatusOnline, 8, 16384, 2, 2048)
					row[2] = "source-node:8443"
					return &fakeRow{values: row}
				case targetNodeID:
					row := nodeRow(targetNodeID, models.NodeStatusOnline, 8, 16384, 2, 2048)
					row[2] = "target-node:8443"
					return &fakeRow{values: row}
				}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
	}

	firstChunkReceived := make(chan struct{})
	sourceExited := make(chan struct{})

	sourceServer := &testNodeAgentServer{
		transferDiskFunc: func(req *nodeagentpb.TransferDiskRequest, stream nodeagentpb.NodeAgentService_TransferDiskServer) error {
			defer close(sourceExited)

			if err := stream.Send(&nodeagentpb.DiskChunk{
				TargetDiskPath: "/target/disk.qcow2",
				Total:          2,
			}); err != nil {
				return err
			}

			// Wait until the target has consumed the first relayed chunk so the
			// source stream is definitely live before triggering the downstream
			// failure path we want to verify.
			<-firstChunkReceived

			if err := stream.Send(&nodeagentpb.DiskChunk{
				Data:   []byte("ok"),
				Offset: 0,
				Total:  2,
			}); err != nil {
				return err
			}

			<-stream.Context().Done()
			return stream.Context().Err()
		},
	}
	sourceConn, sourceCleanup := startTestNodeAgentServer(t, sourceServer)
	defer sourceCleanup()

	targetServer := &testNodeAgentServer{
		receiveDiskFunc: func(stream nodeagentpb.NodeAgentService_ReceiveDiskServer) error {
			firstChunk, err := stream.Recv()
			if err != nil {
				return err
			}
			if firstChunk.GetTargetDiskPath() != "/target/disk.qcow2" {
				return status.Errorf(codes.InvalidArgument, "unexpected target path %q", firstChunk.GetTargetDiskPath())
			}

			close(firstChunkReceived)
			return status.Error(codes.Internal, "receive failed")
		},
	}
	targetConn, targetCleanup := startTestNodeAgentServer(t, targetServer)
	defer targetCleanup()

	pool := &mockGRPCConnectionPool{
		getConnectionFunc: func(ctx context.Context, gotNodeID, address string) (*grpc.ClientConn, error) {
			switch gotNodeID {
			case sourceNodeID:
				assert.Equal(t, "source-node:8443", address)
				return sourceConn, nil
			case targetNodeID:
				assert.Equal(t, "target-node:8443", address)
				return targetConn, nil
			default:
				t.Fatalf("unexpected nodeID %q", gotNodeID)
			}
			return nil, fmt.Errorf("unexpected nodeID %q", gotNodeID)
		},
	}

	client := NewNodeAgentGRPCClient(
		repository.NewNodeRepository(db),
		repository.NewVMRepository(db),
		pool,
		nil,
		"",
		testVMServiceLogger(),
	)

	err := client.TransferDisk(context.Background(), &tasks.DiskTransferOptions{
		SourceNodeID:   sourceNodeID,
		TargetNodeID:   targetNodeID,
		SourceDiskPath: "/source/disk.qcow2",
		TargetDiskPath: "/target/disk.qcow2",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "receive failed")

	select {
	case <-sourceExited:
	case <-time.After(time.Second):
		t.Fatal("source transfer stream did not close after forwarding failure")
	}
}

func TestNodeAgentGRPCClient_TransferDiskReturnsPromptlyWhenReceiveFailsAfterChunksAccepted(t *testing.T) {
	t.Parallel()

	const sourceNodeID = "source-node"
	const targetNodeID = "target-node"

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM nodes WHERE id = $1") {
				switch args[0] {
				case sourceNodeID:
					row := nodeRow(sourceNodeID, models.NodeStatusOnline, 8, 16384, 2, 2048)
					row[2] = "source-node:8443"
					return &fakeRow{values: row}
				case targetNodeID:
					row := nodeRow(targetNodeID, models.NodeStatusOnline, 8, 16384, 2, 2048)
					row[2] = "target-node:8443"
					return &fakeRow{values: row}
				}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
	}

	targetFailed := make(chan struct{})
	sourceExited := make(chan struct{})

	sourceServer := &testNodeAgentServer{
		transferDiskFunc: func(req *nodeagentpb.TransferDiskRequest, stream nodeagentpb.NodeAgentService_TransferDiskServer) error {
			defer close(sourceExited)

			if err := stream.Send(&nodeagentpb.DiskChunk{
				TargetDiskPath: "/target/disk.qcow2",
				Total:          2,
			}); err != nil {
				return err
			}
			if err := stream.Send(&nodeagentpb.DiskChunk{
				Data:   []byte("ok"),
				Offset: 0,
				Total:  2,
			}); err != nil {
				return err
			}

			<-stream.Context().Done()
			return stream.Context().Err()
		},
	}
	sourceConn, sourceCleanup := startTestNodeAgentServer(t, sourceServer)
	defer sourceCleanup()

	targetServer := &testNodeAgentServer{
		receiveDiskFunc: func(stream nodeagentpb.NodeAgentService_ReceiveDiskServer) error {
			if _, err := stream.Recv(); err != nil {
				return err
			}
			if _, err := stream.Recv(); err != nil {
				return err
			}

			close(targetFailed)
			return status.Error(codes.Internal, "receive failed after chunks accepted")
		},
	}
	targetConn, targetCleanup := startTestNodeAgentServer(t, targetServer)
	defer targetCleanup()

	pool := &mockGRPCConnectionPool{
		getConnectionFunc: func(ctx context.Context, gotNodeID, address string) (*grpc.ClientConn, error) {
			switch gotNodeID {
			case sourceNodeID:
				assert.Equal(t, "source-node:8443", address)
				return sourceConn, nil
			case targetNodeID:
				assert.Equal(t, "target-node:8443", address)
				return targetConn, nil
			default:
				t.Fatalf("unexpected nodeID %q", gotNodeID)
			}
			return nil, fmt.Errorf("unexpected nodeID %q", gotNodeID)
		},
	}

	client := NewNodeAgentGRPCClient(
		repository.NewNodeRepository(db),
		repository.NewVMRepository(db),
		pool,
		nil,
		"",
		testVMServiceLogger(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.TransferDisk(ctx, &tasks.DiskTransferOptions{
			SourceNodeID:   sourceNodeID,
			TargetNodeID:   targetNodeID,
			SourceDiskPath: "/source/disk.qcow2",
			TargetDiskPath: "/target/disk.qcow2",
		})
	}()

	select {
	case <-targetFailed:
	case <-time.After(time.Second):
		t.Fatal("target receive did not fail after accepting chunks")
	}

	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.NotContains(t, err.Error(), context.DeadlineExceeded.Error())
	case <-time.After(200 * time.Millisecond):
		t.Fatal("transfer did not return promptly after target receive failure")
	}

	select {
	case <-sourceExited:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("source transfer stream was not canceled after target receive failure")
	}
}
