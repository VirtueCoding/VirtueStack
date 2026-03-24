// Package storage provides storage backend abstractions for VM disk management.
// It defines the StorageBackend interface that can be implemented by different
// storage providers (e.g., Ceph RBD, file-based QCOW2).
package storage

import (
	"context"
	"errors"
	"time"
)

// StorageType represents the type of storage backend.
type StorageType string

const (
	// StorageTypeCEPH indicates a Ceph RBD storage backend.
	StorageTypeCEPH StorageType = "ceph"
	// StorageTypeQCOW indicates a file-based QCOW2 storage backend.
	StorageTypeQCOW StorageType = "qcow"
	// StorageTypeLVM indicates an LVM thin-provisioned storage backend.
	StorageTypeLVM StorageType = "lvm"
)

// StorageError represents storage-specific errors with typed codes.
type StorageError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

// ErrorCode defines categories of storage errors.
type ErrorCode string

const (
	// ErrCodeNotFound indicates the requested resource was not found.
	ErrCodeNotFound ErrorCode = "NOT_FOUND"
	// ErrCodeAlreadyExists indicates the resource already exists.
	ErrCodeAlreadyExists ErrorCode = "ALREADY_EXISTS"
	// ErrCodePermission indicates a permission or authorization failure.
	ErrCodePermission ErrorCode = "PERMISSION_DENIED"
	// ErrCodeQuota indicates a quota or capacity limit was exceeded.
	ErrCodeQuota ErrorCode = "QUOTA_EXCEEDED"
	// ErrCodeInvalid indicates invalid input parameters.
	ErrCodeInvalid ErrorCode = "INVALID_ARGUMENT"
	// ErrCodeInternal indicates an unexpected internal error.
	ErrCodeInternal ErrorCode = "INTERNAL_ERROR"
	// ErrCodeConnection indicates a connection failure to storage backend.
	ErrCodeConnection ErrorCode = "CONNECTION_FAILED"
	// ErrCodeInUse indicates the resource is in use (e.g., VM running).
	ErrCodeInUse ErrorCode = "IN_USE"
	// ErrCodeNoSpace indicates the pool or volume group is full.
	ErrCodeNoSpace ErrorCode = "NO_SPACE"
)

// Error implements the error interface for StorageError.
func (e *StorageError) Error() string {
	if e.Cause != nil {
		return string(e.Code) + ": " + e.Message + ": " + e.Cause.Error()
	}
	return string(e.Code) + ": " + e.Message
}

// Unwrap returns the underlying cause of the error.
func (e *StorageError) Unwrap() error {
	return e.Cause
}

