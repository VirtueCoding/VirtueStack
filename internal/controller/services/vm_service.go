// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// TaskPublisher abstracts NATS task publishing for async operations.
// This interface allows the VMService to publish tasks without depending
// directly on NATS implementation details.
type TaskPublisher interface {
	// PublishTask publishes a task to be processed asynchronously.
	// Returns the task ID for polling/status checking.
	PublishTask(ctx context.Context, taskType string, payload map[string]any) (string, error)
}

// IPAllocator abstracts IP Address Management operations.
// This interface provides methods for allocating and releasing IP addresses.
type IPAllocator interface {
	AllocateIPv4(ctx context.Context, locationID, vmID, customerID string) (*models.IPAddress, error)
	ReleaseIPsByVM(ctx context.Context, vmID string) error
	GetIPsByVM(ctx context.Context, vmID string) ([]models.IPAddress, error)
}

// VMService provides VM lifecycle orchestration for VirtueStack.
// It handles VM creation, start/stop, resize, reinstall, and deletion,
// coordinating between the database, node agents, and async task system.
type VMService struct {
	vmRepo        *repository.VMRepository
	nodeRepo      *repository.NodeRepository
	ipRepo        *repository.IPRepository
	planRepo      *repository.PlanRepository
	templateRepo  *repository.TemplateRepository
	taskRepo      *repository.TaskRepository
	taskPublisher TaskPublisher
	nodeAgent     NodeAgentClient
	ipamService   IPAllocator
	encryptionKey string // For encrypting root passwords
	logger        *slog.Logger
}

// NewVMService creates a new VMService with the given dependencies.
func NewVMService(
	vmRepo *repository.VMRepository,
	nodeRepo *repository.NodeRepository,
	ipRepo *repository.IPRepository,
	planRepo *repository.PlanRepository,
	templateRepo *repository.TemplateRepository,
	taskRepo *repository.TaskRepository,
	taskPublisher TaskPublisher,
	nodeAgent NodeAgentClient,
	ipamService IPAllocator,
	encryptionKey string,
	logger *slog.Logger,
) *VMService {
	return &VMService{
		vmRepo:        vmRepo,
		nodeRepo:      nodeRepo,
		ipRepo:        ipRepo,
		planRepo:      planRepo,
		templateRepo:  templateRepo,
		taskRepo:      taskRepo,
		taskPublisher: taskPublisher,
		nodeAgent:     nodeAgent,
		ipamService:   ipamService,
		encryptionKey: encryptionKey,
		logger:        logger.With("component", "vm-service"),
	}
}

