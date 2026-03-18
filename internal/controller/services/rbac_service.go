// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// Role constants define the user roles in the system.
const (
	RoleSuperAdmin = "super_admin"
	RoleAdmin      = "admin"
	RoleCustomer   = "customer"
)

// Action constants define the types of operations that can be performed.
const (
	ActionCreate    = "create"
	ActionRead      = "read"
	ActionUpdate    = "update"
	ActionDelete    = "delete"
	ActionStart     = "start"
	ActionStop      = "stop"
	ActionForceStop = "force_stop"
	ActionReinstall = "reinstall"
	ActionMigrate   = "migrate"
	ActionDrain     = "drain"
	ActionFailover  = "failover"
	ActionSuspend   = "suspend"
)

// Resource constants define the types of resources in the system.
const (
	ResourceVM        = "vm"
	ResourceNode      = "node"
	ResourcePlan      = "plan"
	ResourceTemplate  = "template"
	ResourceCustomer  = "customer"
	ResourceAuditLog  = "audit_log"
	ResourceIPAddress = "ip_address"
)

// DestructiveActions maps operations that require re-authentication.
// These actions have significant or irreversible impact on resources.
// A map is used for O(1) lookup instead of linear scan.
var DestructiveActions = map[string]bool{
	ActionDelete:    true,
	ActionForceStop: true,
	ActionReinstall: true,
	ActionMigrate:   true,
	ActionFailover:  true,
}

// ReAuthWindow is the time window within which a re-authentication is considered valid.
const ReAuthWindow = 5 * time.Minute

// SessionReauthStore abstracts session re-authentication timestamp operations.
type SessionReauthStore interface {
	GetSessionLastReauthAt(ctx context.Context, sessionID string) (*time.Time, error)
	UpdateSessionLastReauthAt(ctx context.Context, sessionID string, timestamp time.Time) error
}

// RBACService provides Role-Based Access Control for VirtueStack.
// It enforces permissions based on user roles and logs denied access attempts.
type RBACService struct {
	auditRepo    *repository.AuditRepository
	customerRepo SessionReauthStore
	logger       *slog.Logger
}

// NewRBACService creates a new RBACService with the given dependencies.
func NewRBACService(auditRepo *repository.AuditRepository, customerRepo SessionReauthStore, logger *slog.Logger) *RBACService {
	return &RBACService{
		auditRepo:    auditRepo,
		customerRepo: customerRepo,
		logger:       logger,
	}
}

// CheckPermission checks if a user has permission to perform an action on a resource.
// It returns true if permitted, false otherwise. Denied attempts are logged to audit_logs.
func (s *RBACService) CheckPermission(ctx context.Context, userID, userRole, action, resourceType, resourceID string) (bool, error) {
	// Super admin can do everything
	if userRole == RoleSuperAdmin {
		return true, nil
	}

	// Admin can do most things except certain super-admin-only operations
	if userRole == RoleAdmin {
		// Admin cannot delete nodes or create/delete plans
		switch {
		case resourceType == ResourceNode && action == ActionDelete:
			return s.logDeniedAndReturn(ctx, userID, userRole, action, resourceType, resourceID, "only super_admin can delete nodes"), nil
		case resourceType == ResourcePlan && (action == ActionCreate || action == ActionDelete):
			return s.logDeniedAndReturn(ctx, userID, userRole, action, resourceType, resourceID, "only super_admin can create/delete plans"), nil
		default:
			return true, nil
		}
	}

	// Customer permissions are more restricted
	if userRole == RoleCustomer {
		// Customers can only interact with their own VMs and limited customer actions
		switch resourceType {
		case ResourceVM:
			// Customers need resource ownership check - handled by specific methods
			return false, nil
		case ResourceCustomer:
			// Customers can read/update themselves only - handled by specific methods
			return false, nil
		case ResourceNode, ResourcePlan, ResourceTemplate, ResourceAuditLog:
			return s.logDeniedAndReturn(ctx, userID, userRole, action, resourceType, resourceID, "customers cannot access this resource type"), nil
		}
	}

	return s.logDeniedAndReturn(ctx, userID, userRole, action, resourceType, resourceID, "unknown role"), nil
}

// logDeniedAndReturn logs a denied access attempt and returns false.
func (s *RBACService) logDeniedAndReturn(ctx context.Context, userID, userRole, action, resourceType, resourceID, reason string) bool {
	s.logger.Warn("permission denied",
		"user_id", userID,
		"role", userRole,
		"action", action,
		"resource_type", resourceType,
		"resource_id", resourceID,
		"reason", reason,
	)

	// Log to audit
	errMsg := fmt.Sprintf("permission denied: %s", reason)
	auditLog := &models.AuditLog{
		ActorID:      &userID,
		ActorType:    userRole,
		Action:       fmt.Sprintf("rbac.deny.%s.%s", resourceType, action),
		ResourceType: resourceType,
		ResourceID:   &resourceID,
		Success:      false,
		ErrorMessage: &errMsg,
	}

	if s.auditRepo != nil {
		if err := s.auditRepo.Append(ctx, auditLog); err != nil {
			s.logger.Error("failed to log denied permission to audit", "error", err)
		}
	}

	return false
}

