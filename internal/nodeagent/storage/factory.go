// Package storage provides a factory for creating storage backend instances.
// This consolidates the initialization logic for different storage types
// (Ceph RBD, QCOW2, LVM) into a single location.
package storage

import (
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	"github.com/ceph/go-ceph/rados"
)

// BackendPair contains the storage and template backends for a node.
// Both are created together because they share configuration and connections.
type BackendPair struct {
	// Storage is the storage backend for disk operations.
	Storage StorageBackend
	// Template is the template backend for VM provisioning.
	Template TemplateBackend
	// Type is the storage backend type (ceph, qcow, lvm).
	Type StorageType
}

// NewBackend creates a BackendPair based on the node agent configuration.
// It initializes the appropriate storage and template managers for the
// configured backend type.
func NewBackend(cfg *config.NodeAgentConfig, logger *slog.Logger) (*BackendPair, error) {
	storageBackend := cfg.StorageBackend
	if storageBackend == "" {
		storageBackend = "ceph"
	}

	switch storageBackend {
	case "qcow":
		return newQCOWBackend(cfg, logger)
	case "ceph":
		return newCephBackend(cfg, logger)
	case "lvm":
		return newLVMBackend(cfg, logger)
	default:
		return nil, fmt.Errorf("unsupported storage backend: %q", storageBackend)
	}
}

// newQCOWBackend creates a QCOW2-based storage backend pair.
func newQCOWBackend(cfg *config.NodeAgentConfig, logger *slog.Logger) (*BackendPair, error) {
	qcowMgr, err := NewQCOWManager(cfg.StoragePath, logger)
	if err != nil {
		return nil, fmt.Errorf("creating QCOW manager: %w", err)
	}

	templatesPath := cfg.StoragePath + "/templates"
	vmsPath := cfg.StoragePath + "/vms"
	qcowTemplateMgr, err := NewQCOWTemplateManager(templatesPath, vmsPath, logger)
	if err != nil {
		return nil, fmt.Errorf("creating QCOW template manager: %w", err)
	}

	logger.Info("initialized QCOW storage backend", "path", cfg.StoragePath)
	return &BackendPair{
		Storage:  qcowMgr,
		Template: qcowTemplateMgr,
		Type:     StorageTypeQCOW,
	}, nil
}

// newCephBackend creates a Ceph RBD-based storage backend pair.
func newCephBackend(cfg *config.NodeAgentConfig, logger *slog.Logger) (*BackendPair, error) {
	rbdMgr, err := NewRBDManager(cfg.CephConf, cfg.CephUser, cfg.CephPool, logger)
	if err != nil {
		return nil, fmt.Errorf("connecting to ceph: %w", err)
	}

	// Create rados connection for template manager
	conn, err := rados.NewConnWithUser(cfg.CephUser)
	if err != nil {
		rbdMgr.Close()
		return nil, fmt.Errorf("creating rados connection for template manager: %w", err)
	}

	if err := conn.ReadConfigFile(cfg.CephConf); err != nil {
		conn.Shutdown()
		rbdMgr.Close()
		return nil, fmt.Errorf("reading ceph config for template manager: %w", err)
	}

	if err := conn.Connect(); err != nil {
		conn.Shutdown()
		rbdMgr.Close()
		return nil, fmt.Errorf("connecting to ceph for template manager: %w", err)
	}

	templateMgr := NewTemplateManager(rbdMgr, conn, logger)

	logger.Info("initialized Ceph RBD storage backend", "pool", cfg.CephPool)
	return &BackendPair{
		Storage:  rbdMgr,
		Template: templateMgr,
		Type:     StorageTypeCEPH,
	}, nil
}

// newLVMBackend creates an LVM thin-provisioned storage backend pair.
func newLVMBackend(cfg *config.NodeAgentConfig, logger *slog.Logger) (*BackendPair, error) {
	lvmMgr, err := NewLVMManager(cfg.LVMVolumeGroup, cfg.LVMThinPool, logger)
	if err != nil {
		return nil, fmt.Errorf("creating LVM manager: %w", err)
	}

	lvmTemplateMgr, err := NewLVMTemplateManager(cfg.LVMVolumeGroup, cfg.LVMThinPool, logger)
	if err != nil {
		return nil, fmt.Errorf("creating LVM template manager: %w", err)
	}

	logger.Info("initialized LVM storage backend", "vg", cfg.LVMVolumeGroup, "thin_pool", cfg.LVMThinPool)
	return &BackendPair{
		Storage:  lvmMgr,
		Template: lvmTemplateMgr,
		Type:     StorageTypeLVM,
	}, nil
}