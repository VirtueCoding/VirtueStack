package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTemplate_Fields(t *testing.T) {
	tmpl := Template{
		ID:                "tmpl-123",
		Name:              "Ubuntu 24.04",
		OSFamily:          "ubuntu",
		OSVersion:         "24.04",
		RBDImage:          "vs-images/ubuntu-2404",
		RBDSnapshot:       "base",
		MinDiskGB:         20,
		SupportsCloudInit: true,
		IsActive:          true,
		SortOrder:         1,
		Version:           3,
		Description:       "Ubuntu 24.04 LTS",
		StorageBackend:    "ceph",
	}

	assert.Equal(t, "tmpl-123", tmpl.ID)
	assert.Equal(t, "Ubuntu 24.04", tmpl.Name)
	assert.Equal(t, "ubuntu", tmpl.OSFamily)
	assert.Equal(t, "24.04", tmpl.OSVersion)
	assert.Equal(t, 20, tmpl.MinDiskGB)
	assert.True(t, tmpl.SupportsCloudInit)
	assert.True(t, tmpl.IsActive)
	assert.Equal(t, 3, tmpl.Version)
	assert.Equal(t, "ceph", tmpl.StorageBackend)
}

func TestTemplateCreateRequest_Fields(t *testing.T) {
	req := TemplateCreateRequest{
		Name:              "CentOS 9",
		OSFamily:          "centos",
		OSVersion:         "9",
		RBDImage:          "vs-images/centos-9",
		RBDSnapshot:       "base",
		MinDiskGB:         25,
		SupportsCloudInit: true,
		IsActive:          true,
		SortOrder:         5,
		Description:       "CentOS Stream 9",
		StorageBackend:    "qcow",
		FilePath:          "/var/lib/virtuestack/templates/centos-9.qcow2",
	}

	assert.Equal(t, "CentOS 9", req.Name)
	assert.Equal(t, "centos", req.OSFamily)
	assert.Equal(t, 25, req.MinDiskGB)
	assert.Equal(t, "qcow", req.StorageBackend)
}

func TestTemplateUpdateRequest_PartialUpdate(t *testing.T) {
	name := "Updated Name"
	active := false
	sortOrder := 10

	req := TemplateUpdateRequest{
		Name:      &name,
		IsActive:  &active,
		SortOrder: &sortOrder,
	}

	assert.Equal(t, &name, req.Name)
	assert.Equal(t, &active, req.IsActive)
	assert.Equal(t, &sortOrder, req.SortOrder)
	// Other fields should be nil for partial update
	assert.Nil(t, req.OSFamily)
	assert.Nil(t, req.OSVersion)
	assert.Nil(t, req.MinDiskGB)
	assert.Nil(t, req.StorageBackend)
}
