// Package models provides data model types for VirtueStack Controller.
package models

import "time"

// Template represents an OS image template available for VM provisioning.
type Template struct {
	ID                string `json:"id" db:"id"`
	Name              string `json:"name" db:"name"`
	OSFamily          string `json:"os_family" db:"os_family"`
	OSVersion         string `json:"os_version" db:"os_version"`
	RBDImage          string `json:"rbd_image" db:"rbd_image"`
	RBDSnapshot       string `json:"rbd_snapshot" db:"rbd_snapshot"`
	MinDiskGB         int    `json:"min_disk_gb" db:"min_disk_gb"`
	SupportsCloudInit bool   `json:"supports_cloudinit" db:"supports_cloudinit"`
	IsActive          bool   `json:"is_active" db:"is_active"`
	SortOrder         int    `json:"sort_order" db:"sort_order"`
	Version           int    `json:"version" db:"version"`                   // Version incremented on each update for audit trail
	Description       string `json:"description,omitempty" db:"description"` // Optional description of the template
	// StorageBackend specifies the storage type: "ceph" or "qcow". Defaults to "ceph" for backward compatibility.
	StorageBackend string `json:"storage_backend" db:"storage_backend"`
	// FilePath is the path to the template file for QCOW storage. Empty for Ceph.
	FilePath  string    `json:"file_path,omitempty" db:"file_path"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// TemplateCreateRequest holds the fields required to register a new OS template.
type TemplateCreateRequest struct {
	Name              string `json:"name" validate:"required,max=100"`
	OSFamily          string `json:"os_family" validate:"required,max=50"`
	OSVersion         string `json:"os_version" validate:"required,max=50"`
	RBDImage          string `json:"rbd_image" validate:"required,max=255"`
	RBDSnapshot       string `json:"rbd_snapshot" validate:"required,max=255"`
	MinDiskGB         int    `json:"min_disk_gb" validate:"required,min=1"`
	SupportsCloudInit bool   `json:"supports_cloudinit"`
	IsActive          bool   `json:"is_active"`
	SortOrder         int    `json:"sort_order" validate:"min=0"`
	Description       string `json:"description,omitempty" validate:"max=500"`
	StorageBackend    string `json:"storage_backend" validate:"omitempty,oneof=ceph qcow"`
	FilePath          string `json:"file_path,omitempty" validate:"omitempty,max=500"`
}

// TemplateUpdateRequest holds the fields that can be updated on an existing template.
// All fields are optional to support partial updates.
type TemplateUpdateRequest struct {
	Name              *string `json:"name,omitempty" validate:"omitempty,max=100"`
	OSFamily          *string `json:"os_family,omitempty" validate:"omitempty,max=50"`
	OSVersion         *string `json:"os_version,omitempty" validate:"omitempty,max=50"`
	RBDImage          *string `json:"rbd_image,omitempty" validate:"omitempty,max=255"`
	RBDSnapshot       *string `json:"rbd_snapshot,omitempty" validate:"omitempty,max=255"`
	MinDiskGB         *int    `json:"min_disk_gb,omitempty" validate:"omitempty,min=1"`
	SupportsCloudInit *bool   `json:"supports_cloudinit,omitempty"`
	IsActive          *bool   `json:"is_active,omitempty"`
	SortOrder         *int    `json:"sort_order,omitempty" validate:"omitempty,min=0"`
	Description       *string `json:"description,omitempty" validate:"omitempty,max=500"`
	StorageBackend    *string `json:"storage_backend,omitempty" validate:"omitempty,oneof=ceph qcow"`
	FilePath          *string `json:"file_path,omitempty" validate:"omitempty,max=500"`
}
