package downloadutil

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyFileIntegrity(t *testing.T) {
	content := []byte("template-data")
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])

	tests := []struct {
		name             string
		expectedSize     int64
		expectedChecksum string
		wantErrContains  string
	}{
		{
			name:             "accepts matching size and checksum",
			expectedSize:     int64(len(content)),
			expectedChecksum: strings.ToUpper(checksum),
		},
		{
			name: "accepts when expectations are omitted",
		},
		{
			name:            "rejects size mismatch",
			expectedSize:    int64(len(content) + 1),
			wantErrContains: "size mismatch",
		},
		{
			name:             "rejects checksum mismatch",
			expectedChecksum: strings.Repeat("0", 64),
			wantErrContains:  "checksum mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(t.TempDir(), "template.qcow2")
			if err := os.WriteFile(filePath, content, 0o600); err != nil {
				t.Fatalf("os.WriteFile() error = %v", err)
			}

			err := VerifyFileIntegrity(filePath, tt.expectedSize, tt.expectedChecksum)
			if tt.wantErrContains != "" {
				if err == nil {
					t.Fatalf("VerifyFileIntegrity() expected error containing %q", tt.wantErrContains)
				}
				if !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Fatalf("VerifyFileIntegrity() error = %v, want substring %q", err, tt.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("VerifyFileIntegrity() error = %v", err)
			}
		})
	}
}
