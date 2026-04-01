package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestPreActionWebhookServiceCheckPreAction_RepositoryErrorFailsClosed(t *testing.T) {
	t.Parallel()

	db := &fakeDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return nil, errors.New("database unavailable")
		},
	}

	service := NewPreActionWebhookService(
		repository.NewPreActionWebhookRepository(db),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	err := service.CheckPreAction(context.Background(), "vm.delete", "customer-1", map[string]any{"vm_id": "vm-1"})
	require.Error(t, err)
	require.ErrorIs(t, err, sharederrors.ErrForbidden)
	require.Contains(t, err.Error(), "failed to load pre-action webhooks")
}

func TestPreActionWebhookServiceCheckPreAction_NoWebhooksConfiguredAllowsAction(t *testing.T) {
	t.Parallel()

	db := &fakeDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return &fakeRows{}, nil
		},
	}

	service := NewPreActionWebhookService(
		repository.NewPreActionWebhookRepository(db),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	err := service.CheckPreAction(context.Background(), "vm.delete", "customer-1", map[string]any{"vm_id": "vm-1"})
	require.NoError(t, err)
}
