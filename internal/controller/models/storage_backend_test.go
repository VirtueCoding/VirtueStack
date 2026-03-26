package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStorageBackendTypeConstants(t *testing.T) {
	assert.Equal(t, StorageBackendType("ceph"), StorageTypeCeph)
	assert.Equal(t, StorageBackendType("qcow"), StorageTypeQCOW)
	assert.Equal(t, StorageBackendType("lvm"), StorageTypeLVM)
}

func TestStorageBackendCreateRequest_Fields(t *testing.T) {
	pool := "vs-vms"
	user := "admin"
	monitors := "10.0.0.1,10.0.0.2,10.0.0.3"
	threshold := 90

	req := StorageBackendCreateRequest{
		Name:                    "Ceph Primary",
		Type:                    StorageTypeCeph,
		CephPool:                &pool,
		CephUser:                &user,
		CephMonitors:            &monitors,
		LVMDataPercentThreshold: &threshold,
		NodeIDs:                 []string{"node-1", "node-2"},
	}

	assert.Equal(t, "Ceph Primary", req.Name)
	assert.Equal(t, StorageTypeCeph, req.Type)
	assert.Equal(t, &pool, req.CephPool)
	assert.Equal(t, &user, req.CephUser)
	assert.Equal(t, &monitors, req.CephMonitors)
	assert.Equal(t, &threshold, req.LVMDataPercentThreshold)
	assert.Len(t, req.NodeIDs, 2)
}

func TestStorageBackendUpdateRequest_PartialUpdate(t *testing.T) {
	name := "Updated Backend"
	threshold := 95

	req := StorageBackendUpdateRequest{
		Name:                    &name,
		LVMDataPercentThreshold: &threshold,
	}

	assert.Equal(t, &name, req.Name)
	assert.Equal(t, &threshold, req.LVMDataPercentThreshold)
	assert.Nil(t, req.CephPool)
	assert.Nil(t, req.LVMVolumeGroup)
}

func TestStorageBackendHealth_Fields(t *testing.T) {
	totalGB := int64(1000)
	usedGB := int64(500)
	availGB := int64(500)
	msg := "healthy"
	dataPct := 50.0
	metaPct := 10.0

	health := StorageBackendHealth{
		TotalGB:            &totalGB,
		UsedGB:             &usedGB,
		AvailableGB:        &availGB,
		HealthStatus:       "healthy",
		HealthMessage:      &msg,
		LVMDataPercent:     &dataPct,
		LVMMetadataPercent: &metaPct,
	}

	assert.Equal(t, &totalGB, health.TotalGB)
	assert.Equal(t, &usedGB, health.UsedGB)
	assert.Equal(t, &availGB, health.AvailableGB)
	assert.Equal(t, "healthy", health.HealthStatus)
}

func TestStorageBackendListFilter_Fields(t *testing.T) {
	tp := StorageTypeLVM
	status := "healthy"

	filter := StorageBackendListFilter{
		Type:   &tp,
		Status: &status,
	}

	assert.Equal(t, &tp, filter.Type)
	assert.Equal(t, &status, filter.Status)
}

func TestStorageBackendNode_Fields(t *testing.T) {
	node := StorageBackendNode{
		NodeID:   "node-123",
		Hostname: "node-01",
		Enabled:  true,
	}

	assert.Equal(t, "node-123", node.NodeID)
	assert.Equal(t, "node-01", node.Hostname)
	assert.True(t, node.Enabled)
}
