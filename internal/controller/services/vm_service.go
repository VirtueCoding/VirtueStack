// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
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
	vmRepo               *repository.VMRepository
	nodeRepo             *repository.NodeRepository
	ipRepo               *repository.IPRepository
	planRepo             *repository.PlanRepository
	templateRepo         *repository.TemplateRepository
	taskRepo             *repository.TaskRepository
	taskPublisher        TaskPublisher
	nodeAgent            NodeAgentClient
	ipamService          IPAllocator
	storageBackendSvc    StorageBackendGetter
	preActionWebhookSvc  PreActionChecker
	billingHooks         BillingHookResolver
	customerRepo         *repository.CustomerRepository
	encryptionKey        string // For encrypting root passwords
	logger               *slog.Logger
}

// StorageBackendGetter defines the interface for getting storage backends for a node.
type StorageBackendGetter interface {
	GetBackendsForNodeByType(ctx context.Context, nodeID string, backendType string) ([]models.StorageBackend, error)
}

// PreActionChecker evaluates pre-action webhooks before protected operations.
type PreActionChecker interface {
	CheckPreAction(ctx context.Context, event string, customerID string, data map[string]any) error
}

// BillingHookResolver resolves the billing lifecycle hook for a customer's provider.
type BillingHookResolver interface {
	ForCustomer(providerName string) (billing.VMLifecycleHook, error)
}

// VMServiceConfig holds all dependencies for VMService construction.
// Using a config struct keeps NewVMService compliant with the ≤4-parameter
// constructor rule (QG-01) and makes future dependency additions non-breaking.
type VMServiceConfig struct {
	VMRepo              *repository.VMRepository
	NodeRepo            *repository.NodeRepository
	IPRepo              *repository.IPRepository
	PlanRepo            *repository.PlanRepository
	TemplateRepo        *repository.TemplateRepository
	TaskRepo            *repository.TaskRepository
	TaskPublisher       TaskPublisher
	NodeAgent           NodeAgentClient
	IPAMService         IPAllocator
	StorageBackendSvc   StorageBackendGetter
	PreActionWebhookSvc PreActionChecker
	BillingHooks        BillingHookResolver
	CustomerRepo        *repository.CustomerRepository
	EncryptionKey       string
	Logger              *slog.Logger
}

// NewVMService creates a new VMService with the given configuration.
func NewVMService(cfg VMServiceConfig) *VMService {
	return &VMService{
		vmRepo:              cfg.VMRepo,
		nodeRepo:            cfg.NodeRepo,
		ipRepo:              cfg.IPRepo,
		planRepo:            cfg.PlanRepo,
		templateRepo:        cfg.TemplateRepo,
		taskRepo:            cfg.TaskRepo,
		taskPublisher:       cfg.TaskPublisher,
		nodeAgent:           cfg.NodeAgent,
		ipamService:         cfg.IPAMService,
		storageBackendSvc:   cfg.StorageBackendSvc,
		preActionWebhookSvc: cfg.PreActionWebhookSvc,
		billingHooks:        cfg.BillingHooks,
		customerRepo:        cfg.CustomerRepo,
		encryptionKey:       cfg.EncryptionKey,
		logger:              cfg.Logger.With("component", "vm-service"),
	}
}

// vmCreateDeps bundles the resolved plan, template, node, and storage backend for VM creation.
type vmCreateDeps struct {
	plan           *models.Plan
	template       *models.Template
	node           *models.Node
	storageBackend *models.StorageBackend
}

// notifyBillingHook resolves the billing provider for a customer and calls fn.
// Errors are logged but never returned to avoid blocking VM operations.
func (s *VMService) notifyBillingHook(ctx context.Context, customerID string, fn func(billing.VMLifecycleHook) error) {
	if s.billingHooks == nil || s.customerRepo == nil {
		return
	}
	cust, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		s.logger.Warn("billing hook: failed to get customer",
			"customer_id", customerID, "error", err)
		return
	}
	provider := ""
	if cust.BillingProvider != nil {
		provider = *cust.BillingProvider
	}
	hook, err := s.billingHooks.ForCustomer(provider)
	if err != nil {
		s.logger.Warn("billing hook: provider not found",
			"customer_id", customerID,
			"provider", provider, "error", err)
		return
	}
	if err := fn(hook); err != nil {
		s.logger.Warn("billing hook: callback failed",
			"customer_id", customerID,
			"provider", provider, "error", err)
	}
}

