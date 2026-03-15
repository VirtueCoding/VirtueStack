package models

import (
	"encoding/json"
	"time"
)

const (
	FailoverStatusPending    = "pending"
	FailoverStatusApproved   = "approved"
	FailoverStatusInProgress = "in_progress"
	FailoverStatusCompleted  = "completed"
	FailoverStatusFailed     = "failed"
	FailoverStatusCancelled  = "cancelled"
)

type FailoverRequest struct {
	ID          string          `json:"id" db:"id"`
	NodeID      string          `json:"node_id" db:"node_id"`
	RequestedBy string          `json:"requested_by" db:"requested_by"`
	Status      string          `json:"status" db:"status"`
	Reason      *string         `json:"reason,omitempty" db:"reason"`
	Result      json.RawMessage `json:"result,omitempty" db:"result"`
	ApprovedAt  *time.Time      `json:"approved_at,omitempty" db:"approved_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

type FailoverRequestListFilter struct {
	PaginationParams
	NodeID *string
	Status *string
}
