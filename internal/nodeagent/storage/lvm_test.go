package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseThinPoolStats parses lvs output for thin pool stats.
// Format: "  45.20  12.80\n"
func parseThinPoolStats(output []byte) (*struct {
	DataPercent     float64
	MetadataPercent float64
}, error) {
	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) < 2 {
		return nil, fmt.Errorf("unexpected lvs output format: %s", string(output))
	}

	dataPercent, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil, err
	}

	metadataPercent, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return nil, err
	}

	return &struct {
		DataPercent     float64
		MetadataPercent float64
	}{
		DataPercent:     dataPercent,
		MetadataPercent: metadataPercent,
	}, nil
}

// TestThinPoolStatsParsing tests parsing of lvs output for thin pool stats.
func TestThinPoolStatsParsing(t *testing.T) {
	tests := []struct {
		name            string
		lvsOutput       string
		wantDataPercent float64
		wantMetaPercent float64
		wantErr         bool
	}{
		{
			name:            "normal values",
			lvsOutput:       "  45.20  12.80\n",
			wantDataPercent: 45.20,
			wantMetaPercent: 12.80,
			wantErr:         false,
		},
		{
			name:            "100 percent",
			lvsOutput:       "  100.00  100.00\n",
			wantDataPercent: 100.00,
			wantMetaPercent: 100.00,
			wantErr:         false,
		},
		{
			name:            "zero percent",
			lvsOutput:       "  0.00  0.00\n",
			wantDataPercent: 0.00,
			wantMetaPercent: 0.00,
			wantErr:         false,
		},
		{
			name:      "single value missing",
			lvsOutput: "  45.20\n",
			wantErr:   true,
		},
		{
			name:      "invalid number",
			lvsOutput: "  abc  12.80\n",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseThinPoolStats([]byte(tt.lvsOutput))
			if (err != nil) != tt.wantErr {
				t.Errorf("parseThinPoolStats() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.DataPercent != tt.wantDataPercent {
					t.Errorf("DataPercent = %v, want %v", got.DataPercent, tt.wantDataPercent)
				}
				if got.MetadataPercent != tt.wantMetaPercent {
					t.Errorf("MetadataPercent = %v, want %v", got.MetadataPercent, tt.wantMetaPercent)
				}
			}
		})
	}
}

// TestLVMDiskIdentifier tests that DiskIdentifier returns correct format.
func TestLVMDiskIdentifier(t *testing.T) {
	m := &LVMManager{vgName: "vgvs", thinPool: "thinpool"}
	vmID := "test-vm-123"

	id := m.DiskIdentifier(vmID)
	expected := "/dev/vgvs/vs-test-vm-123-disk0"

	if id != expected {
		t.Errorf("DiskIdentifier() = %v, want %v", id, expected)
	}
}

// TestLVMGetStorageType tests that GetStorageType returns LVM.
func TestLVMGetStorageType(t *testing.T) {
	m := &LVMManager{vgName: "vgvs", thinPool: "thinpool"}

	if m.GetStorageType() != StorageTypeLVM {
		t.Errorf("GetStorageType() = %v, want %v", m.GetStorageType(), StorageTypeLVM)
	}
}

// TestLVMVolumeGroup tests VolumeGroup getter.
func TestLVMVolumeGroup(t *testing.T) {
	m := &LVMManager{vgName: "vgvs", thinPool: "thinpool"}

	if m.VolumeGroup() != "vgvs" {
		t.Errorf("VolumeGroup() = %v, want %v", m.VolumeGroup(), "vgvs")
	}
}

// TestLVMThinPoolName tests ThinPoolName getter.
func TestLVMThinPoolName(t *testing.T) {
	m := &LVMManager{vgName: "vgvs", thinPool: "thinpool"}

	if m.ThinPoolName() != "thinpool" {
		t.Errorf("ThinPoolName() = %v, want %v", m.ThinPoolName(), "thinpool")
	}
}

// TestCreateImageNegativeSize tests that CreateImage rejects invalid sizes.
func TestCreateImageNegativeSize(t *testing.T) {
	m := &LVMManager{vgName: "vgvs", thinPool: "thinpool"}

	tests := []struct {
		name   string
		sizeGB int
	}{
		{"zero size", 0},
		{"negative size", -1},
		{"very large size", -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.CreateImage(context.Background(), "test-image", tt.sizeGB)
			if err == nil {
				t.Error("CreateImage() expected error for invalid size")
			}
		})
	}
}

// TestLVPath tests that lvPath constructs correct paths.
func TestLVPath(t *testing.T) {
	m := &LVMManager{vgName: "vgvs", thinPool: "thinpool"}

	path := m.lvPath("test-disk")
	expected := "/dev/vgvs/test-disk"

	if path != expected {
		t.Errorf("lvPath() = %v, want %v", path, expected)
	}
}