// validateCreateVMRequest checks that the requested plan is active, the template
// exists, and the plan's disk meets the template's minimum requirement.
func (s *VMService) validateCreateVMRequest(ctx context.Context, req *models.VMCreateRequest) (*models.Plan, *models.Template, error) {
	plan, err := s.planRepo.GetByID(ctx, req.PlanID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, nil, fmt.Errorf("plan not found: %s", req.PlanID)
		}
		return nil, nil, fmt.Errorf("getting plan: %w", err)
	}
	if !plan.IsActive {
		return nil, nil, fmt.Errorf("plan %s is not active", req.PlanID)
	}

	template, err := s.templateRepo.GetByID(ctx, req.TemplateID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, nil, fmt.Errorf("template not found: %s", req.TemplateID)
		}
		return nil, nil, fmt.Errorf("getting template: %w", err)
	}

	if plan.DiskGB < template.MinDiskGB {
		return nil, nil, fmt.Errorf("plan disk size (%d GB) is less than template minimum (%d GB)",
			plan.DiskGB, template.MinDiskGB)
	}
	return plan, template, nil
}

// selectNodeForVM picks the least-loaded online node that has sufficient vCPU
// and memory capacity for the requested VM resources (F-094).
// If locationID is empty it scans all online nodes and picks the one with the
// most available memory that meets the capacity requirements.
// If storageBackendType is provided, it also filters nodes to those that have
// the specified storage backend assigned.
func (s *VMService) selectNodeForVM(ctx context.Context, locationID string, vcpu, memoryMB int, storageBackendType string) (*models.Node, error) {
	nodes, err := s.nodeRepo.ListByStatus(ctx, models.NodeStatusOnline)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no available nodes")
	}

	var best *models.Node
	for i := range nodes {
		node := &nodes[i]

		// Filter by location if specified.
		if locationID != "" && (node.LocationID == nil || *node.LocationID != locationID) {
			continue
		}

		// Skip nodes with insufficient vCPU or memory capacity (F-094).
		if node.AllocatedVCPU+vcpu > node.TotalVCPU {
			continue
		}
		if node.AllocatedMemoryMB+memoryMB > node.TotalMemoryMB {
			continue
		}

		// Filter by storage backend compatibility if specified
		if storageBackendType != "" && s.storageBackendSvc != nil {
			backends, err := s.storageBackendSvc.GetBackendsForNodeByType(ctx, node.ID, storageBackendType)
			if err != nil {
				s.logger.Debug("failed to check storage backends for node during selection",
					"node_id", node.ID,
					"storage_backend", storageBackendType,
					"error", err)
				continue
			}
			if len(backends) == 0 {
				// Node doesn't have the required storage backend type assigned
				continue
			}
		}

		if best == nil || (node.TotalMemoryMB-node.AllocatedMemoryMB > best.TotalMemoryMB-best.AllocatedMemoryMB) {
			best = node
		}
	}

	if best == nil {
		if storageBackendType != "" {
			if locationID != "" {
				return nil, fmt.Errorf("no available nodes with sufficient capacity and storage backend %s in location %s", storageBackendType, locationID)
			}
			return nil, fmt.Errorf("no available nodes with sufficient capacity and storage backend %s", storageBackendType)
		}
		if locationID != "" {
			return nil, fmt.Errorf("no available nodes with sufficient capacity in location %s", locationID)
		}
		return nil, fmt.Errorf("no available nodes with sufficient capacity")
	}
	return best, nil
}

