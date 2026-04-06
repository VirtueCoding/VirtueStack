package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTx struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFunc    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	commitFunc   func(ctx context.Context) error
	rollbackFunc func(ctx context.Context) error
}

func (f *fakeTx) Begin(ctx context.Context) (pgx.Tx, error) {
	return nil, fmt.Errorf("nested tx not implemented")
}
func (f *fakeTx) Commit(ctx context.Context) error {
	if f.commitFunc != nil {
		return f.commitFunc(ctx)
	}
	return nil
}
func (f *fakeTx) Rollback(ctx context.Context) error {
	if f.rollbackFunc != nil {
		return f.rollbackFunc(ctx)
	}
	return nil
}
func (f *fakeTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, fmt.Errorf("not implemented")
}
func (f *fakeTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults { return nil }
func (f *fakeTx) LargeObjects() pgx.LargeObjects                               { return pgx.LargeObjects{} }
func (f *fakeTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *fakeTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if f.execFunc != nil {
		return f.execFunc(ctx, sql, arguments...)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (f *fakeTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if f.queryFunc != nil {
		return f.queryFunc(ctx, sql, args...)
	}
	return &fakeRows{}, nil
}
func (f *fakeTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if f.queryRowFunc != nil {
		return f.queryRowFunc(ctx, sql, args...)
	}
	return &fakeRow{scanErr: pgx.ErrNoRows}
}
func (f *fakeTx) Conn() *pgx.Conn { return nil }

type mockBackupNodeAgent struct {
	deleteQCOWBackupErr  error
	createQCOWBackupSize int64
	restoreSnapshotName  string
}

func (m *mockBackupNodeAgent) CreateSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	return nil
}
func (m *mockBackupNodeAgent) DeleteSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	return nil
}
func (m *mockBackupNodeAgent) RestoreSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	m.restoreSnapshotName = snapshotName
	return nil
}
func (m *mockBackupNodeAgent) CloneSnapshot(ctx context.Context, nodeID, vmID, snapshotName, backupPath string) error {
	return nil
}
func (m *mockBackupNodeAgent) GetVMNodeID(ctx context.Context, vmID string) (string, error) {
	return "", nil
}
func (m *mockBackupNodeAgent) CreateQCOWSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName string) error {
	return nil
}
func (m *mockBackupNodeAgent) DeleteQCOWSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName string) error {
	return nil
}
func (m *mockBackupNodeAgent) CreateQCOWBackup(ctx context.Context, nodeID, vmID, diskPath, snapshotName, backupPath string, compress bool) (int64, error) {
	if m.createQCOWBackupSize != 0 {
		return m.createQCOWBackupSize, nil
	}
	return 0, nil
}
func (m *mockBackupNodeAgent) RestoreQCOWBackup(ctx context.Context, nodeID, vmID, backupPath, targetPath string) error {
	return nil
}
func (m *mockBackupNodeAgent) DeleteQCOWBackupFile(ctx context.Context, nodeID, backupPath string) error {
	return m.deleteQCOWBackupErr
}
func (m *mockBackupNodeAgent) GetQCOWDiskInfo(ctx context.Context, nodeID, diskPath string) (*tasks.QCOWDiskInfo, error) {
	return nil, nil
}

func testBackupServiceLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestBackupService_CreateBackupWithLimitCheckErrors(t *testing.T) {
	tests := []struct {
		name        string
		vmScanErr   error
		limitCount  int
		wantErr     bool
		errContains string
	}{
		{
			name:        "vm not found returns not found error",
			vmScanErr:   pgx.ErrNoRows,
			wantErr:     true,
			errContains: "VM not found",
		},
		{
			name:        "quota exceeded returns limit error",
			limitCount:  2,
			wantErr:     true,
			errContains: "backup limit exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := &fakeTx{
				execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("SELECT 1"), nil
				},
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					switch {
					case strings.Contains(sql, "SELECT COUNT(*) FROM backups WHERE vm_id = $1 AND status != 'deleted'"):
						return &fakeRow{values: []any{tt.limitCount}}
					case strings.Contains(sql, "INSERT INTO backups"):
						return &fakeRow{values: backupRow("backup-1", "vm-1")}
					default:
						return &fakeRow{scanErr: pgx.ErrNoRows}
					}
				},
			}

			db := &fakeDB{
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					if strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL") {
						return &fakeRow{values: backupVMRow(models.StorageBackendQcow, nil), scanErr: tt.vmScanErr}
					}
					return &fakeRow{scanErr: pgx.ErrNoRows}
				},
				beginFunc: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
			}

			service := NewBackupService(BackupServiceConfig{
				BackupRepo:   repository.NewBackupRepository(db),
				SnapshotRepo: repository.NewBackupRepository(db),
				VMRepo:       repository.NewVMRepository(db),
				Logger:       testBackupServiceLogger(),
			})

			backup, err := service.CreateBackupWithLimitCheck(context.Background(), "vm-1", "test", 2)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, backup)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, backup)
		})
	}
}

