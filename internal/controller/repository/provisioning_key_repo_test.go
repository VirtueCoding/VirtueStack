package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type mockProvisioningKeyDB struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockProvisioningKeyDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return m.queryRowFunc(ctx, sql, args...)
}

func (m *mockProvisioningKeyDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected query")
}

func (m *mockProvisioningKeyDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected exec")
}

func (m *mockProvisioningKeyDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("unexpected begin")
}

type errorProvisioningKeyRow struct{}

func (errorProvisioningKeyRow) Scan(...any) error {
	return errors.New("stop after query capture")
}

func TestProvisioningKeyUpdatePreservesExpiryWhenOmitted(t *testing.T) {
	var capturedSQL string
	db := &mockProvisioningKeyDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			capturedSQL = sql
			return errorProvisioningKeyRow{}
		},
	}
	repo := NewProvisioningKeyRepository(db)

	_, err := repo.Update(context.Background(), "key-1", &models.ProvisioningKeyUpdateRequest{})

	require.Error(t, err)
	require.Contains(t, strings.Join(strings.Fields(capturedSQL), " "), "expires_at = COALESCE($4, expires_at)")
}