// buildVMRecord constructs the VM struct from the creation request, resolved deps,
// and pre-generated MAC address and encrypted password. It does not touch the database.
func buildVMRecord(req *models.VMCreateRequest, deps vmCreateDeps, customerID, macAddress, encryptedPassword string) *models.VM {
	vmID := uuid.New().String()
	libvirtDomainName := generateLibvirtDomainName(req.Hostname, vmID)
	vm := &models.VM{
		ID: vmID, CustomerID: customerID, NodeID: &deps.node.ID,
		PlanID: deps.plan.ID, Hostname: req.Hostname, Status: models.VMStatusProvisioning,
		VCPU: deps.plan.VCPU, MemoryMB: deps.plan.MemoryMB, DiskGB: deps.plan.DiskGB,
		PortSpeedMbps: deps.plan.PortSpeedMbps, BandwidthLimitGB: deps.plan.BandwidthLimitGB,
		BandwidthUsedBytes: 0, BandwidthResetAt: time.Now().UTC(),
		MACAddress: macAddress, TemplateID: &deps.template.ID,
		LibvirtDomainName: &libvirtDomainName, RootPasswordEncrypted: &encryptedPassword,
		WHMCSServiceID: req.WHMCSServiceID, StorageBackend: deps.plan.StorageBackend,
	}
	// Set StorageBackendID if we have a resolved storage backend
	if deps.storageBackend != nil {
		vm.StorageBackendID = &deps.storageBackend.ID
	}
	return vm
}

// persistVMRecord creates the VM row in the database.
// It returns the created VM. IPv4 allocation is handled exclusively by the async
// task worker (handleVMCreate) to avoid double allocation (F-024).
func (s *VMService) persistVMRecord(ctx context.Context, req *models.VMCreateRequest, deps vmCreateDeps, customerID string) (*models.VM, *models.IPAddress, error) {
	macAddress, err := crypto.GenerateMACAddress()
	if err != nil {
		return nil, nil, fmt.Errorf("generating MAC address: %w", err)
	}
	encryptedPassword, err := crypto.Encrypt(req.Password, s.encryptionKey)
	if err != nil {
		return nil, nil, fmt.Errorf("encrypting password: %w", err)
	}

	vm := buildVMRecord(req, deps, customerID, macAddress, encryptedPassword)
	if err := s.vmRepo.Create(ctx, vm); err != nil {
		return nil, nil, fmt.Errorf("creating VM record: %w", err)
	}

	// IPv4 is allocated by the async task handler, not here, to prevent
	// double allocation (F-024).
	return vm, nil, nil
}

