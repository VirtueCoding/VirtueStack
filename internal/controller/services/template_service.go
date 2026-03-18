// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

const (
	// StorageBackendCeph indicates Ceph/RBD storage backend.
	StorageBackendCeph = "ceph"
	// StorageBackendQcow indicates local QCOW2 file-based storage.
	StorageBackendQcow = "qcow"
	// DefaultMinDiskGB is the default minimum disk size in GB for templates.
	DefaultMinDiskGB = 10
	// MinDiskSizeOverheadFactor is the multiplier applied to the actual template size.
	MinDiskSizeOverheadFactor = 1.2
)

// TemplateStorage abstracts storage operations for template images.
// This interface allows the TemplateService to handle different storage backends
// (Ceph RBD or QCOW2 file-based) without depending on implementation details.
type TemplateStorage interface {
	// ImportTemplate imports a template image from a source path into storage.
	// Returns the template reference and snapshot reference (for Ceph) or file path (for QCOW).
	ImportTemplate(ctx context.Context, name, sourcePath string) (templateRef string, snapshotRef string, err error)
	// DeleteTemplate removes a template image from storage.
	DeleteTemplate(ctx context.Context, templateRef, snapshotRef string) error
	// GetTemplateSize returns the size of a template image in bytes.
	GetTemplateSize(ctx context.Context, templateRef, snapshotRef string) (int64, error)
	// GetStorageType returns the storage backend type: "ceph" or "qcow".
	GetStorageType() string
}

// TemplateService provides business logic for managing OS templates.
// Templates are OS images stored in Ceph RBD or QCOW2 that can be used for VM provisioning.
type TemplateService struct {
	templateRepo   *repository.TemplateRepository
	storageMap     map[string]TemplateStorage
	defaultStorage TemplateStorage
	logger         *slog.Logger
}

// NewTemplateService creates a new TemplateService with the given dependencies.
// For backward compatibility, if only one storage backend is provided, it becomes the default.
func NewTemplateService(
	templateRepo *repository.TemplateRepository,
	storage TemplateStorage,
	logger *slog.Logger,
) *TemplateService {
	storageMap := make(map[string]TemplateStorage)
	if storage != nil {
		storageMap[storage.GetStorageType()] = storage
	}

	return &TemplateService{
		templateRepo:   templateRepo,
		storageMap:     storageMap,
		defaultStorage: storage,
		logger:         logger.With("component", "template-service"),
	}
}

// NewTemplateServiceWithBackends creates a new TemplateService with multiple storage backends.
func NewTemplateServiceWithBackends(
	templateRepo *repository.TemplateRepository,
	storageBackends map[string]TemplateStorage,
	defaultBackend string,
	logger *slog.Logger,
) *TemplateService {
	var defaultStorage TemplateStorage
	if defaultBackend != "" {
		defaultStorage = storageBackends[defaultBackend]
	} else if len(storageBackends) > 0 {
		for _, s := range storageBackends {
			defaultStorage = s
			break
		}
	}

	return &TemplateService{
		templateRepo:   templateRepo,
		storageMap:     storageBackends,
		defaultStorage: defaultStorage,
		logger:         logger.With("component", "template-service"),
	}
}

// GetStorage returns the appropriate storage backend for the given type.
func (s *TemplateService) GetStorage(storageType string) TemplateStorage {
	if storage, ok := s.storageMap[storageType]; ok {
		return storage
	}
	return s.defaultStorage
}

// ListActive returns all active templates ordered by sort_order.
// This is the primary method for displaying templates to customers during VM creation.
func (s *TemplateService) ListActive(ctx context.Context) ([]models.Template, error) {
	templates, err := s.templateRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing active templates: %w", err)
	}
	return templates, nil
}

// List returns a paginated list of templates with optional filtering.
// Supports filtering by active status, OS family, and cloud-init support.
func (s *TemplateService) List(ctx context.Context, filter repository.TemplateListFilter) ([]models.Template, int, error) {
	templates, total, err := s.templateRepo.List(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("listing templates: %w", err)
	}
	return templates, total, nil
}