func TestBackupService_CreateBackupWithLimitCheckPersistsProvidedName(t *testing.T) {
	requestedName := "nightly backup"

	tx := &fakeTx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("SELECT 1"), nil
		},
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "SELECT COUNT(*) FROM backups WHERE vm_id = $1 AND status != 'deleted'"):
				return &fakeRow{values: []any{0}}
			case strings.Contains(sql, "INSERT INTO backups"):
				insertedName, _ := args[2].(*string)
				row := backupRowWithName("backup-1", "vm-1", insertedName)
				row[6] = models.StorageBackendQcow
				return &fakeRow{values: row}
			default:
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL") {
				return &fakeRow{values: backupVMRow(models.StorageBackendQcow, nil)}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
		beginFunc: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
	}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:   repository.NewBackupRepository(db),
		SnapshotRepo: repository.NewBackupRepository(db),
		VMRepo:       repository.NewVMRepository(db),
		Logger:       testBackupServiceLogger(),
	})

	backup, err := service.CreateBackupWithLimitCheck(context.Background(), "vm-1", requestedName, 2)
	require.NoError(t, err)
	require.NotNil(t, backup)
	require.NotNil(t, backup.Name)
	assert.Equal(t, requestedName, *backup.Name)
}

func TestBackupService_RestoreBackupNotFound(t *testing.T) {
	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM backups WHERE id = $1") {
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
	}
	service := NewBackupService(BackupServiceConfig{
		BackupRepo:   repository.NewBackupRepository(db),
		SnapshotRepo: repository.NewBackupRepository(db),
		VMRepo:       repository.NewVMRepository(db),
		Logger:       testBackupServiceLogger(),
	})

	err := service.RestoreBackup(context.Background(), "missing-backup")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backup not found")
}

func TestBackupService_CreateSnapshotAsyncQuotaExceeded(t *testing.T) {
	tx := &fakeTx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("SELECT 1"), nil
		},
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "SELECT COUNT(*) FROM snapshots WHERE vm_id = $1") {
				return &fakeRow{values: []any{DefaultSnapshotQuota}}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
	}
	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL") {
				return &fakeRow{values: vmRow("vm-1", "cust-1", strPtr("node-1"), models.VMStatusRunning, nil)}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
		beginFunc: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
	}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:   repository.NewBackupRepository(db),
		SnapshotRepo: repository.NewBackupRepository(db),
		VMRepo:       repository.NewVMRepository(db),
		Logger:       testBackupServiceLogger(),
	})

	snapshot, taskID, err := service.CreateSnapshotAsync(context.Background(), "vm-1", "snap", "cust-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot quota exceeded")
	assert.Nil(t, snapshot)
	assert.Equal(t, "", taskID)
}

