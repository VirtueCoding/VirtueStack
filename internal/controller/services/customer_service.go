// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
)

// CustomerService provides business logic for managing customer accounts.
// Customers are the primary users of the platform who own and manage VMs.
type CustomerService struct {
	customerRepo *repository.CustomerRepository
	auditRepo    *repository.AuditRepository
	logger       *slog.Logger
}

// NewCustomerService creates a new CustomerService with the given dependencies.
func NewCustomerService(customerRepo *repository.CustomerRepository, auditRepo *repository.AuditRepository, logger *slog.Logger) *CustomerService {
	return &CustomerService{
		customerRepo: customerRepo,
		auditRepo:    auditRepo,
		logger:       logger.With("component", "customer-service"),
	}
}

// GetByID retrieves a customer by their UUID.
// Returns ErrNotFound if the customer doesn't exist or has been deleted.
func (s *CustomerService) GetByID(ctx context.Context, id string) (*models.Customer, error) {
	customer, err := s.customerRepo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("customer not found: %s", id)
		}
		return nil, fmt.Errorf("getting customer: %w", err)
	}
	return customer, nil
}

// List returns a paginated list of customers with optional filtering.
// Supports filtering by status and search query (email/name).
func (s *CustomerService) List(ctx context.Context, filter repository.CustomerListFilter) ([]models.Customer, int, error) {
	customers, total, err := s.customerRepo.List(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("listing customers: %w", err)
	}
	return customers, total, nil
}

// Update updates a customer's profile information.
// Only certain fields like name can be updated through this method.
// Password changes should go through the AuthService.
func (s *CustomerService) Update(ctx context.Context, actorID, actorIP string, customer *models.Customer) error {
	// Verify customer exists
	existing, err := s.customerRepo.GetByID(ctx, customer.ID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("customer not found: %s", customer.ID)
		}
		return fmt.Errorf("getting customer: %w", err)
	}

	// Track changes for audit log
	changes := map[string]any{}
	if customer.Name != existing.Name {
		changes["name"] = map[string]string{"from": existing.Name, "to": customer.Name}
	}
	if customer.Email != existing.Email {
		changes["email"] = map[string]string{"from": existing.Email, "to": customer.Email}
	}

	// Preserve immutable fields
	customer.PasswordHash = existing.PasswordHash
	customer.Status = existing.Status
	customer.TOTPEnabled = existing.TOTPEnabled
	customer.TOTPSecretEncrypted = existing.TOTPSecretEncrypted
	customer.TOTPBackupCodesHash = existing.TOTPBackupCodesHash
	customer.WHMCSClientID = existing.WHMCSClientID
	customer.CreatedAt = existing.CreatedAt

	// Update customer in repository
	if err := s.customerRepo.Update(ctx, customer); err != nil {
		// Log failed audit
		s.logAudit(ctx, actorID, actorIP, "customer.update", customer.ID, changes, false, err.Error())
		return fmt.Errorf("updating customer: %w", err)
	}

	// Log successful audit
	s.logAudit(ctx, actorID, actorIP, "customer.update", customer.ID, changes, true, "")

	s.logger.Info("customer updated",
		"customer_id", customer.ID,
		"email", util.MaskEmail(customer.Email),
		"name", customer.Name)

	return nil
}

// logAudit creates an audit log entry for customer operations.
func (s *CustomerService) logAudit(ctx context.Context, actorID, actorIP, action, resourceID string, changes map[string]any, success bool, errMsg string) {
	// json.Marshal error intentionally ignored: input is map[string]any with only
	// string values; marshalling cannot fail for this type.
	changesJSON, _ := json.Marshal(changes)
	errMsgPtr := (*string)(nil)
	if errMsg != "" {
		errMsgPtr = &errMsg
	}

	audit := &models.AuditLog{
		ActorID:      &actorID,
		ActorType:    models.AuditActorAdmin,
		ActorIP:      &actorIP,
		Action:       action,
		ResourceType: "customer",
		ResourceID:   &resourceID,
		Changes:      changesJSON,
		Success:      success,
		ErrorMessage: errMsgPtr,
	}

	if err := s.auditRepo.Append(ctx, audit); err != nil {
		s.logger.Error("failed to append audit log",
			"action", action,
			"resource_id", resourceID,
			"error", err)
	}
}

// Suspend suspends a customer account.
// Suspended customers cannot log in or perform actions on their VMs.
// All their VMs are typically stopped when an account is suspended.
func (s *CustomerService) Suspend(ctx context.Context, id string) error {
	// Verify customer exists and is active
	customer, err := s.customerRepo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("customer not found: %s", id)
		}
		return fmt.Errorf("getting customer: %w", err)
	}

	if customer.Status == models.CustomerStatusSuspended {
		return fmt.Errorf("customer is already suspended")
	}

	if customer.Status == models.CustomerStatusDeleted {
		return fmt.Errorf("cannot suspend a deleted customer")
	}

	// Update status to suspended
	if err := s.customerRepo.UpdateStatus(ctx, id, models.CustomerStatusSuspended); err != nil {
		return fmt.Errorf("suspending customer: %w", err)
	}

	s.logger.Info("customer suspended",
		"customer_id", id,
		"email", util.MaskEmail(customer.Email))

	return nil
}

