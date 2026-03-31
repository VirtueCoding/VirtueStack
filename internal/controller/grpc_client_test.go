package controller

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
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