func TestBackupService_CreateSnapshotAsyncPublishesPersistedSnapshotID(t *testing.T) {
	const persistedSnapshotID = "persisted-snapshot-id"

	tx := &fakeTx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("SELECT 1"), nil
		},
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "SELECT COUNT(*) FROM snapshots WHERE vm_id = $1"):
				return &fakeRow{values: []any{0}}
			case strings.Contains(sql, "INSERT INTO snapshots"):
				return &fakeRow{values: snapshotRowWithID(persistedSnapshotID, args[0].(string), args[1].(string), args[2].(string), nil)}
			default:
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL") {
				return &fakeRow{values: vmRow("vm-1", "cust-1", strPtr("node-1"), models.VMStatusRunning, nil)}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
		beginFunc: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
	}

	var publishedSnapshotID string
	pub := &testTaskPublisher{publishTaskFunc: func(ctx context.Context, taskType string, payload map[string]any) (string, error) {
		assert.Equal(t, models.TaskTypeSnapshotCreate, taskType)
		publishedSnapshotID, _ = payload["snapshot_id"].(string)
		return "task-1", nil
	}}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:    repository.NewBackupRepository(db),
		SnapshotRepo:  repository.NewBackupRepository(db),
		VMRepo:        repository.NewVMRepository(db),
		TaskPublisher: pub,
		Logger:        testBackupServiceLogger(),
	})

	snapshot, taskID, err := service.CreateSnapshotAsyncWithLimit(context.Background(), "vm-1", "snap", "cust-1", 2)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Equal(t, persistedSnapshotID, snapshot.ID)
	assert.Equal(t, persistedSnapshotID, publishedSnapshotID)
	assert.Equal(t, "task-1", taskID)
}

func TestBackupService_CreateSnapshotAsyncDeletesPersistedSnapshotOnPublishFailure(t *testing.T) {
	const persistedSnapshotID = "persisted-snapshot-id"

	tx := &fakeTx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("SELECT 1"), nil
		},
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "SELECT COUNT(*) FROM snapshots WHERE vm_id = $1"):
				return &fakeRow{values: []any{0}}
			case strings.Contains(sql, "INSERT INTO snapshots"):
				return &fakeRow{values: snapshotRowWithID(persistedSnapshotID, args[0].(string), args[1].(string), args[2].(string), nil)}
			default:
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}

	var deletedSnapshotID string
	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL") {
				return &fakeRow{values: vmRow("vm-1", "cust-1", strPtr("node-1"), models.VMStatusRunning, nil)}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "DELETE FROM snapshots WHERE id = $1") {
				deletedSnapshotID, _ = args[0].(string)
				return pgconn.NewCommandTag("DELETE 1"), nil
			}
			return pgconn.CommandTag{}, fmt.Errorf("unexpected exec: %s", sql)
		},
		beginFunc: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
	}

	pub := &testTaskPublisher{publishTaskFunc: func(ctx context.Context, taskType string, payload map[string]any) (string, error) {
		return "", fmt.Errorf("publish failed")
	}}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:    repository.NewBackupRepository(db),
		SnapshotRepo:  repository.NewBackupRepository(db),
		VMRepo:        repository.NewVMRepository(db),
		TaskPublisher: pub,
		Logger:        testBackupServiceLogger(),
	})

	snapshot, taskID, err := service.CreateSnapshotAsyncWithLimit(context.Background(), "vm-1", "snap", "cust-1", 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "publishing snapshot task")
	assert.Nil(t, snapshot)
	assert.Equal(t, "", taskID)
	assert.Equal(t, persistedSnapshotID, deletedSnapshotID)
}

func TestBackupService_CreateSnapshotAsyncReturnsCleanupFailureOnPublishFailure(t *testing.T) {
	const persistedSnapshotID = "persisted-snapshot-id"

	tx := &fakeTx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("SELECT 1"), nil
		},
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "SELECT COUNT(*) FROM snapshots WHERE vm_id = $1"):
				return &fakeRow{values: []any{0}}
			case strings.Contains(sql, "INSERT INTO snapshots"):
				return &fakeRow{values: snapshotRowWithID(persistedSnapshotID, args[0].(string), args[1].(string), args[2].(string), nil)}
			default:
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL") {
				return &fakeRow{values: vmRow("vm-1", "cust-1", strPtr("node-1"), models.VMStatusRunning, nil)}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "DELETE FROM snapshots WHERE id = $1") {
				return pgconn.CommandTag{}, fmt.Errorf("delete failed")
			}
			return pgconn.CommandTag{}, fmt.Errorf("unexpected exec: %s", sql)
		},
		beginFunc: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
	}

	pub := &testTaskPublisher{publishTaskFunc: func(ctx context.Context, taskType string, payload map[string]any) (string, error) {
		return "", fmt.Errorf("publish failed")
	}}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:    repository.NewBackupRepository(db),
		SnapshotRepo:  repository.NewBackupRepository(db),
		VMRepo:        repository.NewVMRepository(db),
		TaskPublisher: pub,
		Logger:        testBackupServiceLogger(),
	})

	snapshot, taskID, err := service.CreateSnapshotAsyncWithLimit(context.Background(), "vm-1", "snap", "cust-1", 2)
	require.Error(t, err)
	assert.Nil(t, snapshot)
	assert.Equal(t, "", taskID)
	assert.ErrorContains(t, err, "publishing snapshot task")
	assert.ErrorContains(t, err, "cleaning up snapshot record")
	assert.ErrorContains(t, err, "delete failed")
}

