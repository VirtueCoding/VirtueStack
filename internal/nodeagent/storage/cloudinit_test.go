package storage

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCloudInitHostname(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hostname string
		wantErr  bool
	}{
		{
			name:     "accepts simple hostname",
			hostname: "valid-host",
		},
		{
			name:     "rejects empty hostname",
			hostname: "",
			wantErr:  true,
		},
		{
			name:     "rejects line breaks",
			hostname: "bad\nhost",
			wantErr:  true,
		},
		{
			name:     "rejects yaml-ish colon",
			hostname: "bad: host",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateCloudInitHostname(tt.hostname)
			if tt.wantErr {
				if err == nil {
					t.Fatal("validateCloudInitHostname() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("validateCloudInitHostname() error = %v", err)
			}
		})
	}
}

func TestCloudInitGenerateRejectsInvalidHostname(t *testing.T) {
	t.Parallel()

	generator := NewCloudInitGenerator(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := generator.Generate(context.Background(), &CloudInitConfig{
		VMID:     "vm-123",
		Hostname: "bad\nhost",
	})
	if err == nil {
		t.Fatal("Generate() expected error")
	}
	if !strings.Contains(err.Error(), "invalid hostname") {
		t.Fatalf("Generate() error = %v, want invalid hostname", err)
	}
}

func TestCloudInitWriteUserDataRejectsSSHKeyWithTrailingContent(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	generator := NewCloudInitGenerator(tmpDir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cfg := &CloudInitConfig{
		VMID:     "vm-123",
		Hostname: "valid-host",
		SSHPublicKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAWWs+jPqZUy7+5i7EnyzhAu7U/414vQMxrUT87xfg39 hiron@computer\nwrite_files:\n  - path: /tmp/pwned",
		},
	}

	err := generator.writeUserData(tmpDir, cfg)
	if err == nil {
		t.Fatal("writeUserData() expected error")
	}
	if !strings.Contains(err.Error(), "invalid SSH public key") {
		t.Fatalf("writeUserData() error = %v, want invalid SSH public key", err)
	}

	_, statErr := os.Stat(filepath.Join(tmpDir, "user-data"))
	if !os.IsNotExist(statErr) {
		t.Fatalf("expected user-data to not be written, got err=%v", statErr)
	}
}
