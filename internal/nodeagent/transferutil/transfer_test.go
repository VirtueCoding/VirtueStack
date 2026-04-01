package transferutil

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
)

var (
	errBoom       = errors.New("boom")
	errSeekFailed = errors.New("seek failed")
	errOpenFailed = errors.New("open failed")
	errSendFailed = errors.New("send failed")
	errReadFailed = errors.New("read failed")
	errWaitFailed = errors.New("wait failed")
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

func TestValidateLVMImageCapacity(t *testing.T) {
	tests := []struct {
		name       string
		totalBytes int64
		sizeGB     int64
		wantErr    bool
	}{
		{name: "fits within declared image size", totalBytes: bytesPerGiB, sizeGB: 2},
		{name: "rejects oversized transfer", totalBytes: 2*bytesPerGiB + 1, sizeGB: 2, wantErr: true},
		{name: "rejects non positive size", totalBytes: bytesPerGiB, sizeGB: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLVMImageCapacity(tt.totalBytes, tt.sizeGB)
			if tt.wantErr {
				if !errors.Is(err, ErrTransferSize) {
					t.Fatalf("ValidateLVMImageCapacity() error = %v, want ErrTransferSize", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateLVMImageCapacity() error = %v", err)
			}
		})
	}
}

func TestWriteFull(t *testing.T) {
	tests := []struct {
		name    string
		writer  *stubWriteSeeker
		data    []byte
		wantErr error
	}{
		{
			name:   "writes full chunk",
			writer: &stubWriteSeeker{writeN: 4},
			data:   []byte("test"),
		},
		{
			name:    "rejects short write",
			writer:  &stubWriteSeeker{writeN: 2},
			data:    []byte("test"),
			wantErr: io.ErrShortWrite,
		},
		{
			name:    "returns write error",
			writer:  &stubWriteSeeker{writeErr: errBoom},
			data:    []byte("test"),
			wantErr: errBoom,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WriteFull(tt.writer, tt.data)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("WriteFull() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("WriteFull() error = %v", err)
			}
		})
	}
}

func TestSeekAndWriteFull(t *testing.T) {
	tests := []struct {
		name       string
		writer     *stubWriteSeeker
		offset     int64
		data       []byte
		wantSeek   int64
		wantErr    error
		wantWrites int
	}{
		{
			name:       "seeks then writes full chunk",
			writer:     &stubWriteSeeker{writeN: 4},
			offset:     8,
			data:       []byte("test"),
			wantSeek:   8,
			wantWrites: 1,
		},
		{
			name:       "rejects short write after seek",
			writer:     &stubWriteSeeker{writeN: 2},
			offset:     3,
			data:       []byte("test"),
			wantSeek:   3,
			wantErr:    io.ErrShortWrite,
			wantWrites: 1,
		},
		{
			name:     "returns seek error",
			writer:   &stubWriteSeeker{seekErr: errSeekFailed},
			offset:   4,
			data:     []byte("test"),
			wantSeek: 4,
			wantErr:  errSeekFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SeekAndWriteFull(tt.writer, tt.offset, tt.data)
			if tt.writer.seekOffset != tt.wantSeek {
				t.Fatalf("SeekAndWriteFull() seek offset = %d, want %d", tt.writer.seekOffset, tt.wantSeek)
			}
			if tt.writer.writeCalls != tt.wantWrites {
				t.Fatalf("SeekAndWriteFull() write calls = %d, want %d", tt.writer.writeCalls, tt.wantWrites)
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("SeekAndWriteFull() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SeekAndWriteFull() error = %v", err)
			}
		})
	}
}