// CreateVM creates a new virtual machine.
// This is an async operation that publishes a vm.create task.
// Returns the created VM record and task ID for polling.
func (s *VMService) CreateVM(ctx context.Context, req *models.VMCreateRequest, customerID string) (*models.VM, string, error) {
	// 1. Validate plan exists and is active
	plan, err := s.planRepo.GetByID(ctx, req.PlanID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, "", fmt.Errorf("plan not found: %s", req.PlanID)
		}
		return nil, "", fmt.Errorf("getting plan: %w", err)
	}
	if !plan.IsActive {
		return nil, "", fmt.Errorf("plan %s is not active", req.PlanID)
	}

	// 2. Validate template exists
	template, err := s.templateRepo.GetByID(ctx, req.TemplateID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, "", fmt.Errorf("template not found: %s", req.TemplateID)
		}
		return nil, "", fmt.Errorf("getting template: %w", err)
	}

	// 3. Check template disk requirements
	if plan.DiskGB < template.MinDiskGB {
		return nil, "", fmt.Errorf("plan disk size (%d GB) is less than template minimum (%d GB)",
			plan.DiskGB, template.MinDiskGB)
	}

	// 4. Find least loaded node (using default location if not specified)
	locationID := ""
	if req.LocationID != nil {
		locationID = *req.LocationID
	}

	var node *models.Node
	if locationID != "" {
		node, err = s.nodeRepo.GetLeastLoadedNode(ctx, locationID, plan.StorageBackend)
		if err != nil {
			if sharederrors.Is(err, sharederrors.ErrNotFound) {
				return nil, "", fmt.Errorf("no available nodes in location %s with storage backend %s", locationID, plan.StorageBackend)
			}
			return nil, "", fmt.Errorf("finding node: %w", err)
		}
	} else {
		nodes, _, err := s.nodeRepo.List(ctx, models.NodeListFilter{Status: strPtr(models.NodeStatusOnline)})
		if err != nil {
			return nil, "", fmt.Errorf("listing nodes: %w", err)
		}
		var filteredNodes []models.Node
		for _, n := range nodes {
			if n.StorageBackend == plan.StorageBackend {
				filteredNodes = append(filteredNodes, n)
			}
		}
		if len(filteredNodes) == 0 {
			return nil, "", fmt.Errorf("no available nodes with storage backend %s", plan.StorageBackend)
		}
		node = &filteredNodes[0]
		for i := range filteredNodes {
			availableMemory := filteredNodes[i].TotalMemoryMB - filteredNodes[i].AllocatedMemoryMB
			bestMemory := node.TotalMemoryMB - node.AllocatedMemoryMB
			if availableMemory > bestMemory {
				node = &filteredNodes[i]
			}
		}
	}

	// 5. Generate MAC address
	macAddress, err := crypto.GenerateMACAddress()
	if err != nil {
		return nil, "", fmt.Errorf("generating MAC address: %w", err)
	}

	// 6. Encrypt root password
	encryptedPassword, err := crypto.Encrypt(req.Password, s.encryptionKey)
	if err != nil {
		return nil, "", fmt.Errorf("encrypting password: %w", err)
	}

	// 7. Generate VM ID and libvirt domain name
	vmID := uuid.New().String()
	libvirtDomainName := generateLibvirtDomainName(req.Hostname, vmID)

	// 8. Create VM record in database
	vm := &models.VM{
		ID:                    vmID,
		CustomerID:            customerID,
		NodeID:                &node.ID,
		PlanID:                plan.ID,
		Hostname:              req.Hostname,
		Status:                models.VMStatusProvisioning,
		VCPU:                  plan.VCPU,
		MemoryMB:              plan.MemoryMB,
		DiskGB:                plan.DiskGB,
		PortSpeedMbps:         plan.PortSpeedMbps,
		BandwidthLimitGB:      plan.BandwidthLimitGB,
		BandwidthUsedBytes:    0,
		BandwidthResetAt:      time.Now().UTC(),
		MACAddress:            macAddress,
		TemplateID:            &template.ID,
		LibvirtDomainName:     &libvirtDomainName,
		RootPasswordEncrypted: &encryptedPassword,
		WHMCSServiceID:        req.WHMCSServiceID,
		StorageBackend:        plan.StorageBackend,
	}

	if err := s.vmRepo.Create(ctx, vm); err != nil {
		return nil, "", fmt.Errorf("creating VM record: %w", err)
	}

	// 9. Allocate IP addresses (if IPAM service is available)
	var ipv4Address *models.IPAddress
	if s.ipamService != nil && locationID != "" {
		ipv4Address, err = s.ipamService.AllocateIPv4(ctx, locationID, vm.ID, customerID)
		if err != nil {
			s.logger.Warn("failed to allocate IPv4", "vm_id", vm.ID, "error", err)
			// Continue without IP - the task worker can handle this
		}
	}

	// 10. Publish vm.create task
	taskPayload := map[string]any{
		"vm_id":                 vm.ID,
		"node_id":               node.ID,
		"hostname":              vm.Hostname,
		"vcpu":                  vm.VCPU,
		"memory_mb":             vm.MemoryMB,
		"disk_gb":               vm.DiskGB,
		"template_id":           template.ID,
		"template_rbd_image":    template.RBDImage,
		"template_rbd_snapshot": template.RBDSnapshot,
		"mac_address":           vm.MACAddress,
		"port_speed_mbps":       vm.PortSpeedMbps,
		"bandwidth_limit_gb":    vm.BandwidthLimitGB,
		"ssh_keys":              req.SSHKeys,
		"ceph_pool":             node.CephPool,
		"storage_backend":       plan.StorageBackend,
		"storage_path":          node.StoragePath,
	}

	if ipv4Address != nil {
		taskPayload["ipv4_address"] = ipv4Address.Address
	}

	taskID, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeVMCreate, taskPayload)
	if err != nil {
		// Attempt to clean up
		_ = s.vmRepo.SoftDelete(ctx, vm.ID)
		if ipv4Address != nil && s.ipamService != nil {
			_ = s.ipamService.ReleaseIPsByVM(ctx, vm.ID)
		}
		return nil, "", fmt.Errorf("publishing create task: %w", err)
	}

	s.logger.Info("VM creation initiated",
		"vm_id", vm.ID,
		"task_id", taskID,
		"customer_id", customerID,
		"node_id", node.ID,
		"hostname", vm.Hostname)

	return vm, taskID, nil
}

