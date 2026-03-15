// Package storage provides storage backend abstractions for VM disk management.
// It defines the StorageBackend interface that can be implemented by different
// storage providers (e.g., Ceph RBD, file-based QCOW2).
package storage

import (
	"context"
	"errors"
)

// StorageType represents the type of storage backend.
type StorageType string

const (
	// StorageTypeCEPH indicates a Ceph RBD storage backend.
	StorageTypeCEPH StorageType = "ceph"
	// StorageTypeQCOW indicates a file-based QCOW2 storage backend.
	StorageTypeQCOW StorageType = "qcow"
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

// StorageBackend defines the interface for storage operations.
// Implementations must be safe for concurrent use by multiple goroutines.
type StorageBackend interface {
	// CloneFromTemplate clones a template snapshot to create a new VM disk.
	// sourcePool/sourceImage@sourceSnap -> targetPool/targetImage
	CloneFromTemplate(ctx context.Context, sourcePool string, sourceImage string, sourceSnap string, targetImage string) error

	// CloneSnapshotToPool clones a snapshot from one pool to another.
	// Used for backup operations to clone a protected snapshot to the backup pool.
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

	// ProtectSnapshot marks a snapshot as protected to prevent deletion
	// and allow cloning operations.
	ProtectSnapshot(ctx context.Context, imageName string, snapshotName string) error

	// UnprotectSnapshot removes protection from a snapshot.
	UnprotectSnapshot(ctx context.Context, imageName string, snapshotName string) error

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

	// GetStorageType returns the type of storage backend.
	GetStorageType() StorageType
}
