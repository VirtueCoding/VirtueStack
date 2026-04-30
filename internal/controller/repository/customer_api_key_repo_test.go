package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type mockCustomerAPIKeyDB struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockCustomerAPIKeyDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return m.queryRowFunc(ctx, sql, args...)
}

func (m *mockCustomerAPIKeyDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unexpected query")
}

func (m *mockCustomerAPIKeyDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("unexpected exec")
}

func (m *mockCustomerAPIKeyDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, errors.New("unexpected begin")
}

type mockCustomerAPIKeyRow struct {
	key models.CustomerAPIKey
}

func (m mockCustomerAPIKeyRow) Scan(dest ...any) error {
	*(dest[0].(*string)) = m.key.ID
	*(dest[1].(*string)) = m.key.CustomerID
	*(dest[2].(*string)) = m.key.Name
	*(dest[3].(*string)) = m.key.KeyHash
	*(dest[4].(*[]string)) = m.key.AllowedIPs
	*(dest[5].(*[]string)) = m.key.VMIDs
	*(dest[6].(*[]string)) = m.key.Permissions
	*(dest[7].(**time.Time)) = m.key.LastUsedAt
	*(dest[8].(*time.Time)) = m.key.CreatedAt
	*(dest[9].(**time.Time)) = m.key.RevokedAt
	*(dest[10].(**time.Time)) = m.key.ExpiresAt
	return nil
}

func TestCustomerAPIKeyRepositoryCreatePersistsExpiresAt(t *testing.T) {
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Microsecond)

	tests := []struct {
		name string
		key  models.CustomerAPIKey
	}{
		{
			name: "persists non-nil expires_at",
			key: models.CustomerAPIKey{
				ID:          uuid.NewString(),
				CustomerID:  uuid.NewString(),
				Name:        "expiring key",
				KeyHash:     "hash-expiring-key",
				Permissions: []string{"vm:read"},
				ExpiresAt:   &expiresAt,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockCustomerAPIKeyDB{
				queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
					normalizedSQL := strings.Join(strings.Fields(sql), " ")
					require.Contains(t, normalizedSQL, "permissions, expires_at")
					require.Len(t, args, 8)
					require.Equal(t, tt.key.ExpiresAt, args[7])

					created := tt.key
					created.CreatedAt = time.Now().UTC()
					return mockCustomerAPIKeyRow{key: created}
				},
			}
			repo := NewCustomerAPIKeyRepository(db)
			key := tt.key

			require.NoError(t, repo.Create(context.Background(), &key))
			require.NotNil(t, key.ExpiresAt)
			require.WithinDuration(t, expiresAt, *key.ExpiresAt, time.Second)
		})
	}
}