func TestBackupService_CreateSnapshotAsyncPersistsQCOWSnapshotMetadata(t *testing.T) {
	const persistedSnapshotID = "persisted-qcow-snapshot-id"

	var insertedStorageBackend string
	var insertedQCOWSnapshot *string

	tx := &fakeTx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("SELECT 1"), nil
		},
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "SELECT COUNT(*) FROM snapshots WHERE vm_id = $1"):
				return &fakeRow{values: []any{0}}
			case strings.Contains(sql, "INSERT INTO snapshots"):
				if len(args) < 6 {
					return &fakeRow{scanErr: fmt.Errorf("expected snapshot insert to include storage_backend and qcow_snapshot")}
				}
				insertedStorageBackend, _ = args[2].(string)
				insertedQCOWSnapshot, _ = args[4].(*string)
				return &fakeRow{values: snapshotRowWithStorage(persistedSnapshotID, args[0].(string), args[1].(string), insertedStorageBackend, "", insertedQCOWSnapshot, nil)}
			default:
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL") {
				return &fakeRow{values: backupVMRow(models.StorageBackendQcow, strPtr("/var/lib/virtuestack/vms/vm-1-disk0.qcow2"))}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
		beginFunc: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
	}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:   repository.NewBackupRepository(db),
		SnapshotRepo: repository.NewBackupRepository(db),
		VMRepo:       repository.NewVMRepository(db),
		Logger:       testBackupServiceLogger(),
	})

	snapshot, taskID, err := service.CreateSnapshotAsyncWithLimit(context.Background(), "vm-1", "snap", "cust-1", 2)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Equal(t, "", taskID)
	assert.Equal(t, models.StorageBackendQcow, insertedStorageBackend)
	require.NotNil(t, insertedQCOWSnapshot)
	assert.Equal(t, models.StorageBackendQcow, snapshot.StorageBackend)
	require.NotNil(t, snapshot.QCOWSnapshot)
	assert.Equal(t, *insertedQCOWSnapshot, *snapshot.QCOWSnapshot)
	assert.Empty(t, snapshot.RBDSnapshot)
}

func TestBackupService_RestoreSnapshotUsesQCOWHandle(t *testing.T) {
	const snapshotID = "snapshot-1"
	const qcowSnapshotHandle = "qcow-snapshot-handle"

	nodeAgent := &mockBackupNodeAgent{}
	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM snapshots WHERE id = $1"):
				return &fakeRow{values: snapshotRowWithStorage(snapshotID, "vm-1", "snap", models.StorageBackendQcow, "", strPtr(qcowSnapshotHandle), nil)}
			case strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL"):
				return &fakeRow{values: backupVMRow(models.StorageBackendQcow, strPtr("/var/lib/virtuestack/vms/vm-1-disk0.qcow2"))}
			default:
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:   repository.NewBackupRepository(db),
		SnapshotRepo: repository.NewBackupRepository(db),
		VMRepo:       repository.NewVMRepository(db),
		NodeAgent:    nodeAgent,
		Logger:       testBackupServiceLogger(),
	})

	err := service.RestoreSnapshot(context.Background(), snapshotID)
	require.NoError(t, err)
	assert.Equal(t, qcowSnapshotHandle, nodeAgent.restoreSnapshotName)
}

func TestBackupService_RestoreSnapshotRejectsQCOWSnapshotWithoutQCOWHandle(t *testing.T) {
	const snapshotID = "snapshot-1"

	nodeAgent := &mockBackupNodeAgent{}
	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM snapshots WHERE id = $1"):
				return &fakeRow{values: snapshotRowWithStorage(snapshotID, "vm-1", "snap", models.StorageBackendQcow, "stale-rbd-handle", nil, nil)}
			case strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL"):
				return &fakeRow{values: backupVMRow(models.StorageBackendQcow, strPtr("/var/lib/virtuestack/vms/vm-1-disk0.qcow2"))}
			default:
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:   repository.NewBackupRepository(db),
		SnapshotRepo: repository.NewBackupRepository(db),
		VMRepo:       repository.NewVMRepository(db),
		NodeAgent:    nodeAgent,
		Logger:       testBackupServiceLogger(),
	})

	err := service.RestoreSnapshot(context.Background(), snapshotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no backend snapshot handle")
	assert.Empty(t, nodeAgent.restoreSnapshotName)
}