func (s *TemplateService) Create(ctx context.Context, template *models.Template) error {
	if template == nil {
		return fmt.Errorf("template is required")
	}
	if template.Name == "" || template.OSFamily == "" || template.OSVersion == "" {
		return fmt.Errorf("missing required template fields")
	}

	storageBackend := template.StorageBackend
	if storageBackend == "" {
		storageBackend = StorageBackendCeph
	}

	if storageBackend == StorageBackendCeph {
		if template.RBDImage == "" || template.RBDSnapshot == "" {
			return fmt.Errorf("rbd_image and rbd_snapshot are required for ceph storage")
		}
	} else if storageBackend == StorageBackendQcow {
		if template.FilePath == "" {
			return fmt.Errorf("file_path is required for qcow storage")
		}
	}

	if template.MinDiskGB < 1 {
		return fmt.Errorf("min_disk_gb must be at least 1")
	}
	if template.SortOrder < 0 {
		return fmt.Errorf("sort_order cannot be negative")
	}

	existing, err := s.templateRepo.GetByName(ctx, template.Name)
	if err == nil && existing != nil {
		return fmt.Errorf("template with name %s already exists", template.Name)
	}

	if err := s.templateRepo.Create(ctx, template); err != nil {
		return fmt.Errorf("creating template: %w", err)
	}

	s.logger.Info("template created", "template_id", template.ID, "name", template.Name, "storage_backend", storageBackend)
	return nil
}

// GetByID retrieves a template by its UUID.
// Returns ErrNotFound if the template doesn't exist.
func (s *TemplateService) GetByID(ctx context.Context, id string) (*models.Template, error) {
	template, err := s.templateRepo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("template not found: %s", id)
		}
		return nil, fmt.Errorf("getting template: %w", err)
	}
	return template, nil
}

// Import imports a new OS template from a source file.
// The source file is imported into storage and a database record is created.
// Parameters:
//   - name: Human-readable name for the template (e.g., "Ubuntu 22.04 LTS")
//   - osFamily: Operating system family (e.g., "linux", "windows")
//   - osVersion: Operating system version (e.g., "22.04", "2022")
//   - sourcePath: Path to the source image file (qcow2, raw, etc.)
//   - storageBackend: Storage backend type ("ceph" or "qcow")
func (s *TemplateService) Import(ctx context.Context, name, osFamily, osVersion, sourcePath, storageBackend string) (*models.Template, error) {
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("source file not found: %s", sourcePath)
	}

	existing, err := s.templateRepo.GetByName(ctx, name)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("template with name %s already exists", name)
	}

	if storageBackend == "" {
		storageBackend = StorageBackendCeph
	}

	storage := s.GetStorage(storageBackend)
	if storage == nil {
		return nil, fmt.Errorf("storage backend %s is not configured", storageBackend)
	}

	var templateRef, snapshotRef string
	templateRef, snapshotRef, err = storage.ImportTemplate(ctx, name, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("importing template to storage: %w", err)
	}

	minDiskGB := DefaultMinDiskGB
	sizeBytes, err := storage.GetTemplateSize(ctx, templateRef, snapshotRef)
	if err == nil && sizeBytes > 0 {
		minDiskGB = int(float64(sizeBytes) / (1024 * 1024 * 1024) * MinDiskSizeOverheadFactor)
		if minDiskGB < DefaultMinDiskGB {
			minDiskGB = DefaultMinDiskGB
		}
	}

	template := &models.Template{
		Name:              name,
		OSFamily:          osFamily,
		OSVersion:         osVersion,
		MinDiskGB:         minDiskGB,
		SupportsCloudInit: true,
		IsActive:          true,
		SortOrder:         0,
		StorageBackend:    storageBackend,
	}

	if storageBackend == StorageBackendCeph {
		template.RBDImage = templateRef
		template.RBDSnapshot = snapshotRef
	} else {
		template.FilePath = templateRef
	}

	if err := s.templateRepo.Create(ctx, template); err != nil {
		_ = storage.DeleteTemplate(ctx, templateRef, snapshotRef)
		return nil, fmt.Errorf("creating template record: %w", err)
	}

	s.logger.Info("template imported",
		"template_id", template.ID,
		"name", name,
		"os_family", osFamily,
		"os_version", osVersion,
		"storage_backend", storageBackend,
		"template_ref", templateRef,
		"snapshot_ref", snapshotRef,
		"min_disk_gb", minDiskGB,
		"source_path", filepath.Base(sourcePath))

	return template, nil
}

