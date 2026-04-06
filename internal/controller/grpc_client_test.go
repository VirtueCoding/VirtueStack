package controller

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func TestInsecureNodeClientReusesConnection(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := InsecureNodeClient(logger)

	ctx := context.Background()
	conn1, err := client.GetInsecureConnection(ctx, "node-1", "127.0.0.1:50051")
	require.NoError(t, err)

	conn2, err := client.GetInsecureConnection(ctx, "node-1", "127.0.0.1:50051")
	require.NoError(t, err)

	require.Same(t, conn1, conn2)
	require.Equal(t, 1, client.ConnectionCount())
}

func TestInsecureNodeClientRemoveConnectionCreatesNewConnection(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := InsecureNodeClient(logger)

	ctx := context.Background()
	conn1, err := client.GetInsecureConnection(ctx, "node-1", "127.0.0.1:50051")
	require.NoError(t, err)

	client.RemoveConnection("node-1")
	require.Equal(t, 0, client.ConnectionCount())

	conn2, err := client.GetInsecureConnection(ctx, "node-1", "127.0.0.1:50051")
	require.NoError(t, err)

	require.NotSame(t, conn1, conn2)
	require.Equal(t, 1, client.ConnectionCount())
}

func TestInsecureNodeClientConcurrentGetConnectionReturnsSinglePooledConnection(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := InsecureNodeClient(logger)

	ctx := context.Background()
	const goroutines = 8

	results := make([]any, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			conn, err := client.GetInsecureConnection(ctx, "node-1", "127.0.0.1:50051")
			results[idx] = conn
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}

	first := results[0]
	for i := 1; i < len(results); i++ {
		require.Same(t, first, results[i])
	}
	require.Equal(t, 1, client.ConnectionCount())
}

func TestNodeClientGetConnectionReusesOpenPooledConnection(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := InsecureNodeClient(logger)

	ctx := context.Background()
	conn, err := grpc.NewClient("127.0.0.1:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	client.mu.Lock()
	client.conns["node-1"] = conn
	client.mu.Unlock()

	reused, err := client.GetConnection(ctx, "node-1", "127.0.0.1:50051")
	require.NoError(t, err)
	require.Same(t, conn, reused)
	require.NotEqual(t, connectivity.Shutdown, reused.GetState())
}

func TestNodeClientGetConnectionDoubleCheckDoesNotClosePooledConnection(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := InsecureNodeClient(logger)
	triggered := make(chan struct{}, 1)
	proceed := make(chan struct{})
	client.afterReadMissHook = func() {
		triggered <- struct{}{}
		<-proceed
	}

	ctx := context.Background()
	conn, err := grpc.NewClient("127.0.0.1:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	resultCh := make(chan *grpc.ClientConn, 1)
	errCh := make(chan error, 1)
	go func() {
		reused, getErr := client.GetConnection(ctx, "node-1", "127.0.0.1:50051")
		resultCh <- reused
		errCh <- getErr
	}()

	<-triggered
	client.mu.Lock()
	client.conns["node-1"] = conn
	client.mu.Unlock()
	close(proceed)
	reused := <-resultCh
	getErr := <-errCh

	require.NoError(t, getErr)
	require.Same(t, conn, reused)
	require.NotEqual(t, connectivity.Shutdown, reused.GetState())
}