func TestOpenLVMReceiveTarget(t *testing.T) {
	tests := []struct {
		name             string
		openErr          error
		deleteErr        error
		wantErr          bool
		wantDeleteOnOpen bool
		wantRollback     bool
	}{
		{
			name:             "open failure deletes created image",
			openErr:          errOpenFailed,
			wantErr:          true,
			wantDeleteOnOpen: true,
		},
		{
			name:         "successful open returns rollback cleanup",
			wantRollback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var created []string
			var deleted []string
			var deleteCtx context.Context

			file, rollback, err := OpenLVMReceiveTarget(
				context.Background(),
				"vs-vm-disk0",
				10,
				"/dev/vg0/vs-vm-disk0",
				func(_ context.Context, imageID string, sizeGB int) error {
					created = append(created, imageID)
					if sizeGB != 10 {
						t.Fatalf("CreateImage size = %d, want 10", sizeGB)
					}
					return nil
				},
				func(_ string) (*os.File, error) {
					if tt.openErr != nil {
						return nil, tt.openErr
					}
					return os.OpenFile("/dev/null", os.O_WRONLY, 0)
				},
				func(deleteImageCtx context.Context, imageID string) error {
					deleteCtx = deleteImageCtx
					deleted = append(deleted, imageID)
					return tt.deleteErr
				},
			)

			if len(created) != 1 || created[0] != "vs-vm-disk0" {
				t.Fatalf("CreateImage calls = %v, want [vs-vm-disk0]", created)
			}
			if tt.wantErr {
				if err == nil {
					t.Fatal("OpenLVMReceiveTarget() expected error")
				}
				if file != nil {
					t.Fatal("OpenLVMReceiveTarget() file should be nil on error")
				}
				if tt.wantDeleteOnOpen && len(deleted) != 1 {
					t.Fatalf("DeleteImage calls = %v, want one cleanup call", deleted)
				}
				return
			}
			if err != nil {
				t.Fatalf("OpenLVMReceiveTarget() error = %v", err)
			}
			if file == nil {
				t.Fatal("OpenLVMReceiveTarget() returned nil file")
			}
			if closeErr := file.Close(); closeErr != nil {
				t.Fatalf("Close() error = %v", closeErr)
			}
			if !tt.wantRollback {
				if rollback != nil {
					t.Fatal("OpenLVMReceiveTarget() rollback should be nil")
				}
				return
			}
			if rollback == nil {
				t.Fatal("OpenLVMReceiveTarget() rollback should not be nil")
			}
			if err := rollback(); err != nil {
				t.Fatalf("rollback() error = %v", err)
			}
			if len(deleted) != 1 || deleted[0] != "vs-vm-disk0" {
				t.Fatalf("DeleteImage calls = %v, want [vs-vm-disk0]", deleted)
			}
			if deleteCtx == nil {
				t.Fatal("DeleteImage context was not captured")
			}
			if deleteCtx.Err() != nil {
				t.Fatalf("DeleteImage context should not be canceled, got %v", deleteCtx.Err())
			}
		})
	}
}

func TestOpenLVMReceiveTarget_OpenFailureUsesDetachedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var deleteCtx context.Context

	file, rollback, err := OpenLVMReceiveTarget(
		ctx,
		"vs-vm-disk0",
		10,
		"/dev/vg0/vs-vm-disk0",
		func(context.Context, string, int) error { return nil },
		func(_ string) (*os.File, error) {
			return nil, errOpenFailed
		},
		func(ctx context.Context, _ string) error {
			deleteCtx = ctx
			return nil
		},
	)
	if err == nil {
		t.Fatal("OpenLVMReceiveTarget() expected error")
	}
	if file != nil {
		t.Fatal("OpenLVMReceiveTarget() file should be nil on error")
	}
	if rollback != nil {
		t.Fatal("OpenLVMReceiveTarget() rollback should be nil on error")
	}
	if deleteCtx == nil {
		t.Fatal("DeleteImage context was not captured")
	}
	if deleteCtx.Err() != nil {
		t.Fatalf("DeleteImage context should be detached from cancellation, got %v", deleteCtx.Err())
	}
}

func TestOpenLVMReceiveTarget_RollbackUsesDetachedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var deleteCtx context.Context

	file, rollback, err := OpenLVMReceiveTarget(
		ctx,
		"vs-vm-disk0",
		10,
		"/dev/vg0/vs-vm-disk0",
		func(context.Context, string, int) error { return nil },
		func(_ string) (*os.File, error) {
			return os.OpenFile("/dev/null", os.O_WRONLY, 0)
		},
		func(ctx context.Context, _ string) error {
			deleteCtx = ctx
			return nil
		},
	)
	if err != nil {
		t.Fatalf("OpenLVMReceiveTarget() error = %v", err)
	}
	if file == nil {
		t.Fatal("OpenLVMReceiveTarget() returned nil file")
	}
	if closeErr := file.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}
	if rollback == nil {
		t.Fatal("OpenLVMReceiveTarget() rollback should not be nil")
	}
	if err := rollback(); err != nil {
		t.Fatalf("rollback() error = %v", err)
	}
	if deleteCtx == nil {
		t.Fatal("DeleteImage context was not captured")
	}
	if deleteCtx.Err() != nil {
		t.Fatalf("DeleteImage context should be detached from cancellation, got %v", deleteCtx.Err())
	}
}