// publishVMCreateTask builds the task payload and publishes the vm.create task.
// On failure it attempts a best-effort cleanup of the VM record and any allocated IP.
func (s *VMService) publishVMCreateTask(ctx context.Context, req *models.VMCreateRequest, vm *models.VM, deps vmCreateDeps, ipv4Address *models.IPAddress) (string, error) {
	taskPayload := map[string]any{
		"vm_id": vm.ID, "node_id": deps.node.ID, "hostname": vm.Hostname,
		"vcpu": vm.VCPU, "memory_mb": vm.MemoryMB, "disk_gb": vm.DiskGB,
		"template_id": deps.template.ID, "template_rbd_image": deps.template.RBDImage,
		"template_rbd_snapshot": deps.template.RBDSnapshot,
		"mac_address":           vm.MACAddress, "port_speed_mbps": vm.PortSpeedMbps,
		"bandwidth_limit_gb": vm.BandwidthLimitGB, "ssh_keys": req.SSHKeys,
		"storage_backend": deps.plan.StorageBackend,
	}
	if ipv4Address != nil {
		taskPayload["ipv4_address"] = ipv4Address.Address
	}

	// Pass storage backend config from the resolved StorageBackend record
	if deps.storageBackend != nil {
		switch deps.storageBackend.Type {
		case models.StorageTypeCeph:
			if deps.storageBackend.CephPool != nil {
				taskPayload["ceph_pool"] = *deps.storageBackend.CephPool
			}
			if deps.storageBackend.CephUser != nil {
				taskPayload["ceph_user"] = *deps.storageBackend.CephUser
			}
			if deps.storageBackend.CephMonitors != nil {
				taskPayload["ceph_monitors"] = *deps.storageBackend.CephMonitors
			}
		case models.StorageTypeQCOW:
			if deps.storageBackend.StoragePath != nil {
				taskPayload["storage_path"] = *deps.storageBackend.StoragePath
			}
		case models.StorageTypeLVM:
			if deps.storageBackend.LVMVolumeGroup != nil {
				taskPayload["lvm_volume_group"] = *deps.storageBackend.LVMVolumeGroup
			}
			if deps.storageBackend.LVMThinPool != nil {
				taskPayload["lvm_thin_pool"] = *deps.storageBackend.LVMThinPool
			}
		}
		taskPayload["storage_backend_id"] = deps.storageBackend.ID
	} else {
		// Fallback to node-level config for backward compatibility
		taskPayload["ceph_pool"] = deps.node.CephPool
		taskPayload["storage_path"] = deps.node.StoragePath
	}

	taskID, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeVMCreate, taskPayload)
	if err != nil {
		if delErr := s.vmRepo.SoftDelete(ctx, vm.ID); delErr != nil {
			s.logger.Error("failed to soft delete VM after task publish failure", "operation", "SoftDelete", "err", delErr)
		}
		if ipv4Address != nil && s.ipamService != nil {
			if relErr := s.ipamService.ReleaseIPsByVM(ctx, vm.ID); relErr != nil {
				s.logger.Error("failed to release IPs after task publish failure", "operation", "ReleaseIPsByVM", "err", relErr)
			}
		}
		return "", fmt.Errorf("publishing create task: %w", err)
	}
	return taskID, nil
}