// Delete removes a template from the system.
// The template is first deactivated, then removed from storage and database.
// Note: Templates with referencing VMs will have their template_id set to NULL.
func (s *TemplateService) Delete(ctx context.Context, id string) error {
	template, err := s.templateRepo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("template not found: %s", id)
		}
		return fmt.Errorf("getting template: %w", err)
	}

	if err := s.templateRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting template: %w", err)
	}

	storageBackend := template.StorageBackend
	if storageBackend == "" {
		storageBackend = StorageBackendCeph
	}

	storage := s.GetStorage(storageBackend)
	if storage != nil {
		var templateRef, snapshotRef string
		if storageBackend == StorageBackendCeph {
			templateRef = template.RBDImage
			snapshotRef = template.RBDSnapshot
		} else {
			templateRef = template.FilePath
			snapshotRef = ""
		}

		if err := storage.DeleteTemplate(ctx, templateRef, snapshotRef); err != nil {
			s.logger.Warn("failed to delete template from storage",
				"template_id", id,
				"storage_backend", storageBackend,
				"template_ref", templateRef,
				"error", err)
		}
	}

	s.logger.Info("template deleted",
		"template_id", id,
		"name", template.Name,
		"storage_backend", storageBackend)

	return nil
}

// Update applies partial updates to a template and increments its version.
// Only non-nil fields in the update request are modified.
// The version is automatically incremented by the repository.
func (s *TemplateService) Update(ctx context.Context, id string, req *models.TemplateUpdateRequest) (*models.Template, error) {
	template, err := s.templateRepo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("template not found: %s", id)
		}
		return nil, fmt.Errorf("getting template: %w", err)
	}

	if req.Name != nil {
		if *req.Name == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		existing, err := s.templateRepo.GetByName(ctx, *req.Name)
		if err == nil && existing != nil && existing.ID != id {
			return nil, fmt.Errorf("template with name %s already exists", *req.Name)
		}
		template.Name = *req.Name
	}
	if req.OSFamily != nil {
		if *req.OSFamily == "" {
			return nil, fmt.Errorf("os_family cannot be empty")
		}
		template.OSFamily = *req.OSFamily
	}
	if req.OSVersion != nil {
		if *req.OSVersion == "" {
			return nil, fmt.Errorf("os_version cannot be empty")
		}
		template.OSVersion = *req.OSVersion
	}
	if req.RBDImage != nil {
		template.RBDImage = *req.RBDImage
	}
	if req.RBDSnapshot != nil {
		template.RBDSnapshot = *req.RBDSnapshot
	}
	if req.MinDiskGB != nil {
		if *req.MinDiskGB < 1 {
			return nil, fmt.Errorf("min_disk_gb must be at least 1")
		}
		template.MinDiskGB = *req.MinDiskGB
	}
	if req.SupportsCloudInit != nil {
		template.SupportsCloudInit = *req.SupportsCloudInit
	}
	if req.IsActive != nil {
		template.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		if *req.SortOrder < 0 {
			return nil, fmt.Errorf("sort_order cannot be negative")
		}
		template.SortOrder = *req.SortOrder
	}
	if req.Description != nil {
		template.Description = *req.Description
	}
	if req.StorageBackend != nil {
		template.StorageBackend = *req.StorageBackend
	}
	if req.FilePath != nil {
		template.FilePath = *req.FilePath
	}

	if err := s.templateRepo.Update(ctx, template); err != nil {
		return nil, fmt.Errorf("updating template: %w", err)
	}

	s.logger.Info("template updated",
		"template_id", template.ID,
		"name", template.Name,
		"version", template.Version)

	return template, nil
}
