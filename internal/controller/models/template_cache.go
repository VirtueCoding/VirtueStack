// Package models provides data model types for VirtueStack Controller.
package models

import "time"

// TemplateCacheStatus represents the sync state of a template on a node.
type TemplateCacheStatus string

const (
	// TemplateCacheStatusPending means the cache entry was created but download hasn't started.
	TemplateCacheStatusPending TemplateCacheStatus = "pending"
	// TemplateCacheStatusDownloading means the template is being transferred to the node.
	TemplateCacheStatusDownloading TemplateCacheStatus = "downloading"
	// TemplateCacheStatusReady means the template is fully cached and available on the node.
	TemplateCacheStatusReady TemplateCacheStatus = "ready"
	// TemplateCacheStatusFailed means the caching operation failed.
	TemplateCacheStatusFailed TemplateCacheStatus = "failed"
)

// TemplateCacheEntry tracks whether a given template is cached on a given node.
// Ceph nodes skip this entirely — they access templates directly from the shared pool.
// QCOW/LVM nodes cache templates locally via lazy pull (on first VM create) or
// admin-triggered eager distribution.
type TemplateCacheEntry struct {
	TemplateID string              `json:"template_id" db:"template_id"`
	NodeID     string              `json:"node_id" db:"node_id"`
	Status     TemplateCacheStatus `json:"status" db:"status"`
	LocalPath  *string             `json:"local_path,omitempty" db:"local_path"`
	SizeBytes  *int64              `json:"size_bytes,omitempty" db:"size_bytes"`
	SyncedAt   *time.Time          `json:"synced_at,omitempty" db:"synced_at"`
	ErrorMsg   *string             `json:"error_msg,omitempty" db:"error_msg"`
	CreatedAt  time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at" db:"updated_at"`
}

// TemplateCacheStatusResponse is the API response for template cache status.
type TemplateCacheStatusResponse struct {
	TemplateID string               `json:"template_id"`
	Entries    []TemplateCacheEntry `json:"entries"`
}

// TemplateDistributeRequest is the API request for distributing a template to nodes.
type TemplateDistributeRequest struct {
	NodeIDs []string `json:"node_ids" validate:"required,min=1,dive,uuid"`
}