func TestBackupService_CreateBackupWithLimitCheckPersistsQCOWStorageMetadata(t *testing.T) {
	var persisted completedBackupUpdate

	tx := &fakeTx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("SELECT 1"), nil
		},
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "SELECT COUNT(*) FROM backups WHERE vm_id = $1 AND status != 'deleted'"):
				return &fakeRow{values: []any{0}}
			case strings.Contains(sql, "INSERT INTO backups"):
				return &fakeRow{values: backupRow("backup-1", "vm-1")}
			default:
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL") {
				return &fakeRow{values: backupVMRow(models.StorageBackendQcow, strPtr("/var/lib/virtuestack/vms/vm-1-disk0.qcow2"))}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "UPDATE backups SET") {
				persisted = completedBackupUpdate{
					filePath:     stringArg(args, 0),
					snapshotName: stringArg(args, 1),
					storagePath:  stringArg(args, 2),
					sizeBytes:    int64Arg(args, 3),
					rbdSnapshot:  stringArg(args, 4),
				}
				return pgconn.NewCommandTag("UPDATE 1"), nil
			}
			return pgconn.CommandTag{}, fmt.Errorf("unexpected exec: %s", sql)
		},
		beginFunc: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
	}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:   repository.NewBackupRepository(db),
		SnapshotRepo: repository.NewBackupRepository(db),
		VMRepo:       repository.NewVMRepository(db),
		NodeAgent:    &mockBackupNodeAgent{createQCOWBackupSize: 4096},
		Logger:       testBackupServiceLogger(),
	})

	backup, err := service.CreateBackupWithLimitCheck(context.Background(), "vm-1", "metadata-test", 2)
	require.NoError(t, err)
	require.NotNil(t, backup)
	require.NotNil(t, backup.FilePath)
	require.NotNil(t, backup.SnapshotName)
	require.NotNil(t, backup.SizeBytes)
	require.NotNil(t, persisted.filePath)
	require.NotNil(t, persisted.snapshotName)
	require.NotNil(t, persisted.sizeBytes)
	assert.Equal(t, *backup.FilePath, *persisted.filePath)
	assert.Equal(t, *backup.SnapshotName, *persisted.snapshotName)
	assert.Equal(t, *backup.SizeBytes, *persisted.sizeBytes)
}

func TestBackupService_CreateBackupRejectsNonQCOWFullBackupsWithoutPersistingPlaceholder(t *testing.T) {
	var insertCalled bool

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL"):
				return &fakeRow{values: backupVMRow(models.StorageBackendCeph, nil)}
			case strings.Contains(sql, "INSERT INTO backups"):
				insertCalled = true
				return &fakeRow{values: backupRow("backup-1", "vm-1")}
			default:
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
		},
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "UPDATE backups SET status = $1 WHERE id = $2") {
				return pgconn.NewCommandTag("UPDATE 1"), nil
			}
			return pgconn.CommandTag{}, fmt.Errorf("unexpected exec: %s", sql)
		},
	}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:   repository.NewBackupRepository(db),
		SnapshotRepo: repository.NewBackupRepository(db),
		VMRepo:       repository.NewVMRepository(db),
		NodeAgent:    &mockBackupNodeAgent{},
		Logger:       testBackupServiceLogger(),
	})

	backup, err := service.CreateBackup(context.Background(), "vm-1", "ceph-backup")
	require.Error(t, err)
	assert.Nil(t, backup)
	assert.ErrorIs(t, err, sharederrors.ErrNotSupported)
	assert.Contains(t, err.Error(), "synchronous full backups are not supported for storage backend")
	assert.False(t, insertCalled)
}

