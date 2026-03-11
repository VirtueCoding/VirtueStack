// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// TemplateStorage abstracts storage operations for template images.
// This interface allows the TemplateService to handle RBD image operations
// without depending directly on Ceph implementation details.
type TemplateStorage interface {
	// ImportTemplate imports a template image from a source path into RBD storage.
	// Returns the RBD image name and snapshot name.
	ImportTemplate(ctx context.Context, name, sourcePath string) (rbdImage, rbdSnapshot string, err error)
	// DeleteTemplate removes a template image from RBD storage.
	DeleteTemplate(ctx context.Context, rbdImage, rbdSnapshot string) error
	// GetTemplateSize returns the size of a template image in bytes.
	GetTemplateSize(ctx context.Context, rbdImage, rbdSnapshot string) (int64, error)
}

// TemplateService provides business logic for managing OS templates.
// Templates are OS images stored in Ceph RBD that can be used for VM provisioning.
type TemplateService struct {
	templateRepo *repository.TemplateRepository
	storage      TemplateStorage
	logger       *slog.Logger
}

// NewTemplateService creates a new TemplateService with the given dependencies.
func NewTemplateService(
	templateRepo *repository.TemplateRepository,
	storage TemplateStorage,
	logger *slog.Logger,
) *TemplateService {
	return &TemplateService{
		templateRepo: templateRepo,
		storage:      storage,
		logger:       logger.With("component", "template-service"),
	}
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
// The source file is imported into RBD storage and a database record is created.
// Parameters:
//   - name: Human-readable name for the template (e.g., "Ubuntu 22.04 LTS")
//   - osFamily: Operating system family (e.g., "linux", "windows")
//   - osVersion: Operating system version (e.g., "22.04", "2022")
//   - sourcePath: Path to the source image file (qcow2, raw, etc.)
func (s *TemplateService) Import(ctx context.Context, name, osFamily, osVersion, sourcePath string) (*models.Template, error) {
	// Verify source file exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("source file not found: %s", sourcePath)
	}

	// Check if template name already exists
	existing, err := s.templateRepo.GetByName(ctx, name)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("template with name %s already exists", name)
	}

	// Import the image into RBD storage
	var rbdImage, rbdSnapshot string
	if s.storage != nil {
		rbdImage, rbdSnapshot, err = s.storage.ImportTemplate(ctx, name, sourcePath)
		if err != nil {
			return nil, fmt.Errorf("importing template to storage: %w", err)
		}
	} else {
		// Generate placeholder values if no storage backend is configured
		rbdImage = fmt.Sprintf("template-%s", uuid.New().String()[:8])
		rbdSnapshot = "v1"
	}

	// Determine minimum disk size from source file
	minDiskGB := 10 // Default minimum
	if s.storage != nil {
		sizeBytes, err := s.storage.GetTemplateSize(ctx, rbdImage, rbdSnapshot)
		if err == nil && sizeBytes > 0 {
			// Convert to GB and add 20% buffer
			minDiskGB = int(float64(sizeBytes)/(1024*1024*1024) * 1.2)
			if minDiskGB < 10 {
				minDiskGB = 10
			}
		}
	}

	// Create template record
	template := &models.Template{
		Name:              name,
		OSFamily:          osFamily,
		OSVersion:         osVersion,
		RBDImage:          rbdImage,
		RBDSnapshot:       rbdSnapshot,
		MinDiskGB:         minDiskGB,
		SupportsCloudInit: true, // Assume cloud-init support for modern images
		IsActive:          true,
		SortOrder:         0,
	}

	if err := s.templateRepo.Create(ctx, template); err != nil {
		// Attempt to clean up the imported image
		if s.storage != nil {
			_ = s.storage.DeleteTemplate(ctx, rbdImage, rbdSnapshot)
		}
		return nil, fmt.Errorf("creating template record: %w", err)
	}

	s.logger.Info("template imported",
		"template_id", template.ID,
		"name", name,
		"os_family", osFamily,
		"os_version", osVersion,
		"rbd_image", rbdImage,
		"rbd_snapshot", rbdSnapshot,
		"min_disk_gb", minDiskGB,
		"source_path", filepath.Base(sourcePath))

	return template, nil
}

// Delete removes a template from the system.
// The template is first deactivated, then removed from storage and database.
// Note: Templates with referencing VMs will have their template_id set to NULL.
func (s *TemplateService) Delete(ctx context.Context, id string) error {
	// Get template to retrieve RBD info
	template, err := s.templateRepo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("template not found: %s", id)
		}
		return fmt.Errorf("getting template: %w", err)
	}

	// Delete from database first
	if err := s.templateRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting template: %w", err)
	}

	// Delete from storage
	if s.storage != nil {
		if err := s.storage.DeleteTemplate(ctx, template.RBDImage, template.RBDSnapshot); err != nil {
			s.logger.Warn("failed to delete template from storage",
				"template_id", id,
				"rbd_image", template.RBDImage,
				"error", err)
			// Don't fail the operation - the database record is already deleted
		}
	}

	s.logger.Info("template deleted",
		"template_id", id,
		"name", template.Name)

	return nil
}

// generateTemplateID generates a unique UUID for a new template.
func generateTemplateID() string {
	return uuid.New().String()
}