// Package nodeagent provides input validators used by gRPC handlers.
// These helpers guard host-boundary RPCs (LVM transfer, QCOW snapshot export,
// LVM backup) against shell-injection, option-flag injection, and path-traversal
// inputs originating from the controller.
package nodeagent

import (
	"fmt"
	"regexp"
	"strings"
)

// validNodeAgentIdentifier matches LVM volume groups, logical volumes, snapshot
// names, and other arguments that are appended to system commands. It mirrors
// internal/nodeagent/storage.validLVMLVName: it requires a leading alphanumeric
// character (so values cannot start with `-` and be parsed as flags) and only
// permits alphanumeric, underscore, hyphen, and dot characters thereafter. This
// excludes path separators, shell metacharacters, whitespace, NULs, and the
// `,` / `=` characters that qemu-img treats as key=value separators.
var validNodeAgentIdentifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// validateLVMIdentifier rejects empty values, traversal attempts, and any
// identifier that does not match the conservative
// alphanumeric/underscore/hyphen/dot pattern. Used for VG and LV/snapshot
// names that are concatenated into `/dev/<vg>/<lv>` paths or passed as
// argv-positional arguments.
func validateLVMIdentifier(id string) error {
	if id == "" {
		return fmt.Errorf("identifier must not be empty")
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("identifier %q must not contain path traversal characters", id)
	}
	if !validNodeAgentIdentifier.MatchString(id) {
		return fmt.Errorf("identifier %q must match %s",
			id, validNodeAgentIdentifier.String())
	}
	return nil
}

// validateQCOWSnapshotName guards qemu-img invocations such as
// `qemu-img convert -l <snapshot>` from option-flag injection and from values
// that qemu-img would parse as a comma-separated key=value list. The regex is
// identical to the LVM identifier check; both contexts require argv-safe
// tokens with no leading dash, comma, equals, slash, or whitespace.
func validateQCOWSnapshotName(name string) error {
	if name == "" {
		return fmt.Errorf("snapshot name must not be empty")
	}
	if strings.ContainsAny(name, ",=") {
		return fmt.Errorf("snapshot name %q must not contain ',' or '='", name)
	}
	if !validNodeAgentIdentifier.MatchString(name) {
		return fmt.Errorf("snapshot name %q must match %s",
			name, validNodeAgentIdentifier.String())
	}
	return nil
}

// validateTransferLVMArgs validates the VG and snapshot identifiers used to
// build the source block-device path in transferLVMDisk. Both must be safe
// LVM identifiers before they are concatenated into `/dev/<vg>/<snap>`.
func validateTransferLVMArgs(volumeGroup, snapshotName string) error {
	if err := validateLVMIdentifier(volumeGroup); err != nil {
		return fmt.Errorf("invalid source_lvm_volume_group: %w", err)
	}
	if err := validateLVMIdentifier(snapshotName); err != nil {
		return fmt.Errorf("invalid snapshot_name: %w", err)
	}
	return nil
}

// validateCreateLVMBackupArgs validates the snapshot identifier and the full
// backup file path supplied to CreateLVMBackup. The previous implementation
// validated only filepath.Dir(backupFilePath) which let a crafted file
// component escape the storage root (NA-9). This helper enforces both checks
// so the handler can reject hostile input before any block-device or file I/O
// is performed.
func validateCreateLVMBackupArgs(snapshotName, backupFilePath, storageRoot string) error {
	if err := validateLVMIdentifier(snapshotName); err != nil {
		return fmt.Errorf("invalid snapshot_name: %w", err)
	}
	if err := validatePath(backupFilePath, storageRoot); err != nil {
		return fmt.Errorf("invalid backup_file_path: %w", err)
	}
	return nil
}