func TestBackupService_CreateBackupWithLimitCheckRejectsNonQCOWFullBackupsWithoutPersistingPlaceholder(t *testing.T) {
	tx := &fakeTx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("SELECT 1"), nil
		},
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "SELECT COUNT(*) FROM backups WHERE vm_id = $1 AND status != 'deleted'"):
				return &fakeRow{values: []any{0}}
			case strings.Contains(sql, "INSERT INTO backups"):
				t.Fatalf("unexpected backup placeholder insert for unsupported backend: %s", sql)
				return &fakeRow{values: backupRow("backup-1", "vm-1")}
			default:
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}

	var statusUpdates []string
	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL") {
				return &fakeRow{values: backupVMRow(models.StorageBackendCeph, nil)}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "UPDATE backups SET status = $1 WHERE id = $2") {
				if status, ok := args[0].(string); ok {
					statusUpdates = append(statusUpdates, status)
				}
				return pgconn.NewCommandTag("UPDATE 1"), nil
			}
			return pgconn.CommandTag{}, fmt.Errorf("unexpected exec: %s", sql)
		},
		beginFunc: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
	}

	nodeAgent := &mockBackupNodeAgent{}
	service := NewBackupService(BackupServiceConfig{
		BackupRepo:   repository.NewBackupRepository(db),
		SnapshotRepo: repository.NewBackupRepository(db),
		VMRepo:       repository.NewVMRepository(db),
		NodeAgent:    nodeAgent,
		Logger:       testBackupServiceLogger(),
	})

	backup, err := service.CreateBackupWithLimitCheck(context.Background(), "vm-1", "ceph-backup", 2)
	require.Error(t, err)
	assert.Nil(t, backup)
	assert.ErrorIs(t, err, sharederrors.ErrNotSupported)
	assert.Contains(t, err.Error(), "synchronous full backups are not supported for storage backend")
	assert.Empty(t, statusUpdates)
}

func TestBackupService_CreateBackupWithLimitCheckRejectsNonQCOWFullBackupsWithoutNodeAgent(t *testing.T) {
	tx := &fakeTx{
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("SELECT 1"), nil
		},
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "SELECT COUNT(*) FROM backups WHERE vm_id = $1 AND status != 'deleted'"):
				return &fakeRow{values: []any{0}}
			case strings.Contains(sql, "INSERT INTO backups"):
				t.Fatalf("unexpected backup placeholder insert for unsupported backend without node agent: %s", sql)
				return &fakeRow{values: backupRow("backup-1", "vm-1")}
			default:
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}

	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL") {
				return &fakeRow{values: backupVMRow(models.StorageBackendCeph, nil)}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
		execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "UPDATE backups SET status = $1 WHERE id = $2") {
				t.Fatalf("unexpected status update for unsupported backend without node agent: %s", sql)
			}
			return pgconn.CommandTag{}, fmt.Errorf("unexpected exec: %s", sql)
		},
		beginFunc: func(ctx context.Context) (pgx.Tx, error) { return tx, nil },
	}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:   repository.NewBackupRepository(db),
		SnapshotRepo: repository.NewBackupRepository(db),
		VMRepo:       repository.NewVMRepository(db),
		Logger:       testBackupServiceLogger(),
	})

	backup, err := service.CreateBackupWithLimitCheck(context.Background(), "vm-1", "ceph-backup", 2)
	require.Error(t, err)
	assert.Nil(t, backup)
	assert.ErrorIs(t, err, sharederrors.ErrNotSupported)
	assert.Contains(t, err.Error(), "synchronous full backups are not supported for storage backend")
}

func TestBackupService_DeleteBackupStorageError(t *testing.T) {
	backupID := "backup-1"
	qcowPath := "/backup/path.qcow2"
	snapshotName := "snap-1"
	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			switch {
			case strings.Contains(sql, "FROM backups WHERE id = $1"):
				return &fakeRow{values: backupRowWithQCOW(backupID, "vm-1", qcowPath, snapshotName)}
			case strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL"):
				return &fakeRow{values: vmRow("vm-1", "cust-1", strPtr("node-1"), models.VMStatusRunning, nil)}
			default:
				return &fakeRow{scanErr: pgx.ErrNoRows}
			}
		},
	}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:   repository.NewBackupRepository(db),
		SnapshotRepo: repository.NewBackupRepository(db),
		VMRepo:       repository.NewVMRepository(db),
		NodeAgent:    &mockBackupNodeAgent{deleteQCOWBackupErr: fmt.Errorf("storage I/O failed")},
		Logger:       testBackupServiceLogger(),
	})

	err := service.DeleteBackup(context.Background(), backupID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deleting QCOW backup file")
}