func TestStreamProcessOutput(t *testing.T) {
	tests := []struct {
		name             string
		reader           io.Reader
		totalSize        int64
		sendErr          error
		waitErr          error
		wantErr          bool
		wantTerminate    bool
		wantWaitCalls    int
		wantBytesSent    int64
		wantTerminateErr bool
	}{
		{
			name:          "success waits after streaming",
			reader:        &scriptedReader{steps: []readStep{{data: []byte("hello")}, {err: io.EOF}}},
			totalSize:     5,
			wantWaitCalls: 1,
			wantBytesSent: 5,
		},
		{
			name:          "send failure terminates and waits",
			reader:        &scriptedReader{steps: []readStep{{data: []byte("hello")}}},
			totalSize:     5,
			sendErr:       errSendFailed,
			wantErr:       true,
			wantTerminate: true,
			wantWaitCalls: 1,
		},
		{
			name:          "read failure terminates and waits",
			reader:        &scriptedReader{steps: []readStep{{data: []byte("he"), err: errReadFailed}}},
			totalSize:     5,
			wantErr:       true,
			wantTerminate: true,
			wantWaitCalls: 1,
			wantBytesSent: 2,
		},
		{
			name:          "wait failure bubbles up on success path",
			reader:        &scriptedReader{steps: []readStep{{data: []byte("hello")}, {err: io.EOF}}},
			totalSize:     5,
			waitErr:       errWaitFailed,
			wantErr:       true,
			wantWaitCalls: 1,
			wantBytesSent: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent [][]byte
			terminated := false
			waitCalls := 0

			bytesSent, err := StreamProcessOutput(
				tt.reader,
				tt.totalSize,
				func(_ int64, _ int64, data []byte) error {
					sent = append(sent, append([]byte(nil), data...))
					return tt.sendErr
				},
				func() error {
					terminated = true
					return nil
				},
				func() error {
					waitCalls++
					return tt.waitErr
				},
			)

			if bytesSent != tt.wantBytesSent {
				t.Fatalf("StreamProcessOutput() bytesSent = %d, want %d", bytesSent, tt.wantBytesSent)
			}
			if terminated != tt.wantTerminate {
				t.Fatalf("StreamProcessOutput() terminated = %t, want %t", terminated, tt.wantTerminate)
			}
			if waitCalls != tt.wantWaitCalls {
				t.Fatalf("StreamProcessOutput() wait calls = %d, want %d", waitCalls, tt.wantWaitCalls)
			}
			if tt.wantErr {
				if err == nil {
					t.Fatal("StreamProcessOutput() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("StreamProcessOutput() error = %v", err)
			}
			if len(sent) != 1 || string(sent[0]) != "hello" {
				t.Fatalf("StreamProcessOutput() sent = %q, want [hello]", sent)
			}
		})
	}
}

type stubWriteSeeker struct {
	writeN     int
	writeErr   error
	seekErr    error
	seekOffset int64
	writeCalls int
}

func (s *stubWriteSeeker) Write(_ []byte) (int, error) {
	s.writeCalls++
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	return s.writeN, nil
}

func (s *stubWriteSeeker) Seek(offset int64, _ int) (int64, error) {
	s.seekOffset = offset
	if s.seekErr != nil {
		return 0, s.seekErr
	}
	return offset, nil
}

type readStep struct {
	data []byte
	err  error
}

type scriptedReader struct {
	steps []readStep
	index int
}

func (r *scriptedReader) Read(p []byte) (int, error) {
	if r.index >= len(r.steps) {
		return 0, io.EOF
	}
	step := r.steps[r.index]
	r.index++
	n := copy(p, step.data)
	return n, step.err
}
