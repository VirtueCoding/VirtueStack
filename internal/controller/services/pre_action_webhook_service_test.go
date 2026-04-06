package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

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

func TestPreActionWebhookServiceCheckPreAction_SendsSignatureInsteadOfRawSecret(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 4, 0, 0, 0, 0, time.UTC)
	var capturedRequest *http.Request
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRequest = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"approved":true}`)
	}))
	defer server.Close()

	db := &fakeDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return &fakeRows{
				rows: [][]any{{
					"550e8400-e29b-41d4-a716-446655440000",
					"approval-hook",
					server.URL,
					"plain-shared-secret",
					[]string{models.PreActionEventVMCreate},
					5000,
					false,
					true,
					now,
					now,
				}},
			}, nil
		},
	}

	service := NewPreActionWebhookService(
		repository.NewPreActionWebhookRepository(db),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	service.client = server.Client()

	err := service.CheckPreAction(context.Background(), models.PreActionEventVMCreate, "customer-1", map[string]any{"vm_id": "vm-1"})
	require.NoError(t, err)
	require.NotNil(t, capturedRequest)
	require.Empty(t, capturedRequest.Header.Get("X-Webhook-Secret"))
	require.NotEmpty(t, capturedRequest.Header.Get("X-Webhook-Signature"))
}

func TestPreActionWebhookServiceCheckPreAction_RejectsLegacyNonHTTPSWebhook(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.April, 4, 0, 0, 0, 0, time.UTC)

	db := &fakeDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			return &fakeRows{
				rows: [][]any{{
					"550e8400-e29b-41d4-a716-446655440000",
					"approval-hook",
					"http://example.com/approve",
					"plain-shared-secret",
					[]string{models.PreActionEventVMCreate},
					5000,
					false,
					true,
					now,
					now,
				}},
			}, nil
		},
	}

	service := NewPreActionWebhookService(
		repository.NewPreActionWebhookRepository(db),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	service.client = &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("request should not be sent")
		}),
	}

	err := service.CheckPreAction(context.Background(), models.PreActionEventVMCreate, "customer-1", map[string]any{"vm_id": "vm-1"})
	require.Error(t, err)
	require.ErrorIs(t, err, sharederrors.ErrForbidden)
	require.Contains(t, err.Error(), "insecure pre-action webhook URL")
}