func TestBackupService_SchedulerTickNoEligibleVMs(t *testing.T) {
	now := time.Now().UTC()
	vmID := "vm-not-today"

	var publishCalled bool
	db := &fakeDB{
		queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
			if strings.Contains(sql, "FROM vms WHERE deleted_at IS NULL AND node_id IS NOT NULL") {
				return &fakeRows{rows: [][]any{vmRow(vmID, "cust-1", strPtr("node-1"), models.VMStatusRunning, nil)}}, nil
			}
			return &fakeRows{}, nil
		},
		queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
			if strings.Contains(sql, "SELECT COUNT(*) > 0 FROM backups") {
				return &fakeRow{values: []any{false}}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
	}
	pub := &testTaskPublisher{publishTaskFunc: func(ctx context.Context, taskType string, payload map[string]any) (string, error) {
		publishCalled = true
		return "task-1", nil
	}}

	service := NewBackupService(BackupServiceConfig{
		BackupRepo:    repository.NewBackupRepository(db),
		SnapshotRepo:  repository.NewBackupRepository(db),
		VMRepo:        repository.NewVMRepository(db),
		TaskPublisher: pub,
		Logger:        testBackupServiceLogger(),
	})

	for service.getVMBackupDay(vmID) == now.Day() {
		vmID += "-x"
	}

	service.runSchedulerTick(context.Background())
	assert.False(t, publishCalled, "scheduler should no-op when no VM assigned to current day")
}

type testTaskPublisher struct {
	publishTaskFunc func(ctx context.Context, taskType string, payload map[string]any) (string, error)
}

func (t *testTaskPublisher) PublishTask(ctx context.Context, taskType string, payload map[string]any) (string, error) {
	if t.publishTaskFunc != nil {
		return t.publishTaskFunc(ctx, taskType, payload)
	}
	return "task-1", nil
}

func strPtr(v string) *string { return &v }

func backupRow(id, vmID string) []any {
	now := time.Now().UTC()
	name := "backup"
	status := models.BackupStatusCreating
	return []any{
		id, vmID, models.BackupMethodFull, &name, models.BackupSourceManual, nil, "ceph", nil,
		nil, nil, nil, nil,
		status, now, nil,
	}
}

func backupRowWithName(id, vmID string, name *string) []any {
	now := time.Now().UTC()
	status := models.BackupStatusCreating
	return []any{
		id, vmID, models.BackupMethodFull, name, models.BackupSourceManual, nil, "ceph", nil,
		nil, nil, nil, nil,
		status, now, nil,
	}
}

func backupRowWithQCOW(id, vmID, filePath, snapName string) []any {
	now := time.Now().UTC()
	name := "backup"
	status := models.BackupStatusCompleted
	return []any{
		id, vmID, models.BackupMethodFull, &name, models.BackupSourceManual, nil, "qcow", nil,
		&filePath, &snapName, nil, nil,
		status, now, nil,
	}
}

func snapshotRowWithID(id, vmID, name, rbdSnapshot string, sizeBytes *int64) []any {
	return snapshotRowWithStorage(id, vmID, name, models.StorageBackendCeph, rbdSnapshot, nil, sizeBytes)
}

func snapshotRowWithStorage(id, vmID, name, storageBackend, rbdSnapshot string, qcowSnapshot *string, sizeBytes *int64) []any {
	now := time.Now().UTC()
	return []any{id, vmID, name, storageBackend, rbdSnapshot, qcowSnapshot, sizeBytes, now}
}

type completedBackupUpdate struct {
	filePath     *string
	snapshotName *string
	storagePath  *string
	sizeBytes    *int64
	rbdSnapshot  *string
}

func backupVMRow(storageBackend string, diskPath *string) []any {
	row := vmRow("vm-1", "cust-1", strPtr("node-1"), models.VMStatusRunning, nil)
	row[22] = storageBackend
	row[23] = diskPath
	return row
}

func stringArg(args []any, index int) *string {
	if value, ok := args[index].(*string); ok {
		return value
	}
	return nil
}

func int64Arg(args []any, index int) *int64 {
	if value, ok := args[index].(*int64); ok {
		return value
	}
	return nil
}
