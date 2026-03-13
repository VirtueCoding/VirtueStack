// Package models provides data model types for VirtueStack Controller.
package models

import "time"

// Plan represents a VPS service plan with resource allocations and pricing information.
type Plan struct {
	ID               string `json:"id" db:"id"`
	Name             string `json:"name" db:"name"`
	Slug             string `json:"slug" db:"slug"`
	VCPU             int    `json:"vcpu" db:"vcpu"`
	MemoryMB         int    `json:"memory_mb" db:"memory_mb"`
	DiskGB           int    `json:"disk_gb" db:"disk_gb"`
	BandwidthLimitGB int    `json:"bandwidth_limit_gb" db:"bandwidth_limit_gb"`
	PortSpeedMbps    int    `json:"port_speed_mbps" db:"port_speed_mbps"`
	// Billing amounts stored in integer minor units (cents) to avoid floating-point issues.
	PriceMonthly int64     `json:"price_monthly" db:"price_monthly"`
	PriceHourly  int64     `json:"price_hourly" db:"price_hourly"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	SortOrder    int       `json:"sort_order" db:"sort_order"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// PlanCreateRequest holds the fields required to create a new service plan.
// Price fields are in integer minor units (cents).
type PlanCreateRequest struct {
	Name             string `json:"name" validate:"required,max=100"`
	Slug             string `json:"slug" validate:"required,max=100,alphanum"`
	VCPU             int    `json:"vcpu" validate:"required,min=1"`
	MemoryMB         int    `json:"memory_mb" validate:"required,min=512"`
	DiskGB           int    `json:"disk_gb" validate:"required,min=10"`
	BandwidthLimitGB int    `json:"bandwidth_limit_gb" validate:"min=0"`
	PortSpeedMbps    int    `json:"port_speed_mbps" validate:"required,min=1"`
	PriceMonthly     int64  `json:"price_monthly" validate:"min=0"`
	PriceHourly      int64  `json:"price_hourly" validate:"min=0"`
	IsActive         bool   `json:"is_active"`
	SortOrder        int    `json:"sort_order" validate:"min=0"`
}

// PlanUpdateRequest holds the fields that can be updated on an existing plan.
// All fields are optional to support partial updates.
// Price fields are in integer minor units (cents).
type PlanUpdateRequest struct {
	Name             *string `json:"name,omitempty" validate:"omitempty,max=100"`
	Slug             *string `json:"slug,omitempty" validate:"omitempty,max=100,alphanum"`
	VCPU             *int    `json:"vcpu,omitempty" validate:"omitempty,min=1"`
	MemoryMB         *int    `json:"memory_mb,omitempty" validate:"omitempty,min=512"`
	DiskGB           *int    `json:"disk_gb,omitempty" validate:"omitempty,min=10"`
	BandwidthLimitGB *int    `json:"bandwidth_limit_gb,omitempty" validate:"omitempty,min=0"`
	PortSpeedMbps    *int    `json:"port_speed_mbps,omitempty" validate:"omitempty,min=1"`
	PriceMonthly     *int64  `json:"price_monthly,omitempty" validate:"omitempty,min=0"`
	PriceHourly      *int64  `json:"price_hourly,omitempty" validate:"omitempty,min=0"`
	IsActive         *bool   `json:"is_active,omitempty"`
	SortOrder        *int    `json:"sort_order,omitempty" validate:"omitempty,min=0"`
}
