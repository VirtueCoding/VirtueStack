// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// CustomerService provides business logic for managing customer accounts.
// Customers are the primary users of the platform who own and manage VMs.
type CustomerService struct {
	customerRepo *repository.CustomerRepository
	logger       *slog.Logger
}

// NewCustomerService creates a new CustomerService with the given dependencies.
func NewCustomerService(customerRepo *repository.CustomerRepository, logger *slog.Logger) *CustomerService {
	return &CustomerService{
		customerRepo: customerRepo,
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
func (s *CustomerService) Update(ctx context.Context, customer *models.Customer) error {
	// Verify customer exists
	existing, err := s.customerRepo.GetByID(ctx, customer.ID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("customer not found: %s", customer.ID)
		}
		return fmt.Errorf("getting customer: %w", err)
	}

	// Preserve immutable fields
	customer.Email = existing.Email
	customer.PasswordHash = existing.PasswordHash
	customer.Status = existing.Status
	customer.TOTPEnabled = existing.TOTPEnabled
	customer.TOTPSecretEncrypted = existing.TOTPSecretEncrypted
	customer.TOTPBackupCodesHash = existing.TOTPBackupCodesHash
	customer.WHMCSClientID = existing.WHMCSClientID
	customer.CreatedAt = existing.CreatedAt

	// Note: The CustomerRepository doesn't have a general Update method yet.
	// This would require adding one. For now, we log the update.
	s.logger.Info("customer update requested",
		"customer_id", customer.ID,
		"name", customer.Name)

	// TODO: Implement repository.Update method for customers
	return fmt.Errorf("customer update not yet implemented in repository")
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
		"email", customer.Email)

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
		"email", customer.Email)

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
		"email", customer.Email)

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