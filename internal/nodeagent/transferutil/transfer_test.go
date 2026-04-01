package transferutil

import (
	"errors"
	"testing"
)

func TestResolveLVMSourcePath(t *testing.T) {
	tests := []struct {
		name           string
		sourceDiskPath string
		snapshotName   string
		requestedVG    string
		configuredVG   string
		wantPath       string
		wantErr        bool
	}{
		{
			name:           "uses canonical configured vg path",
			sourceDiskPath: "/dev/vg0/vs-vm-disk0",
			requestedVG:    "vg0",
			configuredVG:   "vg0",
			wantPath:       "/dev/vg0/vs-vm-disk0",
		},
		{
			name:         "uses snapshot within configured vg",
			snapshotName: "snap-01",
			requestedVG:  "vg0",
			configuredVG: "vg0",
			wantPath:     "/dev/vg0/snap-01",
		},
		{
			name:           "rejects other volume groups",
			sourceDiskPath: "/dev/vg1/vs-vm-disk0",
			requestedVG:    "vg1",
			configuredVG:   "vg0",
			wantErr:        true,
		},
		{
			name:           "rejects nested path escape",
			sourceDiskPath: "/dev/vg0/nested/vs-vm-disk0",
			requestedVG:    "vg0",
			configuredVG:   "vg0",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveLVMSourcePath(tt.sourceDiskPath, tt.snapshotName, tt.requestedVG, tt.configuredVG)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ResolveLVMSourcePath() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveLVMSourcePath() error = %v", err)
			}
			if got != tt.wantPath {
				t.Fatalf("ResolveLVMSourcePath() = %q, want %q", got, tt.wantPath)
			}
		})
	}
}

func TestResolveReceiveTarget(t *testing.T) {
	tests := []struct {
		name              string
		storageBackend    string
		targetPath        string
		storagePath       string
		configuredVG      string
		configuredThin    string
		requestedVG       string
		requestedThin     string
		wantOpenPath      string
		wantCreateImageID string
		wantErr           bool
	}{
		{
			name:           "qcow stays within storage path",
			storageBackend: "qcow",
			targetPath:     "/var/lib/virtuestack/vm-1-disk0.qcow2",
			storagePath:    "/var/lib/virtuestack",
			wantOpenPath:   "/var/lib/virtuestack/vm-1-disk0.qcow2",
		},
		{
			name:           "rejects qcow path escape",
			storageBackend: "qcow",
			targetPath:     "/etc/passwd",
			storagePath:    "/var/lib/virtuestack",
			wantErr:        true,
		},
		{
			name:              "lvm uses configured target path",
			storageBackend:    "lvm",
			targetPath:        "/dev/vg0/vs-vm-disk0",
			configuredVG:      "vg0",
			configuredThin:    "thin0",
			requestedVG:       "vg0",
			requestedThin:     "thin0",
			wantOpenPath:      "/dev/vg0/vs-vm-disk0",
			wantCreateImageID: "vs-vm-disk0",
		},
		{
			name:           "rejects mismatched thin pool",
			storageBackend: "lvm",
			targetPath:     "/dev/vg0/vs-vm-disk0",
			configuredVG:   "vg0",
			configuredThin: "thin0",
			requestedVG:    "vg0",
			requestedThin:  "thin1",
			wantErr:        true,
		},
		{
			name:           "rejects non canonical lvm path",
			storageBackend: "lvm",
			targetPath:     "/dev/vg0/../../etc/passwd",
			configuredVG:   "vg0",
			configuredThin: "thin0",
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveReceiveTarget(tt.storageBackend, tt.targetPath, tt.storagePath, tt.configuredVG, tt.configuredThin, tt.requestedVG, tt.requestedThin)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ResolveReceiveTarget() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveReceiveTarget() error = %v", err)
			}
			if got.OpenPath != tt.wantOpenPath {
				t.Fatalf("ResolveReceiveTarget().OpenPath = %q, want %q", got.OpenPath, tt.wantOpenPath)
			}
			if got.CreateImageID != tt.wantCreateImageID {
				t.Fatalf("ResolveReceiveTarget().CreateImageID = %q, want %q", got.CreateImageID, tt.wantCreateImageID)
			}
		})
	}
}

func TestReceiveTracker(t *testing.T) {
	t.Run("accepts sequential chunks and finalizes exact total", func(t *testing.T) {
		tracker, err := NewReceiveTracker(5)
		if err != nil {
			t.Fatalf("NewReceiveTracker() error = %v", err)
		}
		if err := tracker.Accept(0, 2); err != nil {
			t.Fatalf("Accept(first) error = %v", err)
		}
		if err := tracker.Accept(2, 3); err != nil {
			t.Fatalf("Accept(second) error = %v", err)
		}
		if err := tracker.Finalize(); err != nil {
			t.Fatalf("Finalize() error = %v", err)
		}
		if tracker.BytesReceived() != 5 {
			t.Fatalf("BytesReceived() = %d, want 5", tracker.BytesReceived())
		}
	})

	t.Run("rejects negative offset", func(t *testing.T) {
		tracker, err := NewReceiveTracker(5)
		if err != nil {
			t.Fatalf("NewReceiveTracker() error = %v", err)
		}
		err = tracker.Accept(-1, 1)
		if !errors.Is(err, ErrInvalidOffset) {
			t.Fatalf("Accept() error = %v, want ErrInvalidOffset", err)
		}
	})

	t.Run("rejects offset gaps before write", func(t *testing.T) {
		tracker, err := NewReceiveTracker(5)
		if err != nil {
			t.Fatalf("NewReceiveTracker() error = %v", err)
		}
		if err := tracker.Accept(0, 2); err != nil {
			t.Fatalf("Accept(first) error = %v", err)
		}
		err = tracker.Accept(4, 1)
		if !errors.Is(err, ErrInvalidOffset) {
			t.Fatalf("Accept() error = %v, want ErrInvalidOffset", err)
		}
	})

	t.Run("rejects oversized transfers", func(t *testing.T) {
		tracker, err := NewReceiveTracker(5)
		if err != nil {
			t.Fatalf("NewReceiveTracker() error = %v", err)
		}
		err = tracker.Accept(0, 6)
		if !errors.Is(err, ErrTransferSize) {
			t.Fatalf("Accept() error = %v, want ErrTransferSize", err)
		}
	})

	t.Run("rejects truncated transfers", func(t *testing.T) {
		tracker, err := NewReceiveTracker(5)
		if err != nil {
			t.Fatalf("NewReceiveTracker() error = %v", err)
		}
		if err := tracker.Accept(0, 4); err != nil {
			t.Fatalf("Accept() error = %v", err)
		}
		err = tracker.Finalize()
		if !errors.Is(err, ErrTransferSize) {
			t.Fatalf("Finalize() error = %v, want ErrTransferSize", err)
		}
	})
}

func TestValidateTransferredBytes(t *testing.T) {
	tests := []struct {
		name     string
		expected int64
		actual   int64
		wantErr  bool
	}{
		{name: "exact transfer", expected: 10, actual: 10},
		{name: "truncated transfer", expected: 10, actual: 9, wantErr: true},
		{name: "oversized transfer", expected: 10, actual: 11, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransferredBytes(tt.expected, tt.actual)
			if tt.wantErr && !errors.Is(err, ErrTransferSize) {
				t.Fatalf("ValidateTransferredBytes() error = %v, want ErrTransferSize", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateTransferredBytes() error = %v", err)
			}
		})
	}
}
