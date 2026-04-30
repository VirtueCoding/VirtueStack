// Package nodeagent provides an HTTP health endpoint for storage backend status.
// This is separate from the gRPC server and binds to localhost only for security.
package nodeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"syscall"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/storage"
)

// HealthServerConfig holds configuration for the health HTTP server.
type HealthServerConfig struct {
	// Addr is the address to bind the health HTTP server.
	// Must be localhost for security (e.g., "127.0.0.1:8081").
	Addr string
}

// HealthServer provides an HTTP endpoint for storage backend health checks.
type HealthServer struct {
	config         *HealthServerConfig
	storageBackend storage.StorageBackend
	storageType    storage.StorageType
	logger         *slog.Logger
	server         *http.Server
}

// StorageHealthResponse is the JSON response for /health/storage endpoint.
type StorageHealthResponse struct {
	// Backend is the storage backend type (ceph, qcow, lvm).
	Backend string `json:"backend"`
	// Connected indicates whether the storage backend is healthy.
	Connected bool `json:"connected"`
	// ThinPool contains LVM thin pool stats (only for LVM backend).
	ThinPool *LVMThinPoolHealth `json:"thin_pool,omitempty"`
	// Ceph contains Ceph cluster health (only for Ceph backend).
	Ceph *CephHealth `json:"ceph,omitempty"`
	// Filesystem contains filesystem stats (only for QCOW backend).
	Filesystem *FilesystemHealth `json:"filesystem,omitempty"`
	// Warnings contains any health warnings.
	Warnings []string `json:"warnings"`
}

// LVMThinPoolHealth contains LVM thin pool health metrics.
type LVMThinPoolHealth struct {
	// VG is the volume group name.
	VG string `json:"vg"`
	// Pool is the thin pool LV name.
	Pool string `json:"pool"`
	// DataPercent is the data usage percentage (0-100).
	DataPercent float64 `json:"data_percent"`
	// MetadataPercent is the metadata usage percentage (0-100).
	MetadataPercent float64 `json:"metadata_percent"`
	// TotalBytes is the total pool size in bytes.
	TotalBytes int64 `json:"total_bytes"`
	// UsedBytes is the used pool space in bytes.
	UsedBytes int64 `json:"used_bytes"`
}

// CephHealth contains Ceph cluster health information.
type CephHealth struct {
	// Health is the overall cluster health status (HEALTH_OK, HEALTH_WARN, HEALTH_ERR).
	Health string `json:"health"`
	// Details contains additional health details from ceph -s.
	Details string `json:"details,omitempty"`
}

// FilesystemHealth contains filesystem stats for QCOW storage.
type FilesystemHealth struct {
	// Path is the storage path.
	Path string `json:"path"`
	// TotalBytes is the total filesystem size in bytes.
	TotalBytes int64 `json:"total_bytes"`
	// UsedBytes is the used filesystem space in bytes.
	UsedBytes int64 `json:"used_bytes"`
	// AvailableBytes is the available space in bytes.
	AvailableBytes int64 `json:"available_bytes"`
	// UsedPercent is the disk usage percentage (0-100).
	UsedPercent float64 `json:"used_percent"`
	// InodePercent is the inode usage percentage (0-100).
	InodePercent float64 `json:"inode_percent"`
}

// NewHealthServer creates a new health HTTP server.
func NewHealthServer(cfg *HealthServerConfig, storageBackend storage.StorageBackend, storageType storage.StorageType, logger *slog.Logger) *HealthServer {
	return &HealthServer{
		config:         cfg,
		storageBackend: storageBackend,
		storageType:    storageType,
		logger:         logger.With("component", "health-server"),
	}
}

// Start starts the health HTTP server. It blocks until the context is cancelled.
func (s *HealthServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/storage", s.handleStorageHealth)

	// Parse the address to ensure it's localhost-only
	host, port, err := net.SplitHostPort(s.config.Addr)
	if err != nil {
		return fmt.Errorf("parsing health address %q: %w", s.config.Addr, err)
	}

	// Security: ensure the host is localhost
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		s.logger.Warn("health server address is not localhost, forcing localhost binding for security", "requested", host)
		host = "127.0.0.1"
	}

	listenAddr := net.JoinHostPort(host, port)

	s.server = &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errChan := make(chan error, 1)
	go func() {
		s.logger.Info("starting health HTTP server", "address", listenAddr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("health HTTP server error: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("context cancelled, stopping health HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("error shutting down health HTTP server", "error", err)
		}
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

// handleStorageHealth handles the /health/storage HTTP endpoint.
func (s *HealthServer) handleStorageHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := &StorageHealthResponse{
		Backend:   string(s.storageType),
		Warnings:  []string{},
		Connected: false,
	}

	switch s.storageType {
	case storage.StorageTypeCEPH:
		s.populateCephHealth(response)
	case storage.StorageTypeQCOW:
		s.populateQCOWHealth(response)
	case storage.StorageTypeLVM:
		s.populateLVMHealth(response)
	default:
		response.Warnings = append(response.Warnings, fmt.Sprintf("unknown storage type: %s", s.storageType))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("error encoding health response", "error", err)
	}
}

// populateCephHealth populates health information for Ceph storage backend.
func (s *HealthServer) populateCephHealth(response *StorageHealthResponse) {
	rbdMgr, ok := s.storageBackend.(*storage.RBDManager)
	if !ok {
		response.Warnings = append(response.Warnings, "storage backend is not RBDManager")
		return
	}

	// Check connection
	response.Connected = rbdMgr.IsConnected()
	if !response.Connected {
		response.Warnings = append(response.Warnings, "ceph cluster not connected")
		return
	}

	// Get ceph status
	cephHealth := &CephHealth{}

	// Run ceph -s to get cluster health
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ceph", "status", "--format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		cephHealth.Health = "UNKNOWN"
		cephHealth.Details = fmt.Sprintf("failed to get ceph status: %v", err)
		response.Warnings = append(response.Warnings, fmt.Sprintf("ceph status command failed: %v", err))
	} else {
		// Parse ceph status JSON
		var status struct {
			Health struct {
				Status string `json:"status"`
			} `json:"health"`
		}
		if parseErr := json.Unmarshal(output, &status); parseErr != nil {
			cephHealth.Health = "UNKNOWN"
			cephHealth.Details = fmt.Sprintf("failed to parse ceph status: %v", parseErr)
		} else {
			cephHealth.Health = status.Health.Status
		}
	}

	// Add warnings for non-OK health
	if cephHealth.Health != "HEALTH_OK" {
		response.Warnings = append(response.Warnings, fmt.Sprintf("ceph cluster health: %s", cephHealth.Health))
	}

	response.Ceph = cephHealth
}