// StartVM starts a stopped or suspended VM.
// This is a synchronous operation that calls the node agent directly.
func (s *VMService) StartVM(ctx context.Context, vmID, customerID string, isAdmin bool) error {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return err
	}

	// Verify status allows starting
	if vm.Status != models.VMStatusStopped && vm.Status != models.VMStatusSuspended {
		return fmt.Errorf("cannot start VM in status %s", vm.Status)
	}

	// Check if node is assigned
	if vm.NodeID == nil {
		return fmt.Errorf("VM has no node assigned")
	}

	// Call node agent to start VM
	if err := s.nodeAgent.StartVM(ctx, *vm.NodeID, vm.ID); err != nil {
		return fmt.Errorf("starting VM on node agent: %w", err)
	}

	// Update status
	if err := s.vmRepo.UpdateStatus(ctx, vm.ID, models.VMStatusRunning); err != nil {
		s.logger.Warn("failed to update VM status after start", "vm_id", vm.ID, "error", err)
	}

	s.logger.Info("VM started", "vm_id", vm.ID, "customer_id", customerID)
	return nil
}

// StopVM stops a running VM.
// If force is true, performs a hard power-off; otherwise graceful ACPI shutdown.
func (s *VMService) StopVM(ctx context.Context, vmID, customerID string, force bool, isAdmin bool) error {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return err
	}

	// Verify status allows stopping
	if vm.Status != models.VMStatusRunning {
		return fmt.Errorf("cannot stop VM in status %s", vm.Status)
	}

	// Check if node is assigned
	if vm.NodeID == nil {
		return fmt.Errorf("VM has no node assigned")
	}

	// Call appropriate stop method
	if force {
		if err := s.nodeAgent.ForceStopVM(ctx, *vm.NodeID, vm.ID); err != nil {
			return fmt.Errorf("force stopping VM on node agent: %w", err)
		}
	} else {
		// Graceful shutdown with 120 second timeout
		if err := s.nodeAgent.StopVM(ctx, *vm.NodeID, vm.ID, 120); err != nil {
			return fmt.Errorf("stopping VM on node agent: %w", err)
		}
	}

	// Update status
	if err := s.vmRepo.UpdateStatus(ctx, vm.ID, models.VMStatusStopped); err != nil {
		s.logger.Warn("failed to update VM status after stop", "vm_id", vm.ID, "error", err)
	}

	s.logger.Info("VM stopped", "vm_id", vm.ID, "force", force, "customer_id", customerID)
	return nil
}

// RestartVM restarts a VM by stopping and starting it.
func (s *VMService) RestartVM(ctx context.Context, vmID, customerID string, isAdmin bool) error {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return err
	}

	// Verify status allows restart
	if vm.Status != models.VMStatusRunning {
		return fmt.Errorf("cannot restart VM in status %s", vm.Status)
	}

	// Check if node is assigned
	if vm.NodeID == nil {
		return fmt.Errorf("VM has no node assigned")
	}

	// Graceful shutdown with 60 second timeout
	if err := s.nodeAgent.StopVM(ctx, *vm.NodeID, vm.ID, 60); err != nil {
		s.logger.Warn("graceful stop failed during restart, attempting force stop", "vm_id", vm.ID, "error", err)
		if err := s.nodeAgent.ForceStopVM(ctx, *vm.NodeID, vm.ID); err != nil {
			return fmt.Errorf("force stopping VM during restart: %w", err)
		}
	}

	// Start the VM
	if err := s.nodeAgent.StartVM(ctx, *vm.NodeID, vm.ID); err != nil {
		return fmt.Errorf("starting VM during restart: %w", err)
	}

	s.logger.Info("VM restarted", "vm_id", vm.ID, "customer_id", customerID)
	return nil
}