// CreateVM creates a new virtual machine.
// This is an async operation that publishes a vm.create task.
// Returns the created VM record and task ID for polling.
//
// Idempotency: If an IdempotencyKey is provided and a task with that key exists,
// returns the existing VM and task instead of creating duplicates.
// Also checks for existing VM by WHMCSServiceID to handle WHMCS retries.
func (s *VMService) CreateVM(ctx context.Context, req *models.VMCreateRequest, customerID string) (*models.VM, string, error) {
	// Idempotency check: If WHMCSServiceID provided, check for existing VM
	// This handles WHMCS retry scenarios where the provisioning call times out
	// but the VM was already created.
	if req.WHMCSServiceID != nil && *req.WHMCSServiceID > 0 {
		existingVM, err := s.vmRepo.GetByWHMCSServiceID(ctx, *req.WHMCSServiceID)
		if err == nil && existingVM != nil {
			// VM already exists for this WHMCS service - find the associated task
			s.logger.Info("VM already exists for WHMCS service, returning existing",
				"vm_id", existingVM.ID, "whmcs_service_id", *req.WHMCSServiceID, "customer_id", customerID)
			// Return the existing VM. Task lookup would require storing vm_id in task payload.
			// For now, return empty task ID since the VM exists.
			return existingVM, "", nil
		}
	}

	// Idempotency check: If IdempotencyKey provided, check for existing task
	if req.IdempotencyKey != "" {
		existingTask, err := s.taskRepo.GetByIDempotencyKey(ctx, req.IdempotencyKey)
		if err == nil && existingTask != nil {
			// Task exists - extract VM ID from payload if available
			s.logger.Info("Task already exists for idempotency key, returning existing",
				"task_id", existingTask.ID, "idempotency_key", req.IdempotencyKey)
			// Parse the VM ID from the task payload (json.RawMessage)
			var payload struct {
				VMID string `json:"vm_id"`
			}
			if err := json.Unmarshal(existingTask.Payload, &payload); err == nil && payload.VMID != "" {
				vm, err := s.vmRepo.GetByID(ctx, payload.VMID)
				if err == nil {
					return vm, existingTask.ID, nil
				}
			}
			// Task exists but VM not found - return task ID only
			return nil, existingTask.ID, nil
		}
	}

	// PRE-ACTION WEBHOOK CHECK
	if s.preActionWebhookSvc != nil {
		whData := map[string]any{
			"hostname": req.Hostname,
			"plan_id":  req.PlanID,
		}
		if err := s.preActionWebhookSvc.CheckPreAction(ctx, models.PreActionEventVMCreate, customerID, whData); err != nil {
			return nil, "", err
		}
	}

	plan, template, err := s.validateCreateVMRequest(ctx, req)
	if err != nil {
		return nil, "", err
	}

	locationID := ""
	if req.LocationID != nil {
		locationID = *req.LocationID
	}

	node, err := s.selectNodeForVM(ctx, locationID, plan.VCPU, plan.MemoryMB, plan.StorageBackend)
	if err != nil {
		return nil, "", err
	}

	deps := vmCreateDeps{plan: plan, template: template, node: node}

	// Resolve storage backend for the node
	if s.storageBackendSvc != nil && plan.StorageBackend != "" {
		backends, err := s.storageBackendSvc.GetBackendsForNodeByType(ctx, node.ID, plan.StorageBackend)
		if err != nil {
			s.logger.Warn("failed to query storage backends for node, using node defaults",
				"node_id", node.ID,
				"storage_backend", plan.StorageBackend,
				"error", err)
		} else if len(backends) > 0 {
			// Select the first available backend (could add preferred logic later)
			deps.storageBackend = &backends[0]
			// Validate storage backend health status before creating VM
			if deps.storageBackend.HealthStatus == "critical" {
				return nil, "", fmt.Errorf("storage backend %s is unhealthy (critical), cannot create VM", deps.storageBackend.Name)
			}
		} else {
			s.logger.Warn("no storage backend of required type found for node",
				"node_id", node.ID,
				"storage_backend", plan.StorageBackend)
		}
	}
	vm, ipv4Address, err := s.persistVMRecord(ctx, req, deps, customerID)
	if err != nil {
		return nil, "", err
	}

	taskID, err := s.publishVMCreateTask(ctx, req, vm, deps, ipv4Address)
	if err != nil {
		return nil, "", err
	}

	s.logger.Info("VM creation initiated",
		"vm_id", vm.ID, "task_id", taskID,
		"customer_id", customerID, "node_id", node.ID, "hostname", vm.Hostname)

	s.notifyBillingHook(ctx, customerID, func(hook billing.VMLifecycleHook) error {
		return hook.OnVMCreated(ctx, billing.VMRef{
			ID: vm.ID, CustomerID: customerID,
			PlanID: vm.PlanID, Hostname: vm.Hostname,
		})
	})

	return vm, taskID, nil
}

// StartVM starts a stopped or suspended VM.
// This is a synchronous operation that calls the node agent directly.

// StopVM stops a running VM.
// If force is true, performs a hard power-off; otherwise graceful ACPI shutdown.

// RestartVM restarts a VM by stopping and starting it.

// DeleteVM deletes a VM.
// This is an async operation that publishes a vm.delete task.
// The VM is soft-deleted immediately, and cleanup happens asynchronously.
func (s *VMService) DeleteVM(ctx context.Context, vmID, customerID string, isAdmin bool) (string, error) {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return "", fmt.Errorf("verifying ownership for delete: %w", err)
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

	s.notifyBillingHook(ctx, customerID, func(hook billing.VMLifecycleHook) error {
		return hook.OnVMDeleted(ctx, billing.VMRef{
			ID: vm.ID, CustomerID: customerID,
			PlanID: vm.PlanID, Hostname: vm.Hostname,
		})
	})

	return taskID, nil
}

