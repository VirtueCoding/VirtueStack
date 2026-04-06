package storage

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	sharedconfig "github.com/AbuGosok/VirtueStack/internal/shared/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateBuilderDownloadISO_UsesConfiguredMaxISOLimit(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(6*gib, 10))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	builder := &TemplateBuilder{
		logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		maxISOSizeBytes: 5 * gib,
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
				},
			},
		},
	}

	path, cleanup, err := builder.downloadISO(context.Background(), "http://example.com/test.iso")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ISO too large")
	assert.Empty(t, path)
	assert.Nil(t, cleanup)
}

func TestTemplateBuilderResolvedMaxISOSizeBytes_UsesSharedDefault(t *testing.T) {
	builder := &TemplateBuilder{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	assert.Equal(t, sharedconfig.DefaultISOMaxSizeBytes(), builder.resolvedMaxISOSizeBytes())
}

func TestTemplateBuilderGenerateInstallConfig_AutoGeneratesRootPassword(t *testing.T) {
	builder := &TemplateBuilder{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	cfg := BuildConfig{
		TemplateName: "Debian Template",
		OSFamily:     "debian",
		OSVersion:    "12",
	}

	installCfg, err := builder.generateInstallConfig(cfg)
	require.NoError(t, err)

	assert.Contains(t, installCfg, "d-i passwd/root-password password ")
	assert.NotContains(t, installCfg, "d-i passwd/root-password password virtuestack")
	assert.NotContains(t, installCfg, "d-i passwd/root-password-again password virtuestack")
}

func TestTemplateBuilderBuild_CleansTempDirOnInstallConfigError(t *testing.T) {
	tmpRoot, err := os.MkdirTemp("/tmp", "builder-temp-root-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(tmpRoot))
	})
	t.Setenv("TMPDIR", tmpRoot)

	isoFile, err := os.CreateTemp("/tmp", "builder-source-*.iso")
	require.NoError(t, err)
	require.NoError(t, isoFile.Close())
	t.Cleanup(func() {
		require.NoError(t, os.Remove(isoFile.Name()))
	})

	builder := &TemplateBuilder{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	_, err = builder.Build(context.Background(), BuildConfig{
		TemplateName: "Broken Template",
		ISOPath:      isoFile.Name(),
		OSFamily:     "unsupported",
		OSVersion:    "1",
		DiskSizeGB:   10,
		MemoryMB:     1024,
		VCPUs:        1,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported OS family")

	entries, err := os.ReadDir(tmpRoot)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestTemplateBuilderBuild_RejectsAmbiguousISOSource(t *testing.T) {
	testDir := templateBuilderTestDir(t)
	isoPath := filepath.Join(testDir, "source.iso")
	require.NoError(t, os.WriteFile(isoPath, []byte("iso"), 0o600))

	builder := &TemplateBuilder{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	_, err := builder.Build(context.Background(), BuildConfig{
		TemplateName: "Ambiguous ISO Source",
		ISOPath:      isoPath,
		ISOURL:       "http://127.0.0.1/source.iso",
		OSFamily:     "debian",
		OSVersion:    "12",
		DiskSizeGB:   10,
		MemoryMB:     1024,
		VCPUs:        1,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one of iso_path or iso_url must be set")
}

func templateBuilderTestDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp(".", "template-builder-test-")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(dir))
	})

	return dir
}
