package storage

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLVMTemplateContextCancellation verifies that helpers shelling out to
// lvs/lvremove honor a cancelled context (NA-13). Without exec.CommandContext,
// the subprocesses run to completion regardless of ctx state.
func TestLVMTemplateContextCancellation(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("requires unix shell to install fake lvs/lvremove binaries on PATH")
	}

	binDir := t.TempDir()
	// Fake binary sleeps long enough that an unrespected ctx cancellation is observable.
	fakeScript := "#!/bin/sh\nsleep 3\n"
	for _, name := range []string{"lvs", "lvremove"} {
		path := filepath.Join(binDir, name)
		require.NoError(t, os.WriteFile(path, []byte(fakeScript), 0o755))
	}
	t.Setenv("PATH", binDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := &LVMTemplateManager{
		vgName:   "vgvs",
		thinPool: "thinpool",
		logger:   logger,
	}

	tests := []struct {
		name string
		run  func(ctx context.Context) error
	}{
		{
			name: "hasDependents",
			run: func(ctx context.Context) error {
				_, err := m.hasDependents(ctx, "test-base")
				return err
			},
		},
		{
			name: "removeLV",
			run: func(ctx context.Context) error {
				return m.removeLV(ctx, "test-base", logger)
			},
		},
		{
			name: "getLVSize",
			run: func(ctx context.Context) error {
				_, err := m.getLVSize(ctx, "test-base")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			start := time.Now()
			err := tt.run(ctx)
			elapsed := time.Since(start)

			require.Error(t, err, "cancelled context must produce an error")
			assert.Less(t, elapsed, 1*time.Second,
				"helper must respect cancelled context (got %s)", elapsed)
		})
	}
}