// Unsuspend reactivates a suspended customer account.
// The customer can log in and manage their VMs again.
func (s *CustomerService) Unsuspend(ctx context.Context, id string) error {
	// Verify customer exists and is suspended
	customer, err := s.customerRepo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("customer not found: %s", id)
		}
		return fmt.Errorf("getting customer: %w", err)
	}

	if customer.Status != models.CustomerStatusSuspended {
		return fmt.Errorf("customer is not suspended (status: %s)", customer.Status)
	}

	// Update status to active
	if err := s.customerRepo.UpdateStatus(ctx, id, models.CustomerStatusActive); err != nil {
		return fmt.Errorf("unsuspending customer: %w", err)
	}

	s.logger.Info("customer unsuspended",
		"customer_id", id,
		"email", util.MaskEmail(customer.Email))

	return nil
}

// Delete soft-deletes a customer account.
// The customer record is retained but marked as deleted.
// This is typically used for GDPR compliance or account closure requests.
func (s *CustomerService) Delete(ctx context.Context, id string) error {
	// Verify customer exists
	customer, err := s.customerRepo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("customer not found: %s", id)
		}
		return fmt.Errorf("getting customer: %w", err)
	}

	// Soft delete the customer
	if err := s.customerRepo.SoftDelete(ctx, id); err != nil {
		return fmt.Errorf("deleting customer: %w", err)
	}

	s.logger.Info("customer deleted",
		"customer_id", id,
		"email", util.MaskEmail(customer.Email))

	return nil
}

// GetByEmail retrieves a customer by their email address.
// Returns ErrNotFound if no customer matches the email.
func (s *CustomerService) GetByEmail(ctx context.Context, email string) (*models.Customer, error) {
	customer, err := s.customerRepo.GetByEmail(ctx, email)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("customer not found with email: %s", email)
		}
		return nil, fmt.Errorf("getting customer by email: %w", err)
	}
	return customer, nil
}

// Create creates a new customer account.
// The customer will be created with active status and the provided credentials.
func (s *CustomerService) Create(ctx context.Context, actorID, actorIP string, customer *models.Customer) (*models.Customer, error) {
	// Create the customer in the repository
	if err := s.customerRepo.Create(ctx, customer); err != nil {
		return nil, fmt.Errorf("creating customer: %w", err)
	}

	// Log audit event
	changes := map[string]any{
		"email": customer.Email,
		"name":  customer.Name,
	}
	s.logAudit(ctx, actorID, actorIP, "customer.create", customer.ID, changes, true, "")

	s.logger.Info("customer created",
		"customer_id", customer.ID,
		"email", util.MaskEmail(customer.Email),
		"name", customer.Name)

	return customer, nil
}

type ProfileUpdateParams struct {
	Name  *string
	Email *string
	Phone *string
}

func (s *CustomerService) UpdateProfile(ctx context.Context, customerID, actorIP string, params ProfileUpdateParams) (*models.Customer, error) {
	existing, err := s.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("getting customer: %w", err)
	}

	changes := map[string]any{}
	if params.Name != nil && *params.Name != existing.Name {
		changes["name"] = map[string]string{"from": existing.Name, "to": *params.Name}
	}
	if params.Email != nil && *params.Email != existing.Email {
		changes["email"] = map[string]string{"from": existing.Email, "to": *params.Email}
	}
	if params.Phone != nil {
		existingPhone := ""
		if existing.Phone != nil {
			existingPhone = *existing.Phone
		}
		newPhone := *params.Phone
		if newPhone != existingPhone {
			changes["phone"] = map[string]string{"from": existingPhone, "to": newPhone}
		}
	}

	if len(changes) == 0 {
		return existing, nil
	}

	repoParams := repository.ProfileUpdateParams{
		Name:  params.Name,
		Email: params.Email,
		Phone: params.Phone,
	}

	updated, err := s.customerRepo.UpdateProfile(ctx, customerID, repoParams)
	if err != nil {
		s.logAudit(ctx, customerID, actorIP, "customer.profile.update", customerID, changes, false, err.Error())
		return nil, fmt.Errorf("updating profile: %w", err)
	}

	s.logAudit(ctx, customerID, actorIP, "customer.profile.update", customerID, changes, true, "")

	s.logger.Info("customer profile updated",
		"customer_id", customerID,
		"email", util.MaskEmail(updated.Email))

	return updated, nil
}
