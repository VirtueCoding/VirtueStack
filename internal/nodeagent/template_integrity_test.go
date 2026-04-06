package nodeagent

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyTemplateIntegrity(t *testing.T) {
	t.Parallel()

	content := []byte("template-data")
	checksum := templateIntegrityChecksum(content)

	tests := []struct {
		name             string
		expectedSize     int64
		expectedChecksum string
		wantErrContains  string
	}{
		{
			name: "accepts when metadata omitted",
		},
		{
			name:             "accepts matching metadata",
			expectedSize:     int64(len(content)),
			expectedChecksum: strings.ToUpper(checksum),
		},
		{
			name:             "rejects size mismatch",
			expectedSize:     int64(len(content) + 1),
			expectedChecksum: checksum,
			wantErrContains:  "size mismatch",
		},
		{
			name:             "rejects checksum mismatch",
			expectedSize:     int64(len(content)),
			expectedChecksum: strings.Repeat("0", 64),
			wantErrContains:  "checksum mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := templateIntegrityTestDir(t)
			path := filepath.Join(dir, "template.qcow2")
			require.NoError(t, os.WriteFile(path, content, 0o600))

			err := verifyTemplateIntegrity(path, tt.expectedSize, tt.expectedChecksum)
			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestVerifyCachedTemplateIntegrity(t *testing.T) {
	t.Parallel()

	content := []byte("cached-template-data")
	checksum := templateIntegrityChecksum(content)

	tests := []struct {
		name             string
		storageBackend   string
		path             string
		expectedSize     int64
		expectedChecksum string
		wantErrContains  string
	}{
		{
			name:             "qcow cached template enforces source integrity metadata",
			storageBackend:   "qcow",
			expectedSize:     int64(len(content) + 1),
			expectedChecksum: checksum,
			wantErrContains:  "size mismatch",
		},
		{
			name:             "lvm cached template skips source artifact integrity checks",
			storageBackend:   "lvm",
			expectedSize:     int64(len(content) + 1),
			expectedChecksum: checksum,
		},
		{
			name:             "ceph cached template skips source artifact integrity checks",
			storageBackend:   "ceph",
			path:             "vs-images/template-base",
			expectedSize:     int64(len(content) + 1),
			expectedChecksum: checksum,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path
			if path == "" {
				dir := templateIntegrityTestDir(t)
				path = filepath.Join(dir, "template.qcow2")
				require.NoError(t, os.WriteFile(path, content, 0o600))
			}

			err := verifyCachedTemplateIntegrity(tt.storageBackend, path, tt.expectedSize, tt.expectedChecksum)
			if tt.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}

			require.NoError(t, err)
		})
	}
}

func templateIntegrityChecksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func templateIntegrityTestDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp(".", "template-integrity-test-")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(dir))
	})
	return dir
}
