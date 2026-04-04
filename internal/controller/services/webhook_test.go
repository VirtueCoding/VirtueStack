package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidWebhookEvents_AllPresent(t *testing.T) {
	expectedEvents := []string{
		"vm.created",
		"vm.deleted",
		"vm.started",
		"vm.stopped",
		"vm.reinstalled",
		"vm.migrated",
		"backup.completed",
		"backup.failed",
		"snapshot.created",
		"bandwidth.threshold",
	}

	assert.Equal(t, len(expectedEvents), len(ValidWebhookEvents))

	for _, event := range expectedEvents {
		assert.True(t, ValidWebhookEvents[event], "expected event %q to be present", event)
	}
}

func TestValidWebhookEvents_UnknownEventAbsent(t *testing.T) {
	assert.False(t, ValidWebhookEvents["unknown.event"])
}

func TestMaxWebhooksPerCustomer(t *testing.T) {
	assert.Equal(t, 5, MaxWebhooksPerCustomer)
}

func TestWebhookErrorVariables(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{"ErrInvalidURL", ErrInvalidURL, "webhook URL must be HTTPS"},
		{"ErrInvalidEvent", ErrInvalidEvent, "invalid webhook event"},
		{"ErrTooManyWebhooks", ErrTooManyWebhooks, "maximum webhook limit reached"},
		{"ErrWebhookNotFound", ErrWebhookNotFound, "webhook not found"},
		{"ErrSecretTooShort", ErrSecretTooShort, "secret must be at least 16 characters"},
		{"ErrSecretTooLong", ErrSecretTooLong, "secret must be at most 128 characters"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.err)
			assert.Equal(t, tt.wantMsg, tt.err.Error())
		})
	}
}

func TestSetSkipURLValidation_PanicsInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")

	svc := &WebhookService{}
	assert.Panics(t, func() {
		svc.SetSkipURLValidation(true)
	})
}

func TestSetSkipURLValidation_NoPanicOutsideProduction(t *testing.T) {
	t.Setenv("APP_ENV", "test")

	svc := &WebhookService{}
	assert.NotPanics(t, func() {
		svc.SetSkipURLValidation(true)
	})
}

type webhookServiceTestDB struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (m *webhookServiceTestDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return webhookServiceTestRow{err: pgx.ErrNoRows}
}

func (m *webhookServiceTestDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (m *webhookServiceTestDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *webhookServiceTestDB) Begin(context.Context) (pgx.Tx, error) {
	return nil, nil
}

type webhookServiceTestRow struct {
	values []any
	err    error
}

func (r webhookServiceTestRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("scan destination count mismatch: got %d want %d", len(dest), len(r.values))
	}
	for i := range dest {
		if err := assignWebhookServiceTestValue(dest[i], r.values[i]); err != nil {
			return err
		}
	}
	return nil
}

func assignWebhookServiceTestValue(dest any, val any) error {
	dv := reflect.ValueOf(dest)
	if dv.Kind() != reflect.Ptr || dv.IsNil() {
		return fmt.Errorf("destination is not a pointer")
	}
	if val == nil {
		dv.Elem().Set(reflect.Zero(dv.Elem().Type()))
		return nil
	}

	v := reflect.ValueOf(val)
	if v.Type().AssignableTo(dv.Elem().Type()) {
		dv.Elem().Set(v)
		return nil
	}
	if v.Type().ConvertibleTo(dv.Elem().Type()) {
		dv.Elem().Set(v.Convert(dv.Elem().Type()))
		return nil
	}
	return fmt.Errorf("cannot assign %T to %T", val, dest)
}

func testWebhookRow() []any {
	now := time.Now().UTC()
	return []any{
		"webhook-1",
		"customer-1",
		"https://old.example.com/webhook",
		"encrypted-secret",
		[]string{"vm.created"},
		true,
		0,
		(*time.Time)(nil),
		(*time.Time)(nil),
		now,
		now,
	}
}

func TestWebhookServiceUpdate_InvalidSecretDoesNotApplyOtherChanges(t *testing.T) {
	newURL := "https://new.example.com/webhook"
	shortSecret := "too-short"
	updateCalled := false
	updateSecretCalled := false

	repo := repository.NewWebhookRepository(&webhookServiceTestDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			return webhookServiceTestRow{values: testWebhookRow()}
		},
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if len(args) > 0 && args[len(args)-1] == "webhook-1" {
				updateCalled = true
			}
			if len(args) == 2 && args[0] != newURL {
				updateSecretCalled = true
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	})

	svc := NewWebhookService(
		repo,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	)
	svc.SetSkipURLValidation(true)

	_, err := svc.Update(context.Background(), "webhook-1", "customer-1", UpdateWebhookRequest{
		URL:    &newURL,
		Secret: &shortSecret,
	})

	require.ErrorIs(t, err, ErrSecretTooShort)
	assert.False(t, updateCalled, "non-secret updates should not be persisted when secret validation fails")
	assert.False(t, updateSecretCalled, "secret update must not run when validation fails")
}

func TestWebhookServiceUpdate_SecretWriteFailureDoesNotPartiallyApplyChanges(t *testing.T) {
	oldURL := "https://old.example.com/webhook"
	storedURL := oldURL
	storedSecret := "encrypted-secret"
	newURL := "https://new.example.com/webhook"
	newSecret := "0123456789abcdef"
	updateAttempted := false

	repo := repository.NewWebhookRepository(&webhookServiceTestDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			rowValues := testWebhookRow()
			rowValues[2] = storedURL
			rowValues[3] = storedSecret
			return webhookServiceTestRow{values: rowValues}
		},
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if !strings.Contains(sql, "UPDATE webhooks SET") {
				return pgconn.NewCommandTag("UPDATE 1"), nil
			}
			updateAttempted = true
			assert.Contains(t, sql, "url =")
			assert.Contains(t, sql, "secret_hash =")
			assert.NotContains(t, sql, "events =")
			assert.NotContains(t, sql, "active =")
			require.Len(t, args, 3)
			assert.Equal(t, newURL, args[0])
			assert.NotEmpty(t, args[1])
			assert.Equal(t, "webhook-1", args[2])
			return pgconn.CommandTag{}, fmt.Errorf("secret write failed")
		},
	})

	svc := NewWebhookService(
		repo,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	)
	svc.SetSkipURLValidation(true)

	_, err := svc.Update(context.Background(), "webhook-1", "customer-1", UpdateWebhookRequest{
		URL:    &newURL,
		Secret: &newSecret,
	})

	require.ErrorContains(t, err, "updating webhook")
	require.ErrorContains(t, err, "secret write failed")
	assert.True(t, updateAttempted, "combined webhook update should be attempted once")
	assert.Equal(t, oldURL, storedURL, "webhook URL should remain unchanged when secret persistence fails")
	assert.Equal(t, "encrypted-secret", storedSecret, "webhook secret should remain unchanged when secret persistence fails")
}
