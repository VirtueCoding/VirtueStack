package nodeagent

import (
	"strings"
	"testing"
)

// TestValidatePath covers the early-rejection contract of validatePath, including
// the NA-4 fix that empty allowedPrefix must be refused (previously any absolute
// path passed when allowedPrefix == "").
func TestValidatePath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		allowedPrefix string
		wantErr       bool
		errSubstr     string
	}{
		{
			name:          "empty allowed prefix is rejected",
			path:          "/anything",
			allowedPrefix: "",
			wantErr:       true,
			errSubstr:     "allowed",
		},
		{
			name:          "empty allowed prefix rejected even for benign relative path",
			path:          "foo",
			allowedPrefix: "",
			wantErr:       true,
		},
		{
			name:          "empty path rejected",
			path:          "",
			allowedPrefix: "/var/lib/virtuestack",
			wantErr:       true,
		},
		{
			name:          "path inside allowed prefix",
			path:          "/var/lib/virtuestack/disks/vm.qcow2",
			allowedPrefix: "/var/lib/virtuestack",
			wantErr:       false,
		},
		{
			name:          "traversal escapes prefix",
			path:          "/var/lib/virtuestack/../etc/shadow",
			allowedPrefix: "/var/lib/virtuestack",
			wantErr:       true,
		},
		{
			name:          "absolute path outside prefix",
			path:          "/etc/shadow",
			allowedPrefix: "/var/lib/virtuestack",
			wantErr:       true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validatePath(tc.path, tc.allowedPrefix)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validatePath(%q, %q) returned nil error, want error",
						tc.path, tc.allowedPrefix)
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("validatePath error %q does not contain %q",
						err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validatePath(%q, %q) returned unexpected error: %v",
					tc.path, tc.allowedPrefix, err)
			}
		})
	}
}

// TestValidateLVMIdentifier covers the LV/VG/snapshot identifier guard used by
// the LVM transfer and backup paths (NA-2, NA-9) so callers cannot smuggle
// option flags or path separators into LVM commands.
func TestValidateLVMIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"empty", "", true},
		{"path traversal", "..", true},
		{"option-flag short", "-h", true},
		{"option-flag long", "--help", true},
		{"path separator", "a/b", true},
		{"shell metacharacter semicolon", "snap;rm", true},
		{"shell metacharacter ampersand", "snap&rm", true},
		{"shell metacharacter pipe", "snap|rm", true},
		{"comma", "snap,foo=bar", true},
		{"space", "snap name", true},
		{"newline", "snap\nname", true},
		{"valid alphanumeric", "vm-12_3.snap", false},
		{"valid lowercase", "vm123", false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validateLVMIdentifier(tc.id)
			if tc.wantErr && err == nil {
				t.Fatalf("validateLVMIdentifier(%q) returned nil, want error", tc.id)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateLVMIdentifier(%q) returned %v, want nil", tc.id, err)
			}
		})
	}
}

// TestValidateQCOWSnapshotName covers NA-3: snapshot names forwarded to
// `qemu-img -l <name>` must reject option-flag prefixes, commas (qemu-img
// key=value list separator), and other shell-injection vectors.
func TestValidateQCOWSnapshotName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", true},
		{"leading dash", "-snap", true},
		{"long option", "--help", true},
		{"comma injection", "evil,foo=bar", true},
		{"equals injection", "snap=other", true},
		{"path traversal", "..", true},
		{"path separator", "a/b", true},
		{"semicolon", "snap;rm", true},
		{"valid simple", "snap-1", false},
		{"valid with dot", "snap.v2", false},
		{"valid with underscore", "snap_v2", false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validateQCOWSnapshotName(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("validateQCOWSnapshotName(%q) returned nil, want error", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateQCOWSnapshotName(%q) returned %v, want nil", tc.input, err)
			}
		})
	}
}

// TestTransferLVMDiskValidation covers NA-2: the LVM transfer path must reject
// untrusted volume-group and snapshot identifiers before any block-device path
// is constructed or `dd`/`blockdev` is invoked.
func TestTransferLVMDiskValidation(t *testing.T) {
	tests := []struct {
		name    string
		vg      string
		snap    string
		wantErr bool
	}{
		{"empty vg", "", "snap", true},
		{"empty snap", "vgvs", "", true},
		{"vg traversal", "..", "snap", true},
		{"snap traversal", "vgvs", "..", true},
		{"vg option flag", "-h", "snap", true},
		{"snap option flag", "vgvs", "--help", true},
		{"vg path separator", "a/b", "snap", true},
		{"snap path separator", "vgvs", "a/b", true},
		{"snap shell injection semicolon", "vgvs", "snap;rm -rf /", true},
		{"valid", "vgvs", "vm-1.snap", false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validateTransferLVMArgs(tc.vg, tc.snap)
			if tc.wantErr && err == nil {
				t.Fatalf("validateTransferLVMArgs(%q,%q) returned nil, want error",
					tc.vg, tc.snap)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateTransferLVMArgs(%q,%q) returned %v, want nil",
					tc.vg, tc.snap, err)
			}
		})
	}
}

// TestCreateLVMBackupValidation covers NA-9: snapshot names must be validated
// as LVM identifiers and the *full* backup_file_path must lie under the
// configured storage root, not just its parent directory.
func TestCreateLVMBackupValidation(t *testing.T) {
	const storageRoot = "/var/lib/virtuestack"

	tests := []struct {
		name        string
		snapshot    string
		backupPath  string
		storageRoot string
		wantErr     bool
	}{
		{
			name:        "empty snapshot rejected",
			snapshot:    "",
			backupPath:  storageRoot + "/backups/vm.img",
			storageRoot: storageRoot,
			wantErr:     true,
		},
		{
			name:        "snapshot with semicolon rejected",
			snapshot:    "snap;rm -rf /",
			backupPath:  storageRoot + "/backups/vm.img",
			storageRoot: storageRoot,
			wantErr:     true,
		},
		{
			name:        "snapshot path traversal rejected",
			snapshot:    "..",
			backupPath:  storageRoot + "/backups/vm.img",
			storageRoot: storageRoot,
			wantErr:     true,
		},
		{
			name:        "backup path escapes storage root via traversal",
			snapshot:    "snap-1",
			backupPath:  storageRoot + "/../etc/shadow",
			storageRoot: storageRoot,
			wantErr:     true,
		},
		{
			name:        "backup path absolute outside storage root",
			snapshot:    "snap-1",
			backupPath:  "/etc/shadow",
			storageRoot: storageRoot,
			wantErr:     true,
		},
		{
			name:        "empty backup path rejected",
			snapshot:    "snap-1",
			backupPath:  "",
			storageRoot: storageRoot,
			wantErr:     true,
		},
		{
			name:        "valid full path inside storage root",
			snapshot:    "snap-1",
			backupPath:  storageRoot + "/backups/vm-1.img",
			storageRoot: storageRoot,
			wantErr:     false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := validateCreateLVMBackupArgs(tc.snapshot, tc.backupPath, tc.storageRoot)
			if tc.wantErr && err == nil {
				t.Fatalf("validateCreateLVMBackupArgs(%q,%q,%q) returned nil, want error",
					tc.snapshot, tc.backupPath, tc.storageRoot)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateCreateLVMBackupArgs(%q,%q,%q) returned %v, want nil",
					tc.snapshot, tc.backupPath, tc.storageRoot, err)
			}
		})
	}
}