// DeleteVM deletes a VM.
// This is an async operation that publishes a vm.delete task.
// The VM is soft-deleted immediately, and cleanup happens asynchronously.
func (s *VMService) DeleteVM(ctx context.Context, vmID, customerID string, isAdmin bool) (string, error) {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return "", err
	}

	// Check if already deleted
	if vm.IsDeleted() {
		return "", fmt.Errorf("VM is already deleted")
	}

	// Check if node is assigned
	nodeID := ""
	if vm.NodeID != nil {
		nodeID = *vm.NodeID
	}

	// Publish vm.delete task
	taskPayload := map[string]any{
		"vm_id":    vm.ID,
		"node_id":  nodeID,
		"hostname": vm.Hostname,
	}

	taskID, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeVMDelete, taskPayload)
	if err != nil {
		return "", fmt.Errorf("publishing delete task: %w", err)
	}

	// Soft delete the VM record
	if err := s.vmRepo.SoftDelete(ctx, vm.ID); err != nil {
		s.logger.Warn("failed to soft delete VM", "vm_id", vm.ID, "error", err)
	}

	// Release IP addresses
	if s.ipamService != nil {
		if err := s.ipamService.ReleaseIPsByVM(ctx, vm.ID); err != nil {
			s.logger.Warn("failed to release IPs during VM deletion", "vm_id", vm.ID, "error", err)
		}
	}

	s.logger.Info("VM deletion initiated",
		"vm_id", vm.ID,
		"task_id", taskID,
		"customer_id", customerID)

	return taskID, nil
}

// ReinstallVM reinstalls a VM with a new template.
// This is an async operation that publishes a vm.reinstall task.
// Returns the updated VM record and task ID for polling.
func (s *VMService) ReinstallVM(ctx context.Context, vmID, templateID, customerID string, password string, isAdmin bool) (*models.VM, string, error) {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return nil, "", err
	}

	// Validate new template
	template, err := s.templateRepo.GetByID(ctx, templateID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, "", fmt.Errorf("template not found: %s", templateID)
		}
		return nil, "", fmt.Errorf("getting template: %w", err)
	}

	// Check template disk requirements
	if vm.DiskGB < template.MinDiskGB {
		return nil, "", fmt.Errorf("VM disk size (%d GB) is less than template minimum (%d GB)",
			vm.DiskGB, template.MinDiskGB)
	}

	// Check if node is assigned
	if vm.NodeID == nil {
		return nil, "", fmt.Errorf("VM has no node assigned")
	}

	// Encrypt new password
	encryptedPassword, err := crypto.Encrypt(password, s.encryptionKey)
	if err != nil {
		return nil, "", fmt.Errorf("encrypting password: %w", err)
	}

	// Update VM record with new template and password
	vm.TemplateID = &template.ID
	vm.RootPasswordEncrypted = &encryptedPassword
	vm.Status = models.VMStatusReinstalling

	// Publish vm.reinstall task
	taskPayload := map[string]any{
		"vm_id":                 vm.ID,
		"node_id":               *vm.NodeID,
		"hostname":              vm.Hostname,
		"template_id":           template.ID,
		"template_rbd_image":    template.RBDImage,
		"template_rbd_snapshot": template.RBDSnapshot,
		"vcpu":                  vm.VCPU,
		"memory_mb":             vm.MemoryMB,
		"disk_gb":               vm.DiskGB,
		"mac_address":           vm.MACAddress,
	}

	taskID, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeVMReinstall, taskPayload)
	if err != nil {
		return nil, "", fmt.Errorf("publishing reinstall task: %w", err)
	}

	s.logger.Info("VM reinstall initiated",
		"vm_id", vm.ID,
		"task_id", taskID,
		"template_id", templateID,
		"customer_id", customerID)

	return vm, taskID, nil
}