// RequireReauthForDestructive checks if an action requires re-authentication.
// Destructive operations require the user to have re-authenticated within the last 5 minutes.
// Returns true if re-authentication is required, false if the action can proceed.
// For destructive actions, checks if last_reauth_at is within the 5-minute window.
func (s *RBACService) RequireReauthForDestructive(ctx context.Context, sessionID, action string) (bool, error) {
	// Non-destructive actions don't require re-auth
	if !DestructiveActions[action] {
		return false, nil
	}

	// For destructive actions, check the last re-auth timestamp
	if s.customerRepo == nil {
		return true, fmt.Errorf("customer repository not configured")
	}

	lastReauthAt, err := s.customerRepo.GetSessionLastReauthAt(ctx, sessionID)
	if err != nil {
		// If session not found or other error, require re-auth for safety
		return true, fmt.Errorf("checking session last_reauth_at: %w", err)
	}

	// If no last_reauth_at recorded, re-auth is required
	if lastReauthAt == nil {
		return true, nil
	}

	// Check if within 5-minute window
	if time.Since(*lastReauthAt) <= ReAuthWindow {
		// Within window - no re-auth needed
		return false, nil
	}

	// Outside 5-minute window - re-auth required
	return true, nil
}

// CanCreateVM checks if a user can create a VM.
// Customers can create VMs up to plan limit, admins have no limit.
func (s *RBACService) CanCreateVM(ctx context.Context, userID, userRole string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		// Customer can create VMs - plan limit check would be done by caller
		return true, nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionCreate, ResourceVM, "", "unknown role"), nil
	}
}

// CanDeleteVM checks if a user can delete a VM.
// Customer can delete own VMs only, admin can delete any VM.
// Requires re-auth for destructive action.
func (s *RBACService) CanDeleteVM(ctx context.Context, userID, userRole, vmID, vmCustomerID string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		if userID == vmCustomerID {
			return true, nil
		}
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionDelete, ResourceVM, vmID, "customer can only delete own VMs"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionDelete, ResourceVM, vmID, "unknown role"), nil
	}
}

// CanStartVM checks if a user can start a VM.
// Customer can start own VMs only, admin can start any VM.
func (s *RBACService) CanStartVM(ctx context.Context, userID, userRole, vmID, vmCustomerID string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		if userID == vmCustomerID {
			return true, nil
		}
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionStart, ResourceVM, vmID, "customer can only start own VMs"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionStart, ResourceVM, vmID, "unknown role"), nil
	}
}

// CanStopVM checks if a user can stop a VM.
// Customer can stop own VMs only, admin can stop any VM.
func (s *RBACService) CanStopVM(ctx context.Context, userID, userRole, vmID, vmCustomerID string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		if userID == vmCustomerID {
			return true, nil
		}
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionStop, ResourceVM, vmID, "customer can only stop own VMs"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionStop, ResourceVM, vmID, "unknown role"), nil
	}
}

// CanForceStopVM checks if a user can force stop a VM.
// Force stop requires admin role - customers cannot perform this action.
func (s *RBACService) CanForceStopVM(ctx context.Context, userID, userRole, vmID, vmCustomerID string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionForceStop, ResourceVM, vmID, "customers cannot force stop VMs"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionForceStop, ResourceVM, vmID, "unknown role"), nil
	}
}

// CanReinstallVM checks if a user can reinstall a VM.
// Reinstall requires admin role - customers cannot perform this action.
func (s *RBACService) CanReinstallVM(ctx context.Context, userID, userRole, vmID, vmCustomerID string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionReinstall, ResourceVM, vmID, "customers cannot reinstall VMs"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionReinstall, ResourceVM, vmID, "unknown role"), nil
	}
}

// CanMigrateVM checks if a user can migrate a VM.
// Only admin and super_admin can migrate VMs.
func (s *RBACService) CanMigrateVM(ctx context.Context, userID, userRole string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionMigrate, ResourceVM, "", "customers cannot migrate VMs"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionMigrate, ResourceVM, "", "unknown role"), nil
	}
}

// CanCreateNode checks if a user can create a node.
// Only admin and super_admin can create nodes.
func (s *RBACService) CanCreateNode(ctx context.Context, userID, userRole string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionCreate, ResourceNode, "", "customers cannot create nodes"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionCreate, ResourceNode, "", "unknown role"), nil
	}
}

// CanDeleteNode checks if a user can delete a node.
// Only super_admin can delete nodes.
func (s *RBACService) CanDeleteNode(ctx context.Context, userID, userRole string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin:
		return true, nil
	case RoleAdmin:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionDelete, ResourceNode, "", "only super_admin can delete nodes"), nil
	case RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionDelete, ResourceNode, "", "customers cannot delete nodes"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionDelete, ResourceNode, "", "unknown role"), nil
	}
}

