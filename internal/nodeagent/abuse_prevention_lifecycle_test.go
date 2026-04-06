package nodeagent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type abuseLifecycleTestManager struct {
	applyCalls  []string
	removeCalls []string
	applyErr    error
	removeErr   error
}

func (m *abuseLifecycleTestManager) ApplyVMRules(_ context.Context, tapInterface string) error {
	m.applyCalls = append(m.applyCalls, tapInterface)
	return m.applyErr
}

func (m *abuseLifecycleTestManager) RemoveVMRules(_ context.Context, tapInterface string) error {
	m.removeCalls = append(m.removeCalls, tapInterface)
	return m.removeErr
}

func TestEnsureAbusePreventionAfterCreateDeletesVMWhenRuleInstallFails(t *testing.T) {
	t.Parallel()

	manager := &abuseLifecycleTestManager{
		applyErr: errors.New("nft add rule failed"),
	}
	deletedVMs := make([]string, 0, 1)
	deletedDisks := make([]string, 0, 1)

	err := ensureAbusePreventionAfterCreate(
		context.Background(),
		"vm-1",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(context.Context, string) (string, error) {
			return "vnet0", nil
		},
		manager,
		func(_ context.Context, vmID string) error {
			deletedVMs = append(deletedVMs, vmID)
			return nil
		},
		func(_ context.Context, vmID string) error {
			deletedDisks = append(deletedDisks, vmID)
			return nil
		},
	)

	require.Error(t, err)
	assert.ErrorContains(t, err, "applying abuse prevention rules")
	assert.Equal(t, []string{"vnet0"}, manager.applyCalls)
	assert.Equal(t, []string{"vnet0"}, manager.removeCalls)
	assert.Equal(t, []string{"vm-1"}, deletedVMs)
	assert.Equal(t, []string{"vm-1"}, deletedDisks)
}

func TestEnsureAbusePreventionAfterCreateRetriesTapLookupBeforeFailing(t *testing.T) {
	t.Parallel()

	manager := &abuseLifecycleTestManager{}
	lookupCalls := 0

	originalRetries := abusePreventionTapLookupRetries
	originalInterval := abusePreventionTapLookupInterval
	abusePreventionTapLookupRetries = 3
	abusePreventionTapLookupInterval = time.Millisecond
	t.Cleanup(func() {
		abusePreventionTapLookupRetries = originalRetries
		abusePreventionTapLookupInterval = originalInterval
	})

	err := ensureAbusePreventionAfterCreate(
		context.Background(),
		"vm-2",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(context.Context, string) (string, error) {
			lookupCalls++
			if lookupCalls < 3 {
				return "", errors.New("tap not ready")
			}
			return "vnet9", nil
		},
		manager,
		func(context.Context, string) error {
			t.Fatal("deleteVM should not be called when tap lookup eventually succeeds")
			return nil
		},
		func(context.Context, string) error {
			t.Fatal("deleteDisk should not be called when tap lookup eventually succeeds")
			return nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, 3, lookupCalls)
	assert.Equal(t, []string{"vnet9"}, manager.applyCalls)
	assert.Empty(t, manager.removeCalls)
}

func TestDeleteVMWithAbusePreventionCleanupStillDeletesOnRuleCleanupFailure(t *testing.T) {
	t.Parallel()

	manager := &abuseLifecycleTestManager{
		removeErr: errors.New("nft delete chain failed"),
	}
	deletedVMs := make([]string, 0, 1)
	deletedDisks := make([]string, 0, 1)

	err := deleteVMWithAbusePreventionCleanup(
		context.Background(),
		"vm-3",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(context.Context, string) (string, error) {
			return "vnet5", nil
		},
		manager,
		func(_ context.Context, vmID string) error {
			deletedVMs = append(deletedVMs, vmID)
			return nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, []string{"vnet5"}, manager.removeCalls)
	assert.Equal(t, []string{"vm-3"}, deletedVMs)
	assert.Empty(t, deletedDisks)
}

func TestEnsureAbusePreventionAfterCreateDeletesDiskEvenWhenVMCleanupFails(t *testing.T) {
	t.Parallel()

	manager := &abuseLifecycleTestManager{
		applyErr: errors.New("nft add rule failed"),
	}
	deletedDisks := make([]string, 0, 1)

	err := ensureAbusePreventionAfterCreate(
		context.Background(),
		"vm-4",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(context.Context, string) (string, error) {
			return "vnet4", nil
		},
		manager,
		func(context.Context, string) error {
			return errors.New("domain delete failed")
		},
		func(_ context.Context, vmID string) error {
			deletedDisks = append(deletedDisks, vmID)
			return nil
		},
	)

	require.Error(t, err)
	assert.ErrorContains(t, err, "cleanup VM vm-4")
	assert.Equal(t, []string{"vm-4"}, deletedDisks)
}