// ResizeVM resizes a VM's resources.
// This can be done synchronously for CPU/memory or may require async for disk.
func (s *VMService) ResizeVM(ctx context.Context, vmID, customerID string, newVcpu, newMemoryMB, newDiskGB int, isAdmin bool) error {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return err
	}

	// Validate resources are being changed
	if newVcpu == 0 && newMemoryMB == 0 && newDiskGB == 0 {
		return fmt.Errorf("no resize parameters provided")
	}

	// Use current values for any unspecified parameters
	if newVcpu == 0 {
		newVcpu = vm.VCPU
	}
	if newMemoryMB == 0 {
		newMemoryMB = vm.MemoryMB
	}
	if newDiskGB == 0 {
		newDiskGB = vm.DiskGB
	}

	// Validate against plan limits (admins can override)
	if !isAdmin {
		plan, err := s.planRepo.GetByID(ctx, vm.PlanID)
		if err != nil {
			return fmt.Errorf("getting plan: %w", err)
		}
		if newVcpu > plan.VCPU {
			return fmt.Errorf("requested vCPU (%d) exceeds plan limit (%d)", newVcpu, plan.VCPU)
		}
		if newMemoryMB > plan.MemoryMB {
			return fmt.Errorf("requested memory (%d MB) exceeds plan limit (%d MB)", newMemoryMB, plan.MemoryMB)
		}
		if newDiskGB > plan.DiskGB {
			return fmt.Errorf("requested disk (%d GB) exceeds plan limit (%d GB)", newDiskGB, plan.DiskGB)
		}
	}

	// Validate disk is not shrinking (not supported)
	if newDiskGB < vm.DiskGB {
		return fmt.Errorf("disk shrinking is not supported (current: %d GB, requested: %d GB)", vm.DiskGB, newDiskGB)
	}

	// Check if node is assigned
	if vm.NodeID == nil {
		return fmt.Errorf("VM has no node assigned")
	}

	// For disk resize, we need to stop the VM first
	requiresStop := newDiskGB > vm.DiskGB

	// If resize requires stop and VM is running, we need to handle it
	if requiresStop && vm.Status == models.VMStatusRunning {
		// Stop the VM gracefully
		if err := s.nodeAgent.StopVM(ctx, *vm.NodeID, vm.ID, 120); err != nil {
			return fmt.Errorf("stopping VM for resize: %w", err)
		}
	}

	// Call node agent to resize VM
	if err := s.nodeAgent.ResizeVM(ctx, *vm.NodeID, vm.ID, newVcpu, newMemoryMB, newDiskGB); err != nil {
		return fmt.Errorf("resizing VM on node agent: %w", err)
	}

	// Update VM record
	if err := s.vmRepo.UpdateResources(ctx, vm.ID, newVcpu, newMemoryMB, newDiskGB); err != nil {
		return fmt.Errorf("updating VM resources: %w", err)
	}

	// Start the VM if we stopped it
	if requiresStop && vm.Status == models.VMStatusRunning {
		if err := s.nodeAgent.StartVM(ctx, *vm.NodeID, vm.ID); err != nil {
			s.logger.Warn("failed to start VM after resize", "vm_id", vm.ID, "error", err)
		}
	}

	s.logger.Info("VM resized",
		"vm_id", vm.ID,
		"vcpu", newVcpu,
		"memory_mb", newMemoryMB,
		"disk_gb", newDiskGB,
		"customer_id", customerID)

	return nil
}

// GetVM retrieves a VM by ID.
func (s *VMService) GetVM(ctx context.Context, vmID, customerID string, isAdmin bool) (*models.VM, error) {
	return s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
}

// ListVMs lists VMs with optional filtering and pagination.
// For non-admin users, only their own VMs are returned.
func (s *VMService) ListVMs(ctx context.Context, filter models.VMListFilter, customerID string, isAdmin bool) ([]models.VM, int, error) {
	// Non-admins can only see their own VMs
	if !isAdmin {
		filter.CustomerID = &customerID
	}

	return s.vmRepo.List(ctx, filter)
}

// GetVMMetrics retrieves real-time metrics for a VM.
func (s *VMService) GetVMMetrics(ctx context.Context, vmID, customerID string, isAdmin bool) (*models.VMMetrics, error) {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return nil, err
	}

	// Check if node is assigned
	if vm.NodeID == nil {
		return nil, fmt.Errorf("VM has no node assigned")
	}

	// Check if VM is running
	if vm.Status != models.VMStatusRunning {
		return nil, fmt.Errorf("VM is not running (status: %s)", vm.Status)
	}

	// Get metrics from node agent
	metrics, err := s.nodeAgent.GetVMMetrics(ctx, *vm.NodeID, vm.ID)
	if err != nil {
		return nil, fmt.Errorf("getting VM metrics: %w", err)
	}

	return metrics, nil
}

// GetVMStatus retrieves the current status of a VM.
func (s *VMService) GetVMStatus(ctx context.Context, vmID, customerID string, isAdmin bool) (string, error) {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return "", err
	}

	// If VM has no node, return database status
	if vm.NodeID == nil {
		return vm.Status, nil
	}

	// Get actual status from node agent
	status, err := s.nodeAgent.GetVMStatus(ctx, *vm.NodeID, vm.ID)
	if err != nil {
		s.logger.Warn("failed to get status from node agent, returning database status",
			"vm_id", vm.ID, "error", err)
		return vm.Status, nil
	}

	return status, nil
}