// CanDrainNode checks if a user can drain a node.
// Only admin and super_admin can drain nodes.
func (s *RBACService) CanDrainNode(ctx context.Context, userID, userRole string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionDrain, ResourceNode, "", "customers cannot drain nodes"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionDrain, ResourceNode, "", "unknown role"), nil
	}
}

// CanFailoverNode checks if a user can perform failover on a node.
// Only admin and super_admin can perform failover.
func (s *RBACService) CanFailoverNode(ctx context.Context, userID, userRole string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionFailover, ResourceNode, "", "customers cannot failover nodes"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionFailover, ResourceNode, "", "unknown role"), nil
	}
}

// CanCreatePlan checks if a user can create a plan.
// Only super_admin can create plans.
func (s *RBACService) CanCreatePlan(ctx context.Context, userID, userRole string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin:
		return true, nil
	case RoleAdmin, RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionCreate, ResourcePlan, "", "only super_admin can create plans"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionCreate, ResourcePlan, "", "unknown role"), nil
	}
}

// CanDeletePlan checks if a user can delete a plan.
// Only super_admin can delete plans.
func (s *RBACService) CanDeletePlan(ctx context.Context, userID, userRole string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin:
		return true, nil
	case RoleAdmin, RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionDelete, ResourcePlan, "", "only super_admin can delete plans"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionDelete, ResourcePlan, "", "unknown role"), nil
	}
}

// CanCreateTemplate checks if a user can create a template.
// Only admin and super_admin can create templates.
func (s *RBACService) CanCreateTemplate(ctx context.Context, userID, userRole string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionCreate, ResourceTemplate, "", "customers cannot create templates"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionCreate, ResourceTemplate, "", "unknown role"), nil
	}
}

// CanDeleteTemplate checks if a user can delete a template.
// Only admin and super_admin can delete templates.
func (s *RBACService) CanDeleteTemplate(ctx context.Context, userID, userRole string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionDelete, ResourceTemplate, "", "customers cannot delete templates"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionDelete, ResourceTemplate, "", "unknown role"), nil
	}
}

// CanViewCustomer checks if a user can view a customer's information.
// Customer can view self only, admin can view any.
func (s *RBACService) CanViewCustomer(ctx context.Context, userID, userRole, targetCustomerID string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		if userID == targetCustomerID {
			return true, nil
		}
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionRead, ResourceCustomer, targetCustomerID, "customer can only view own profile"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionRead, ResourceCustomer, targetCustomerID, "unknown role"), nil
	}
}

// CanUpdateCustomer checks if a user can update a customer's information.
// Customer can update self only (name, password, 2FA), admin can update any.
func (s *RBACService) CanUpdateCustomer(ctx context.Context, userID, userRole, targetCustomerID string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		if userID == targetCustomerID {
			return true, nil
		}
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionUpdate, ResourceCustomer, targetCustomerID, "customer can only update own profile"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionUpdate, ResourceCustomer, targetCustomerID, "unknown role"), nil
	}
}

// CanSuspendCustomer checks if a user can suspend a customer account.
// Only admin and super_admin can suspend customers.
func (s *RBACService) CanSuspendCustomer(ctx context.Context, userID, userRole string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionSuspend, ResourceCustomer, "", "customers cannot suspend accounts"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionSuspend, ResourceCustomer, "", "unknown role"), nil
	}
}

// CanViewAuditLogs checks if a user can view audit logs.
// Only admin and super_admin can view audit logs.
func (s *RBACService) CanViewAuditLogs(ctx context.Context, userID, userRole string) (bool, error) {
	switch userRole {
	case RoleSuperAdmin, RoleAdmin:
		return true, nil
	case RoleCustomer:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionRead, ResourceAuditLog, "", "customers cannot view audit logs"), nil
	default:
		return s.logDeniedAndReturn(ctx, userID, userRole, ActionRead, ResourceAuditLog, "", "unknown role"), nil
	}
}

// IsDestructiveAction returns true if the given action is considered destructive.
func IsDestructiveAction(action string) bool {
	return DestructiveActions[action]
}

// IsAdmin checks if the user has admin or super_admin role.
func IsAdmin(userRole string) bool {
	return userRole == RoleSuperAdmin || userRole == RoleAdmin
}

// IsSuperAdmin checks if the user has super_admin role.
func IsSuperAdmin(userRole string) bool {
	return userRole == RoleSuperAdmin
}

// IsCustomer checks if the user has customer role.
func IsCustomer(userRole string) bool {
	return userRole == RoleCustomer
}

// PermissionDeniedError creates a standardized permission denied error.
func PermissionDeniedError(action, resourceType string) error {
	return fmt.Errorf("%w: cannot %s %s", sharederrors.ErrForbidden, action, resourceType)
}

// ReAuthRequiredError creates a standardized re-authentication required error.
func ReAuthRequiredError(action string) error {
	return fmt.Errorf("%w: re-authentication required for %s (valid for %v)", sharederrors.ErrForbidden, action, ReAuthWindow)
}
