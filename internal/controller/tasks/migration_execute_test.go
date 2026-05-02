package tasks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/stretchr/testify/assert"
)

type fakeMigrationDiskCleaner struct {
	calls  int
	nodeID string
	vmID   string
	err    error
}

func (c *fakeMigrationDiskCleaner) DeleteDisk(_ context.Context, nodeID, vmID string) error {
	c.calls++
	c.nodeID = nodeID
	c.vmID = vmID
	return c.err
}

func TestMigrationFinalStatus(t *testing.T) {
	tests := []struct {
		name              string
		preMigrationState string
		want              string
	}{
		{"running stays running", models.VMStatusRunning, models.VMStatusRunning},
		{"stopped stays stopped", models.VMStatusStopped, models.VMStatusStopped},
		{"suspended stays suspended", models.VMStatusSuspended, models.VMStatusSuspended},
		{"empty defaults to running", "", models.VMStatusRunning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, migrationFinalStatus(tt.preMigrationState))
		})
	}
}

func TestCleanupSourceDiskAfterCommit(t *testing.T) {
	tests := []struct {
		name     string
		strategy MigrationStrategy
		err      error
		wantCall bool
	}{
		{"skips shared storage migration", MigrationStrategyLiveSharedStorage, nil, false},
		{"cleans source disk for disk copy migration", MigrationStrategyDiskCopy, nil, true},
		{"logs cleanup failure without failing commit", MigrationStrategyDiskCopy, errors.New("delete failed"), true},
		{"cleans source disk for cold migration", MigrationStrategyCold, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaner := &fakeMigrationDiskCleaner{err: tt.err}
			payload := VMMigratePayload{
				VMID:              "vm-1",
				SourceNodeID:      "source-node",
				MigrationStrategy: tt.strategy,
			}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))

			cleanupSourceDiskAfterCommit(context.Background(), cleaner, payload, logger)

			if !tt.wantCall {
				assert.Zero(t, cleaner.calls)
				return
			}
			assert.Equal(t, 1, cleaner.calls)
			assert.Equal(t, "source-node", cleaner.nodeID)
			assert.Equal(t, "vm-1", cleaner.vmID)
		})
	}
}