// Is compares StorageError instances by code.
func (e *StorageError) Is(target error) bool {
	var t *StorageError
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

// NewStorageError creates a new StorageError with the given parameters.
func NewStorageError(code ErrorCode, message string, cause error) *StorageError {
	return &StorageError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// ImageInfo holds metadata about a storage image.
type ImageInfo struct {
	// Filename is the canonical disk identifier (image name or file path).
	Filename string
	// Format is the storage format (e.g., "qcow2", "raw", "rbd").
	Format string
	// VirtualSizeBytes is the logical size of the image.
	VirtualSizeBytes int64
	// ActualSizeBytes is the physical space used on storage.
	ActualSizeBytes int64
	// DirtyFlag indicates unsaved write operations.
	DirtyFlag bool
	// ClusterSize is the allocation unit size (backend-specific).
	ClusterSize int64
	// BackingFile is the backing image path, if any.
	BackingFile string
}

// StorageBackend defines the interface for storage operations.
// Implementations must be safe for concurrent use by multiple goroutines.
type StorageBackend interface {
	// DiskIdentifier returns the canonical disk identifier for a VM.
	// For RBD this is the image name ("vs-{vmID}-disk0").
	// For QCOW this is the absolute file path ("{basePath}/vms/{vmID}-disk0.qcow2").
	// This method enables storage-agnostic disk operations in handlers.
	DiskIdentifier(vmID string) string

	// CloneFromTemplate clones a template snapshot to create a new VM disk.
	// sourcePool/sourceImage@sourceSnap -> targetPool/targetImage
	CloneFromTemplate(ctx context.Context, sourcePool string, sourceImage string, sourceSnap string, targetImage string) error

	// CloneSnapshotToPool clones a snapshot from one pool to another.
	// Used for backup operations to clone a protected snapshot to the backup pool.
	// This method handles any backend-specific protection internally (e.g., RBD
	// protects snapshots before cloning; QCOW2 just copies the file).
	CloneSnapshotToPool(ctx context.Context, sourcePool string, sourceImage string, sourceSnap string, targetPool string, targetImage string) error

	// Resize changes the size of an existing image.
	// newSizeGB must be greater than the current size (storage can only grow).
	Resize(ctx context.Context, imageName string, newSizeGB int) error

	// Delete removes an image and all its associated snapshots.
	Delete(ctx context.Context, imageName string) error

	// CreateSnapshot creates a new snapshot of an existing image.
	CreateSnapshot(ctx context.Context, imageName string, snapshotName string) error

	// DeleteSnapshot removes a snapshot from an image.
	DeleteSnapshot(ctx context.Context, imageName string, snapshotName string) error

	// ListSnapshots returns all snapshots associated with an image.
	ListSnapshots(ctx context.Context, imageName string) ([]SnapshotInfo, error)

	// GetImageSize returns the current size of an image in bytes.
	GetImageSize(ctx context.Context, imageName string) (int64, error)

	// ImageExists checks whether an image exists in the storage pool.
	ImageExists(ctx context.Context, imageName string) (bool, error)

	// FlattenImage removes the dependency of a cloned image on its parent,
	// making it an independent image. This is useful after cloning to
	// free up the parent snapshot.
	FlattenImage(ctx context.Context, imageName string) error

	// GetPoolStats returns storage capacity and usage statistics.
	GetPoolStats(ctx context.Context) (*PoolStats, error)

	// Rollback reverts an image to a previous snapshot state in-place.
	// The VM must be stopped before calling this method.
	Rollback(ctx context.Context, imageName string, snapshotName string) error

	// CreateImage creates a new empty image of the specified size.
	// This is a backend-specific helper for operations that need to create
	// standalone images (e.g., LVM volume creation). Returns an error for
	// backends that don't support this operation.
	CreateImage(ctx context.Context, imageName string, sizeGB int) error

	// GetImageInfo returns detailed information about an image.
	// Returns backend-specific metadata including virtual size, actual size,
	// and format details. Returns error if image doesn't exist.
	GetImageInfo(ctx context.Context, imageName string) (*ImageInfo, error)

	// GetStorageType returns the type of storage backend.
	GetStorageType() StorageType
}

// TemplateInfo holds metadata about a template image.
// This unified struct is returned by all TemplateBackend implementations.
type TemplateInfo struct {
	// Name is the template name (e.g., "ubuntu-2204").
	Name string
	// FilePath is the canonical path or RBD image name for the template.
	FilePath string
	// SizeBytes is the virtual size of the template in bytes.
	SizeBytes int64
	// CreatedAt is the creation timestamp.
	CreatedAt time.Time
	// OSFamily is the operating system family (e.g., "ubuntu", "debian").
	OSFamily string
	// OSVersion is the operating system version (e.g., "22.04", "12").
	OSVersion string
}

// TemplateMeta contains optional metadata for template imports.
type TemplateMeta struct {
	OSFamily  string
	OSVersion string
}

// TemplateBackend defines the interface for template management operations.
// Implementations must be safe for concurrent use by multiple goroutines.
type TemplateBackend interface {
	// ImportTemplate imports a template from the given source path.
	// The ref parameter is the template name/identifier.
	// Returns the canonical path/reference and size in bytes.
	ImportTemplate(ctx context.Context, ref, sourcePath string, meta TemplateMeta) (filePath string, sizeBytes int64, err error)

	// DeleteTemplate removes a template identified by ref.
	// ref is the template name for RBD or file path for QCOW.
	DeleteTemplate(ctx context.Context, ref string) error

	// CloneForVM clones a template to create a new VM disk.
	// templateRef is the template name (RBD) or file path (QCOW).
	// Returns the disk identifier (image name for RBD, file path for QCOW).
	CloneForVM(ctx context.Context, templateRef, vmID string, sizeGB int) (diskID string, err error)

	// TemplateExists checks if a template exists.
	TemplateExists(ctx context.Context, ref string) (bool, error)

	// GetTemplateSize returns the virtual size of a template in bytes.
	GetTemplateSize(ctx context.Context, ref string) (int64, error)

	// ListTemplates returns all available templates.
	ListTemplates(ctx context.Context) ([]TemplateInfo, error)
}