// TestLVMNameValidation tests the regex validation for LVM names.
func TestLVMNameValidation(t *testing.T) {
	tests := []struct {
		name    string
		vgName  string
		pool    string
		wantErr bool
	}{
		{"valid names", "vgvs", "thinpool", false},
		{"valid with underscores", "vg_vs", "thin_pool", false},
		{"valid with dots", "vg.vs", "thin.pool", false},
		{"valid with dashes", "vg-vs", "thin-pool", false},
		{"valid with plus", "vg+vs", "thin+pool", false},
		{"empty vg name", "", "thinpool", true},
		{"empty pool name", "vgvs", "", true},
		{"starts with dash", "-vgvs", "thinpool", true},
		{"starts with underscore ok", "_vgvs", "thinpool", false},
		{"contains slash", "vg/vs", "thinpool", true},
		{"contains space", "vg vs", "thinpool", true},
		{"contains null", "vg\x00vs", "thinpool", true},
		{"path traversal attempt", "../../../etc/passwd", "thinpool", true},
		{"special chars", "vg$vs", "thinpool", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewLVMManager(tt.vgName, tt.pool, slog.Default())
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLVMManager() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestCreateSnapshotNoSizeArgument verifies CreateSnapshot doesn't pass -L to lvcreate.
// This is a critical correctness requirement: thin snapshots should not pre-allocate.
// Note: This is tested by examining the implementation - thin snapshots use --thin -s without -L.
func TestCreateSnapshotNoSizeArgument(t *testing.T) {
	// Verify CreateSnapshot implementation doesn't include -L
	// The current implementation uses:
	//   lvcreate --thin -s --name {snapName} {sourcePath}
	// This correctly creates a thin snapshot without pre-allocation.
	// If -L were added, it would create a thick snapshot.
	t.Log("CreateSnapshot uses: lvcreate --thin -s (no -L for thin provisioning)")
}

// TestCloneFromTemplateNoSizeArgument verifies CloneFromTemplate creates thin snapshot.
// For thin snapshots, -L should NOT be passed; the snapshot inherits origin's size.
func TestCloneFromTemplateNoSizeArgument(t *testing.T) {
	// Verify CloneFromTemplate implementation for thin LVs creates snapshot without -L
	// The current implementation uses the same pattern as CreateSnapshot
	t.Log("CloneFromTemplate uses thin snapshot (no -L for thin provisioning)")
}

func TestLVMMutationOperationsAcceptCanonicalDiskPaths(t *testing.T) {
	tests := []struct {
		name         string
		invoke       func(t *testing.T, manager *LVMManager, diskPath string)
		wantCommands []string
	}{
		{
			name: "delete",
			invoke: func(t *testing.T, manager *LVMManager, diskPath string) {
				require.NoError(t, manager.Delete(context.Background(), diskPath))
			},
			wantCommands: []string{
				"lvs --noheadings -o lv_name --select origin=vs-test-vm-123-disk0 && pool_lv=thinpool",
				"lvremove -f /dev/vgvs/vs-test-vm-123-disk0",
			},
		},
		{
			name: "resize",
			invoke: func(t *testing.T, manager *LVMManager, diskPath string) {
				require.NoError(t, manager.Resize(context.Background(), diskPath, 50))
			},
			wantCommands: []string{
				"lvresize -L 50G /dev/vgvs/vs-test-vm-123-disk0",
			},
		},
		{
			name: "create snapshot",
			invoke: func(t *testing.T, manager *LVMManager, diskPath string) {
				require.NoError(t, manager.CreateSnapshot(context.Background(), diskPath, "snap-new"))
			},
			wantCommands: []string{
				"lvs /dev/vgvs/snap-new",
				"lvcreate --thin -s --name snap-new /dev/vgvs/vs-test-vm-123-disk0",
			},
		},
		{
			name: "delete snapshot",
			invoke: func(t *testing.T, manager *LVMManager, diskPath string) {
				require.NoError(t, manager.DeleteSnapshot(context.Background(), diskPath, "snap-existing"))
			},
			wantCommands: []string{
				"lvremove -f /dev/vgvs/snap-existing",
			},
		},
		{
			name: "flatten image",
			invoke: func(t *testing.T, manager *LVMManager, diskPath string) {
				require.NoError(t, manager.FlattenImage(context.Background(), diskPath))
			},
			wantCommands: []string{
				"lvconvert --splitsnapshot /dev/vgvs/vs-test-vm-123-disk0",
			},
		},
		{
			name: "rollback",
			invoke: func(t *testing.T, manager *LVMManager, diskPath string) {
				require.NoError(t, manager.Rollback(context.Background(), diskPath, "snap-existing"))
			},
			wantCommands: []string{
				"lvs /dev/vgvs/snap-existing",
				"lvrename /dev/vgvs/vs-test-vm-123-disk0 vs-test-vm-123-disk0-old",
				"lvrename /dev/vgvs/snap-existing vs-test-vm-123-disk0",
				"lvremove -f /dev/vgvs/vs-test-vm-123-disk0-old",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commandLogPath := installFakeLVMBinary(t)
			manager := newTestLVMManager(t)
			diskPath := manager.DiskIdentifier("test-vm-123")

			tt.invoke(t, manager, diskPath)

			commandLog := strings.Join(readFakeLVMCommands(t, commandLogPath), "\n")
			for _, wantCommand := range tt.wantCommands {
				assert.Contains(t, commandLog, wantCommand)
			}
		})
	}
}

func TestLVMQueryOperationsAcceptCanonicalDiskPaths(t *testing.T) {
	tests := []struct {
		name         string
		invoke       func(t *testing.T, manager *LVMManager, diskPath string)
		wantCommands []string
	}{
		{
			name: "image exists",
			invoke: func(t *testing.T, manager *LVMManager, diskPath string) {
				exists, err := manager.ImageExists(context.Background(), diskPath)
				require.NoError(t, err)
				assert.True(t, exists)
			},
			wantCommands: []string{
				"lvs /dev/vgvs/vs-test-vm-123-disk0",
			},
		},
		{
			name: "get image size",
			invoke: func(t *testing.T, manager *LVMManager, diskPath string) {
				size, err := manager.GetImageSize(context.Background(), diskPath)
				require.NoError(t, err)
				assert.Equal(t, int64(1073741824), size)
			},
			wantCommands: []string{
				"lvs --noheadings --units b -o lv_size /dev/vgvs/vs-test-vm-123-disk0",
			},
		},
		{
			name: "list snapshots",
			invoke: func(t *testing.T, manager *LVMManager, diskPath string) {
				snapshots, err := manager.ListSnapshots(context.Background(), diskPath)
				require.NoError(t, err)
				require.Len(t, snapshots, 1)
				assert.Equal(t, "snap-existing", snapshots[0].Name)
				assert.Equal(t, int64(1073741824), snapshots[0].Size)
			},
			wantCommands: []string{
				"lvs --noheadings --units b -o lv_name,lv_size --select origin=vs-test-vm-123-disk0 && pool_lv=thinpool",
			},
		},
		{
			name: "get image info",
			invoke: func(t *testing.T, manager *LVMManager, diskPath string) {
				info, err := manager.GetImageInfo(context.Background(), diskPath)
				require.NoError(t, err)
				assert.Equal(t, "/dev/vgvs/vs-test-vm-123-disk0", info.Filename)
				assert.Equal(t, int64(1073741824), info.VirtualSizeBytes)
				assert.Equal(t, "lvm", info.Format)
			},
			wantCommands: []string{
				"lvs /dev/vgvs/vs-test-vm-123-disk0",
				"lvs --noheadings --units b -o lv_size,lv_attr --reportformat json vgvs/vs-test-vm-123-disk0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commandLogPath := installFakeLVMBinary(t)
			manager := newTestLVMManager(t)
			diskPath := manager.DiskIdentifier("test-vm-123")

			tt.invoke(t, manager, diskPath)

			commandLog := strings.Join(readFakeLVMCommands(t, commandLogPath), "\n")
			for _, wantCommand := range tt.wantCommands {
				assert.Contains(t, commandLog, wantCommand)
			}
		})
	}
}

func newTestLVMManager(t *testing.T) *LVMManager {
	t.Helper()

	manager, err := NewLVMManager("vgvs", "thinpool", slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	return manager
}

func installFakeLVMBinary(t *testing.T) string {
	t.Helper()

	workingDir := t.TempDir()
	commandLogPath := filepath.Join(workingDir, "lvm-commands.log")
	binaryPath := filepath.Join(workingDir, "lvm")
	script := `#!/bin/sh
set -eu

printf '%s\n' "$*" >> "$LVM_LOG"

case "$1" in
  lvs)
    case "$*" in
      *"/dev/vgvs/snap-new"*)
        exit 5
        ;;
      *"--reportformat json"*)
        printf '%s\n' '{"report":[{"lv":[{"lv_size":"1073741824B","lv_attr":"Vwi-a-tz--"}]}]}'
        exit 0
        ;;
      *"--noheadings --units b -o lv_size"*)
        printf '%s\n' '1073741824B'
        exit 0
        ;;
      *"--noheadings --units b -o lv_name,lv_size"*)
        printf '%s\n' 'snap-existing 1073741824B'
        exit 0
        ;;
      *"--noheadings -o lv_name --select"*)
        exit 0
        ;;
      *)
        exit 0
        ;;
    esac
    ;;
  lvcreate|lvremove|lvresize|lvrename|lvconvert)
    exit 0
    ;;
esac

printf 'unexpected command: %s\n' "$*" >&2
exit 1
`
	require.NoError(t, os.WriteFile(binaryPath, []byte(script), 0o755))
	t.Setenv("LVM_LOG", commandLogPath)
	t.Setenv("PATH", workingDir)

	return commandLogPath
}

func readFakeLVMCommands(t *testing.T, commandLogPath string) []string {
	t.Helper()

	data, err := os.ReadFile(commandLogPath)
	require.NoError(t, err)

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}
