package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

type mockBillingHook struct {
	called    bool
	returnErr error
}

func (m *mockBillingHook) OnVMCreated(_ context.Context, _ billing.VMRef) error {
	m.called = true
	return m.returnErr
}

func (m *mockBillingHook) OnVMDeleted(_ context.Context, _ billing.VMRef) error {
	m.called = true
	return m.returnErr
}

func (m *mockBillingHook) OnVMResized(_ context.Context, _ billing.VMRef, _, _ string) error {
	m.called = true
	return m.returnErr
}

type mockBillingHookResolver struct {
	hook    billing.VMLifecycleHook
	hookErr error
}

func (m *mockBillingHookResolver) ForCustomer(_ string) (billing.VMLifecycleHook, error) {
	return m.hook, m.hookErr
}

type billingTestCustomerDB struct {
	customer *models.Customer
	err      error
}

func (d *billingTestCustomerDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	return &billingTestCustomerRow{customer: d.customer, err: d.err}
}

func (d *billingTestCustomerDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}

func (d *billingTestCustomerDB) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (d *billingTestCustomerDB) Begin(_ context.Context) (pgx.Tx, error) {
	return nil, nil
}

type billingTestCustomerRow struct {
	customer *models.Customer
	err      error
}

func (r *billingTestCustomerRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.customer == nil {
		return pgx.ErrNoRows
	}
	c := r.customer
	if len(dest) >= 14 {
		if v, ok := dest[0].(*string); ok {
			*v = c.ID
		}
		if v, ok := dest[1].(*string); ok {
			*v = c.Email
		}
		if v, ok := dest[2].(*string); ok {
			*v = c.PasswordHash
		}
		if v, ok := dest[3].(*string); ok {
			*v = c.Name
		}
		if v, ok := dest[4].(**string); ok {
			*v = c.Phone
		}
		if v, ok := dest[5].(**int); ok {
			*v = c.WHMCSClientID
		}
		if v, ok := dest[6].(*string); ok {
			*v = c.BillingProvider
		}
		if v, ok := dest[7].(**string); ok {
			*v = c.TOTPSecretEncrypted
		}
		if v, ok := dest[8].(*bool); ok {
			*v = c.TOTPEnabled
		}
		if v, ok := dest[9].(*[]string); ok {
			*v = c.TOTPBackupCodesHash
		}
		if v, ok := dest[10].(*bool); ok {
			*v = c.TOTPBackupCodesShown
		}
		if v, ok := dest[11].(*string); ok {
			*v = c.Status
		}
		if v, ok := dest[12].(*interface{}); ok {
			_ = v
		}
		if v, ok := dest[13].(*interface{}); ok {
			_ = v
		}
	}
	return nil
}

func billingTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNotifyBillingHook_NilResolver(t *testing.T) {
	svc := &VMService{
		billingHooks: nil,
		customerRepo: repository.NewCustomerRepository(&billingTestCustomerDB{
			customer: &models.Customer{ID: "c1", BillingProvider: "whmcs"},
		}),
		logger: billingTestLogger().With("component", "vm-service"),
	}

	hook := &mockBillingHook{}

	svc.notifyBillingHook(context.Background(), "c1", func(h billing.VMLifecycleHook) error {
		return h.OnVMCreated(context.Background(), billing.VMRef{})
	})

	assert.False(t, hook.called)
}

func TestNotifyBillingHook_NilCustomerRepo(t *testing.T) {
	hook := &mockBillingHook{}
	svc := &VMService{
		billingHooks: &mockBillingHookResolver{hook: hook},
		customerRepo: nil,
		logger:       billingTestLogger().With("component", "vm-service"),
	}

	svc.notifyBillingHook(context.Background(), "c1", func(h billing.VMLifecycleHook) error {
		return h.OnVMCreated(context.Background(), billing.VMRef{})
	})

	assert.False(t, hook.called)
}

func TestNotifyBillingHook_ProviderNotFound(t *testing.T) {
	hook := &mockBillingHook{}
	svc := &VMService{
		billingHooks: &mockBillingHookResolver{
			hook:    nil,
			hookErr: errors.New("provider not found"),
		},
		customerRepo: repository.NewCustomerRepository(&billingTestCustomerDB{
			customer: &models.Customer{ID: "c1", BillingProvider: "unknown"},
		}),
		logger: billingTestLogger().With("component", "vm-service"),
	}

	svc.notifyBillingHook(context.Background(), "c1", func(h billing.VMLifecycleHook) error {
		return h.OnVMCreated(context.Background(), billing.VMRef{})
	})

	assert.False(t, hook.called)
}

func TestNotifyBillingHook_HookError(t *testing.T) {
	hook := &mockBillingHook{returnErr: errors.New("hook failed")}
	svc := &VMService{
		billingHooks: &mockBillingHookResolver{hook: hook},
		customerRepo: repository.NewCustomerRepository(&billingTestCustomerDB{
			customer: &models.Customer{ID: "c1", BillingProvider: "whmcs"},
		}),
		logger: billingTestLogger().With("component", "vm-service"),
	}

	svc.notifyBillingHook(context.Background(), "c1", func(h billing.VMLifecycleHook) error {
		return h.OnVMCreated(context.Background(), billing.VMRef{})
	})

	assert.True(t, hook.called)
}