// populateQCOWHealth populates health information for QCOW storage backend.
func (s *HealthServer) populateQCOWHealth(response *StorageHealthResponse) {
	qcowMgr, ok := s.storageBackend.(*storage.QCOWManager)
	if !ok {
		response.Warnings = append(response.Warnings, "storage backend is not QCOWManager")
		return
	}

	// QCOW is file-based, so it's always "connected" if we can access the path
	response.Connected = true

	// Get the base path from QCOW manager
	// Use reflection or interface to get basePath, or use GetPoolStats
	ctx := context.Background()
	poolStats, err := qcowMgr.GetPoolStats(ctx)
	if err != nil {
		response.Warnings = append(response.Warnings, fmt.Sprintf("failed to get pool stats: %v", err))
		response.Connected = false
		return
	}

	// Get filesystem stats including inode usage
	basePath := qcowMgr.BasePath()

	var stat syscall.Statfs_t
	if err := syscall.Statfs(basePath, &stat); err != nil {
		response.Warnings = append(response.Warnings, fmt.Sprintf("failed to get filesystem stats: %v", err))
	} else {
		totalBytes := int64(stat.Blocks) * int64(stat.Bsize)
		availBytes := int64(stat.Bavail) * int64(stat.Bsize)
		usedBytes := totalBytes - availBytes
		usedPercent := float64(usedBytes) / float64(totalBytes) * 100

		// Calculate inode usage
		totalInodes := int64(stat.Files)
		freeInodes := int64(stat.Ffree)
		usedInodes := totalInodes - freeInodes
		inodePercent := float64(0)
		if totalInodes > 0 {
			inodePercent = float64(usedInodes) / float64(totalInodes) * 100
		}

		response.Filesystem = &FilesystemHealth{
			Path:           basePath,
			TotalBytes:     totalBytes,
			UsedBytes:      usedBytes,
			AvailableBytes: availBytes,
			UsedPercent:    usedPercent,
			InodePercent:   inodePercent,
		}

		// Add warnings for high usage
		if usedPercent > 80 {
			response.Warnings = append(response.Warnings, fmt.Sprintf("disk usage high: %.1f%%", usedPercent))
		}
		if inodePercent > 80 {
			response.Warnings = append(response.Warnings, fmt.Sprintf("inode usage high: %.1f%%", inodePercent))
		}
	}

	// Use pool stats for consistency
	if response.Filesystem != nil {
		response.Filesystem.TotalBytes = poolStats.Total
		response.Filesystem.UsedBytes = poolStats.Used
		response.Filesystem.AvailableBytes = poolStats.Free
		if poolStats.Total > 0 {
			response.Filesystem.UsedPercent = float64(poolStats.Used) / float64(poolStats.Total) * 100
		}
	}
}

// populateLVMHealth populates health information for LVM storage backend.
func (s *HealthServer) populateLVMHealth(response *StorageHealthResponse) {
	lvmMgr, ok := s.storageBackend.(*storage.LVMManager)
	if !ok {
		response.Warnings = append(response.Warnings, "storage backend is not LVMManager")
		return
	}

	ctx := context.Background()

	// Get thin pool stats
	dataPercent, metadataPercent, err := lvmMgr.ThinPoolStats(ctx)
	if err != nil {
		response.Warnings = append(response.Warnings, fmt.Sprintf("failed to get thin pool stats: %v", err))
		response.Connected = false
		return
	}

	response.Connected = true

	// Get pool stats for total/used bytes
	poolStats, err := lvmMgr.GetPoolStats(ctx)
	if err != nil {
		response.Warnings = append(response.Warnings, fmt.Sprintf("failed to get pool stats: %v", err))
	}

	response.ThinPool = &LVMThinPoolHealth{
		VG:              lvmMgr.VolumeGroup(),
		Pool:            lvmMgr.ThinPoolName(),
		DataPercent:     dataPercent,
		MetadataPercent: metadataPercent,
	}

	if poolStats != nil {
		response.ThinPool.TotalBytes = poolStats.Total
		response.ThinPool.UsedBytes = poolStats.Used
	}

	// Add warnings for high usage
	if dataPercent >= 80 {
		response.Warnings = append(response.Warnings, fmt.Sprintf("thin pool data usage high: %.1f%%", dataPercent))
	}
	if dataPercent >= 95 {
		response.Warnings = append(response.Warnings, fmt.Sprintf("thin pool data usage critical: %.1f%% >= 95%%", dataPercent))
	}
	if metadataPercent >= 50 {
		response.Warnings = append(response.Warnings, fmt.Sprintf("thin pool metadata usage high: %.1f%%", metadataPercent))
	}
	if metadataPercent >= 70 {
		response.Warnings = append(response.Warnings, fmt.Sprintf("thin pool metadata usage critical: %.1f%% >= 70%%", metadataPercent))
	}
}