// ReinstallVM reinstalls a VM with a new template.
// This is an async operation that publishes a vm.reinstall task.
// Returns the updated VM record and task ID for polling.
func (s *VMService) ReinstallVM(ctx context.Context, vmID, templateID, customerID string, password string, isAdmin bool) (*models.VM, string, error) {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return nil, "", fmt.Errorf("verifying ownership for reinstall: %w", err)
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

// GetPlan retrieves a plan by ID.
// This is a convenience method for handlers that need to validate plan existence.
func (s *VMService) GetPlan(ctx context.Context, planID string) (*models.Plan, error) {
	plan, err := s.planRepo.GetByID(ctx, planID)
	if err != nil {
		return nil, fmt.Errorf("getting plan: %w", err)
	}
	return plan, nil
}

// ResizeVM resizes a VM's resources.
// CPU/memory-only changes are performed synchronously (libvirt hot-plug).
// Disk resize changes are performed asynchronously via vm.resize task.
// Returns a task ID when disk resize is required, empty string for synchronous ops.
func (s *VMService) ResizeVM(ctx context.Context, vmID, customerID string, newVcpu, newMemoryMB, newDiskGB int, isAdmin bool) (string, error) {
	return s.ResizeVMWithPlan(ctx, vmID, customerID, newVcpu, newMemoryMB, newDiskGB, "", isAdmin)
}

// ResizeVMWithPlan resizes a VM's resources and optionally updates the plan.
// When newPlanID is provided, the VM's plan is updated to the new plan.
// CPU/memory-only changes are performed synchronously (libvirt hot-plug).
// Disk resize changes are performed asynchronously via vm.resize task.
// Returns a task ID when disk resize is required, empty string for synchronous ops.
func (s *VMService) ResizeVMWithPlan(ctx context.Context, vmID, customerID string, newVcpu, newMemoryMB, newDiskGB int, newPlanID string, isAdmin bool) (string, error) {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return "", fmt.Errorf("verifying ownership for resize: %w", err)
	}

	// Validate resources are being changed
	if newVcpu == 0 && newMemoryMB == 0 && newDiskGB == 0 {
		return "", fmt.Errorf("no resize parameters provided")
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
			return "", fmt.Errorf("getting plan: %w", err)
		}
		if newVcpu > plan.VCPU {
			return "", fmt.Errorf("requested vCPU (%d) exceeds plan limit (%d)", newVcpu, plan.VCPU)
		}
		if newMemoryMB > plan.MemoryMB {
			return "", fmt.Errorf("requested memory (%d MB) exceeds plan limit (%d MB)", newMemoryMB, plan.MemoryMB)
		}
		if newDiskGB > plan.DiskGB {
			return "", fmt.Errorf("requested disk (%d GB) exceeds plan limit (%d GB)", newDiskGB, plan.DiskGB)
		}
	}

	// Validate disk is not shrinking (not supported)
	if newDiskGB < vm.DiskGB {
		return "", fmt.Errorf("disk shrinking is not supported (current: %d GB, requested: %d GB)", vm.DiskGB, newDiskGB)
	}

	// Check if node is assigned
	if vm.NodeID == nil {
		return "", fmt.Errorf("VM has no node assigned")
	}

	// Update plan if a new plan is provided
	if newPlanID != "" && newPlanID != vm.PlanID {
		if err := s.vmRepo.UpdatePlanID(ctx, vm.ID, newPlanID); err != nil {
			return "", fmt.Errorf("updating VM plan: %w", err)
		}
		s.logger.Info("VM plan updated", "vm_id", vm.ID, "old_plan_id", vm.PlanID, "new_plan_id", newPlanID)
	}

	requiresDiskResize := newDiskGB > vm.DiskGB

	if requiresDiskResize {
		taskPayload := map[string]any{
			"vm_id":         vm.ID,
			"node_id":       *vm.NodeID,
			"new_vcpu":      newVcpu,
			"new_memory_mb": newMemoryMB,
			"new_disk_gb":   newDiskGB,
		}

		taskID, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeVMResize, taskPayload)
		if err != nil {
			return "", fmt.Errorf("publishing resize task: %w", err)
		}

		s.logger.Info("VM resize initiated (async, disk change required)",
			"vm_id", vm.ID,
			"task_id", taskID,
			"vcpu", newVcpu,
			"memory_mb", newMemoryMB,
			"disk_gb", newDiskGB)

		s.notifyBillingHook(ctx, customerID, func(hook billing.VMLifecycleHook) error {
			return hook.OnVMResized(ctx, billing.VMRef{
				ID: vm.ID, CustomerID: customerID,
				PlanID: vm.PlanID, Hostname: vm.Hostname,
			}, vm.PlanID, newPlanID)
		})

		return taskID, nil
	}

	// Synchronous CPU/memory resize (libvirt hot-plug)
	if err := s.nodeAgent.ResizeVM(ctx, *vm.NodeID, vm.ID, newVcpu, newMemoryMB, newDiskGB); err != nil {
		return "", fmt.Errorf("resizing VM on node agent: %w", err)
	}

	// Update VM record
	if err := s.vmRepo.UpdateResources(ctx, vm.ID, newVcpu, newMemoryMB, newDiskGB); err != nil {
		return "", fmt.Errorf("updating VM resources: %w", err)
	}

	s.logger.Info("VM resized (sync, CPU/memory only)",
		"vm_id", vm.ID,
		"vcpu", newVcpu,
		"memory_mb", newMemoryMB,
		"customer_id", customerID)

	s.notifyBillingHook(ctx, customerID, func(hook billing.VMLifecycleHook) error {
		return hook.OnVMResized(ctx, billing.VMRef{
			ID: vm.ID, CustomerID: customerID,
			PlanID: vm.PlanID, Hostname: vm.Hostname,
		}, vm.PlanID, newPlanID)
	})

	return "", nil
}

