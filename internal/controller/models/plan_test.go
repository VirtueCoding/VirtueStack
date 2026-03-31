package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlanStorageBackendConstants(t *testing.T) {
	assert.Equal(t, "ceph", StorageBackendCeph)
	assert.Equal(t, "qcow", StorageBackendQcow)
	assert.Equal(t, "lvm", StorageBackendLvm)
}

func TestDefaultStorageBackend(t *testing.T) {
	assert.Equal(t, StorageBackendCeph, DefaultStorageBackend)
}

func int64Ptr(v int64) *int64 { return &v }

func TestPlan_Fields(t *testing.T) {
	plan := Plan{
		ID:               "plan-123",
		Name:             "Basic VPS",
		Slug:             "basic-vps",
		VCPU:             2,
		MemoryMB:         2048,
		DiskGB:           50,
		BandwidthLimitGB: 1000,
		PortSpeedMbps:    1000,
		PriceMonthly:     int64Ptr(500),
		PriceHourly:      int64Ptr(1),
		Currency:         "USD",
		StorageBackend:   StorageBackendCeph,
		IsActive:         true,
		SortOrder:        1,
		SnapshotLimit:    2,
		BackupLimit:      2,
		ISOUploadLimit:   2,
	}

	assert.Equal(t, "plan-123", plan.ID)
	assert.Equal(t, "Basic VPS", plan.Name)
	assert.Equal(t, "basic-vps", plan.Slug)
	assert.Equal(t, 2, plan.VCPU)
	assert.Equal(t, 2048, plan.MemoryMB)
	assert.Equal(t, 50, plan.DiskGB)
	assert.Equal(t, int64Ptr(500), plan.PriceMonthly)
	assert.Equal(t, int64Ptr(1), plan.PriceHourly)
	assert.Equal(t, 2, plan.SnapshotLimit)
	assert.Equal(t, 2, plan.BackupLimit)
	assert.Equal(t, 2, plan.ISOUploadLimit)
}

func TestPlanCreateRequest_Fields(t *testing.T) {
	req := PlanCreateRequest{
		Name:             "Pro VPS",
		Slug:             "pro-vps",
		VCPU:             4,
		MemoryMB:         8192,
		DiskGB:           100,
		BandwidthLimitGB: 5000,
		PortSpeedMbps:    1000,
		PriceMonthly:     int64Ptr(2000),
		PriceHourly:      int64Ptr(3),
		Currency:         "USD",
		StorageBackend:   "lvm",
		IsActive:         true,
		SnapshotLimit:    5,
		BackupLimit:      5,
		ISOUploadLimit:   3,
	}

	assert.Equal(t, "Pro VPS", req.Name)
	assert.Equal(t, "pro-vps", req.Slug)
	assert.Equal(t, 4, req.VCPU)
	assert.Equal(t, 8192, req.MemoryMB)
	assert.Equal(t, "lvm", req.StorageBackend)
}

func TestPlanUpdateRequest_PartialUpdate(t *testing.T) {
	name := "Updated Plan"
	vcpu := 8
	price := int64(3000)

	req := PlanUpdateRequest{
		Name:         &name,
		VCPU:         &vcpu,
		PriceMonthly: &price,
	}

	assert.Equal(t, &name, req.Name)
	assert.Equal(t, &vcpu, req.VCPU)
	assert.Equal(t, &price, req.PriceMonthly)
	// Other fields should be nil for partial update
	assert.Nil(t, req.Slug)
	assert.Nil(t, req.MemoryMB)
	assert.Nil(t, req.DiskGB)
	assert.Nil(t, req.StorageBackend)
}
