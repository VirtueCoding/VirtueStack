package repository

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type isoUploadTestState struct {
	mu    sync.Mutex
	count int
}

type mockISOUploadDB struct {
	state *isoUploadTestState
}

func (m *mockISOUploadDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *mockISOUploadDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return mockISORow{err: errors.New("unexpected QueryRow on DB")}
}

func (m *mockISOUploadDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (m *mockISOUploadDB) Begin(context.Context) (pgx.Tx, error) {
	return &mockISOUploadTx{state: m.state}, nil
}

type mockISOUploadTx struct {
	state  *isoUploadTestState
	locked bool
}

func (m *mockISOUploadTx) Begin(context.Context) (pgx.Tx, error) { return m, nil }
func (m *mockISOUploadTx) Commit(context.Context) error {
	if m.locked {
		m.state.mu.Unlock()
		m.locked = false
	}
	return nil
}
func (m *mockISOUploadTx) Rollback(context.Context) error {
	if m.locked {
		m.state.mu.Unlock()
		m.locked = false
	}
	return nil
}
func (m *mockISOUploadTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (m *mockISOUploadTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (m *mockISOUploadTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (m *mockISOUploadTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (m *mockISOUploadTx) Conn() *pgx.Conn { return nil }
func (m *mockISOUploadTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (m *mockISOUploadTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}
func (m *mockISOUploadTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "SELECT id FROM vms"):
		m.state.mu.Lock()
		m.locked = true
		return mockISORow{values: []any{"vm-1"}}
	case strings.Contains(sql, "SELECT COUNT(*) FROM iso_uploads"):
		return mockISORow{values: []any{m.state.count}}
	case strings.Contains(sql, "INSERT INTO iso_uploads"):
		m.state.count++
		return mockISORow{values: []any{time.Now()}}
	default:
		return mockISORow{err: errors.New("unexpected query: " + sql)}
	}
}

type mockISORow struct {
	values []any
	err    error
}

func (m mockISORow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	if len(dest) != len(m.values) {
		return errors.New("unexpected scan destination count")
	}
	for i, value := range m.values {
		switch d := dest[i].(type) {
		case *string:
			*d = value.(string)
		case *int:
			*d = value.(int)
		case *time.Time:
			*d = value.(time.Time)
		default:
			return errors.New("unexpected scan destination type")
		}
	}
	return nil
}

func TestISOUploadRepositoryCreateIfUnderLimitConcurrent(t *testing.T) {
	t.Parallel()

	repo := NewISOUploadRepository(&mockISOUploadDB{
		state: &isoUploadTestState{},
	})

	ctx := context.Background()
	planLimit := 1

	uploads := []*ISOUpload{
		{VMID: "vm-1", CustomerID: "customer-1", FileName: "one.iso", FileSize: 1, SHA256: "a", StoragePath: "/tmp/one.iso"},
		{VMID: "vm-1", CustomerID: "customer-1", FileName: "two.iso", FileSize: 1, SHA256: "b", StoragePath: "/tmp/two.iso"},
	}

	errs := make([]error, len(uploads))
	var wg sync.WaitGroup
	wg.Add(len(uploads))

	for i, upload := range uploads {
		go func(idx int, upload *ISOUpload) {
			defer wg.Done()
			errs[idx] = repo.CreateIfUnderLimit(ctx, upload, planLimit)
		}(i, upload)
	}

	wg.Wait()

	var successCount int
	var limitExceededCount int
	for _, err := range errs {
		if err == nil {
			successCount++
			continue
		}
		var limitErr *LimitExceededError
		if errors.As(err, &limitErr) {
			limitExceededCount++
			continue
		}
		require.NoError(t, err)
	}

	require.Equal(t, 1, successCount)
	require.Equal(t, 1, limitExceededCount)
}