// GetVM retrieves a VM by ID.
func (s *VMService) GetVM(ctx context.Context, vmID, customerID string, isAdmin bool) (*models.VM, error) {
	return s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
}

// ListVMs lists VMs with optional filtering and pagination.
// For non-admin users, only their own VMs are returned.
func (s *VMService) ListVMs(ctx context.Context, filter models.VMListFilter, customerID string, isAdmin bool) ([]models.VM, bool, string, error) {
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
		return nil, fmt.Errorf("verifying ownership for metrics: %w", err)
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
		return "", fmt.Errorf("verifying ownership for status: %w", err)
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
		return nil, fmt.Errorf("verifying ownership for detail: %w", err)
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
		return fmt.Errorf("verifying ownership for hostname update: %w", err)
	}

	if err := s.vmRepo.UpdateHostname(ctx, vm.ID, newHostname); err != nil {
		return fmt.Errorf("updating VM hostname: %w", err)
	}

	s.logger.Info("VM hostname updated",
		"vm_id", vmID,
		"new_hostname", newHostname)

	return nil
}

// UpdateVMNetworkLimits updates the port speed and bandwidth limit for a VM.
// Only admins may call this; isAdmin must be true.
func (s *VMService) UpdateVMNetworkLimits(ctx context.Context, vmID string, portSpeedMbps, bandwidthLimitGB int) error {
	if err := s.vmRepo.UpdateNetworkLimits(ctx, vmID, portSpeedMbps, bandwidthLimitGB); err != nil {
		return fmt.Errorf("updating VM network limits: %w", err)
	}

	s.logger.Info("VM network limits updated",
		"vm_id", vmID,
		"port_speed_mbps", portSpeedMbps,
		"bandwidth_limit_gb", bandwidthLimitGB)

	return nil
}

// GetTaskStatus retrieves the status of an async task.
func (s *VMService) GetTaskStatus(ctx context.Context, taskID string) (*models.Task, error) {
	return s.taskRepo.GetByID(ctx, taskID)
}

// ListTasks lists tasks with optional filtering.
func (s *VMService) ListTasks(ctx context.Context, filter repository.TaskListFilter) ([]models.Task, bool, string, error) {
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