// GetVMDetail retrieves a VM with additional details (IPs, node info, etc.).
func (s *VMService) GetVMDetail(ctx context.Context, vmID, customerID string, isAdmin bool) (*models.VMDetail, error) {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return nil, err
	}

	detail := &models.VMDetail{
		VM: *vm,
	}

	// Get IP addresses
	if s.ipamService != nil {
		ips, err := s.ipamService.GetIPsByVM(ctx, vm.ID)
		if err != nil {
			s.logger.Warn("failed to get IPs for VM", "vm_id", vm.ID, "error", err)
		} else {
			detail.IPAddresses = ips
		}
	}

	// Get node hostname
	if vm.NodeID != nil {
		node, err := s.nodeRepo.GetByID(ctx, *vm.NodeID)
		if err != nil {
			s.logger.Warn("failed to get node for VM", "vm_id", vm.ID, "error", err)
		} else {
			detail.NodeHostname = &node.Hostname
		}
	}

	// Get plan name
	plan, err := s.planRepo.GetByID(ctx, vm.PlanID)
	if err != nil {
		s.logger.Warn("failed to get plan for VM", "vm_id", vm.ID, "error", err)
	} else {
		detail.PlanName = plan.Name
	}

	// Get template name
	if vm.TemplateID != nil {
		template, err := s.templateRepo.GetByID(ctx, *vm.TemplateID)
		if err != nil {
			s.logger.Warn("failed to get template for VM", "vm_id", vm.ID, "error", err)
		} else {
			detail.TemplateName = &template.Name
		}
	}

	return detail, nil
}

// UpdateVMHostname updates a VM's hostname.
func (s *VMService) UpdateVMHostname(ctx context.Context, vmID, newHostname, customerID string, isAdmin bool) error {
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return err
	}

	if err := s.vmRepo.UpdateHostname(ctx, vm.ID, newHostname); err != nil {
		return fmt.Errorf("updating VM hostname: %w", err)
	}

	s.logger.Info("VM hostname updated",
		"vm_id", vmID,
		"new_hostname", newHostname)

	return nil
}

// GetTaskStatus retrieves the status of an async task.
func (s *VMService) GetTaskStatus(ctx context.Context, taskID string) (*models.Task, error) {
	return s.taskRepo.GetByID(ctx, taskID)
}

// ListTasks lists tasks with optional filtering.
func (s *VMService) ListTasks(ctx context.Context, filter repository.TaskListFilter) ([]models.Task, int, error) {
	return s.taskRepo.List(ctx, filter)
}

// Helper functions

// getVMAndVerifyOwnership retrieves a VM and verifies the customer has access to it.
func (s *VMService) getVMAndVerifyOwnership(ctx context.Context, vmID, customerID string, isAdmin bool) (*models.VM, error) {
	vm, err := s.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("VM not found: %s", vmID)
		}
		return nil, fmt.Errorf("getting VM: %w", err)
	}

	// Verify ownership (admins can access any VM)
	if !isAdmin && vm.CustomerID != customerID {
		return nil, sharederrors.ErrForbidden
	}

	// Check if VM is deleted
	if vm.IsDeleted() {
		return nil, fmt.Errorf("VM has been deleted")
	}

	return vm, nil
}

// generateLibvirtDomainName generates a libvirt domain name from hostname and VM ID.
// Format: vm-{hostname}-{short-uuid}
func generateLibvirtDomainName(hostname, vmID string) string {
	// Use first 8 characters of UUID for uniqueness
	shortID := vmID
	if len(vmID) >= 8 {
		shortID = vmID[:8]
	}
	return fmt.Sprintf("vm-%s-%s", hostname, shortID)
}

// DefaultTaskPublisher is a basic implementation of TaskPublisher that creates
// tasks directly in the database. For production, use a NATS-based implementation.
type DefaultTaskPublisher struct {
	taskRepo *repository.TaskRepository
	logger   *slog.Logger
}

// NewDefaultTaskPublisher creates a new DefaultTaskPublisher.
func NewDefaultTaskPublisher(taskRepo *repository.TaskRepository, logger *slog.Logger) *DefaultTaskPublisher {
	return &DefaultTaskPublisher{
		taskRepo: taskRepo,
		logger:   logger.With("component", "task-publisher"),
	}
}

// PublishTask creates a task record in the database.
func (p *DefaultTaskPublisher) PublishTask(ctx context.Context, taskType string, payload map[string]any) (string, error) {
	taskID := uuid.New().String()

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshaling task payload: %w", err)
	}

	task := &models.Task{
		ID:        taskID,
		Type:      taskType,
		Status:    models.TaskStatusPending,
		Payload:   payloadJSON,
		Progress:  0,
		CreatedAt: time.Now().UTC(),
	}

	if err := p.taskRepo.Create(ctx, task); err != nil {
		return "", fmt.Errorf("creating task: %w", err)
	}

	p.logger.Info("task published", "task_id", taskID, "type", taskType)
	return taskID, nil
}
