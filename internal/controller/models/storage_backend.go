// Package models provides data model types for VirtueStack Controller.
package models

import "time"

// StorageBackendType defines the type of storage backend.
type StorageBackendType string

const (
	// StorageTypeCeph indicates Ceph/RBD distributed storage.
	StorageTypeCeph StorageBackendType = "ceph"
	// StorageTypeQCOW indicates local QCOW2 file-based storage.
	StorageTypeQCOW StorageBackendType = "qcow"
	// StorageTypeLVM indicates LVM thin provisioning storage.
	StorageTypeLVM StorageBackendType = "lvm"
)

// StorageBackend represents a named storage backend configuration.
// Backends are assigned to nodes and VMs reference them for disk storage.
type StorageBackend struct {
	ID   string             `json:"id" db:"id"`
	Name string             `json:"name" db:"name"`
	Type StorageBackendType `json:"type" db:"type"`

	// Type-specific config (nullable based on type)
	CephPool        *string `json:"ceph_pool,omitempty" db:"ceph_pool"`
	CephUser        *string `json:"ceph_user,omitempty" db:"ceph_user"`
	CephMonitors    *string `json:"ceph_monitors,omitempty" db:"ceph_monitors"`
	CephKeyringPath *string `json:"ceph_keyring_path,omitempty" db:"ceph_keyring_path"`
	StoragePath     *string `json:"storage_path,omitempty" db:"storage_path"`
	LVMVolumeGroup  *string `json:"lvm_volume_group,omitempty" db:"lvm_volume_group"`
	LVMThinPool     *string `json:"lvm_thin_pool,omitempty" db:"lvm_thin_pool"`

	// LVM threshold configuration (alerts trigger when usage exceeds these)
	LVMDataPercentThreshold     *int `json:"lvm_data_percent_threshold,omitempty" db:"lvm_data_percent_threshold"`
	LVMMetadataPercentThreshold *int `json:"lvm_metadata_percent_threshold,omitempty" db:"lvm_metadata_percent_threshold"`

	// Health metrics
	TotalGB          *int64   `json:"total_gb,omitempty" db:"total_gb"`
	UsedGB           *int64   `json:"used_gb,omitempty" db:"used_gb"`
	AvailableGB      *int64   `json:"available_gb,omitempty" db:"available_gb"`
	HealthStatus     string   `json:"health_status" db:"health_status"`
	HealthMessage    *string  `json:"health_message,omitempty" db:"health_message"`
	LVMDataPercent   *float64 `json:"lvm_data_percent,omitempty" db:"lvm_data_percent"`
	LVMMetadataPercent *float64 `json:"lvm_metadata_percent,omitempty" db:"lvm_metadata_percent"`

	// Assigned nodes (populated on read)
	Nodes []StorageBackendNode `json:"nodes,omitempty"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// StorageBackendNode represents a node assignment to a storage backend.
type StorageBackendNode struct {
	NodeID   string `json:"node_id"`
	Hostname string `json:"hostname"`
	Enabled  bool   `json:"enabled"`
}

// StorageBackendCreateRequest holds the fields required to create a new storage backend.
type StorageBackendCreateRequest struct {
	Name string             `json:"name" validate:"required,max=100"`
	Type StorageBackendType `json:"type" validate:"required,oneof=ceph qcow lvm"`

	CephPool        *string `json:"ceph_pool,omitempty" validate:"omitempty,max=100"`
	CephUser        *string `json:"ceph_user,omitempty" validate:"omitempty,max=100"`
	CephMonitors    *string `json:"ceph_monitors,omitempty" validate:"omitempty,max=500"`
	CephKeyringPath *string `json:"ceph_keyring_path,omitempty" validate:"omitempty,max=500"`
	StoragePath     *string `json:"storage_path,omitempty" validate:"omitempty,max=500"`
	LVMVolumeGroup  *string `json:"lvm_volume_group,omitempty" validate:"omitempty,max=100"`
	LVMThinPool     *string `json:"lvm_thin_pool,omitempty" validate:"omitempty,max=100"`

	// LVM threshold configuration (alerts trigger when usage exceeds these)
	LVMDataPercentThreshold     *int `json:"lvm_data_percent_threshold,omitempty" validate:"omitempty,min=1,max=100"`
	LVMMetadataPercentThreshold *int `json:"lvm_metadata_percent_threshold,omitempty" validate:"omitempty,min=1,max=100"`

	NodeIDs []string `json:"node_ids,omitempty" validate:"max=100,dive,uuid"`
}

// StorageBackendUpdateRequest holds the fields that can be updated on an existing storage backend.
// All fields are optional to support partial updates.
type StorageBackendUpdateRequest struct {
	Name             *string `json:"name,omitempty" validate:"omitempty,max=100"`
	CephPool         *string `json:"ceph_pool,omitempty" validate:"omitempty,max=100"`
	CephUser         *string `json:"ceph_user,omitempty" validate:"omitempty,max=100"`
	CephMonitors     *string `json:"ceph_monitors,omitempty" validate:"omitempty,max=500"`
	CephKeyringPath  *string `json:"ceph_keyring_path,omitempty" validate:"omitempty,max=500"`
	StoragePath      *string `json:"storage_path,omitempty" validate:"omitempty,max=500"`
	LVMVolumeGroup   *string `json:"lvm_volume_group,omitempty" validate:"omitempty,max=100"`
	LVMThinPool      *string `json:"lvm_thin_pool,omitempty" validate:"omitempty,max=100"`

	// LVM threshold configuration
	LVMDataPercentThreshold     *int `json:"lvm_data_percent_threshold,omitempty" validate:"omitempty,min=1,max=100"`
	LVMMetadataPercentThreshold *int `json:"lvm_metadata_percent_threshold,omitempty" validate:"omitempty,min=1,max=100"`
}

// StorageBackendHealth holds health metrics for updating a storage backend.
type StorageBackendHealth struct {
	TotalGB            *int64
	UsedGB             *int64
	AvailableGB        *int64
	HealthStatus       string
	HealthMessage      *string
	LVMDataPercent     *float64
	LVMMetadataPercent *float64
}

// StorageBackendListFilter holds query parameters for filtering storage backends.
type StorageBackendListFilter struct {
	Type   *StorageBackendType `form:"type"`
	Status *string             `form:"status"`
	PaginationParams
}