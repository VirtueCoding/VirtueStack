package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// BillingTransactionRepo defines the interface for billing transaction persistence.
type BillingTransactionRepo interface {
	CreditAccount(ctx context.Context, customerID string, amount int64,
		description string, idempotencyKey *string) (*models.BillingTransaction, error)
	DebitAccount(ctx context.Context, customerID string, amount int64,
		description string, referenceType, referenceID, idempotencyKey *string,
	) (*models.BillingTransaction, error)
	GetBalance(ctx context.Context, customerID string) (int64, error)
	ListByCustomer(ctx context.Context, customerID string,
		filter models.PaginationParams) ([]models.BillingTransaction, bool, string, error)
}

// BillingLedgerServiceConfig holds dependencies for BillingLedgerService.
type BillingLedgerServiceConfig struct {
	TransactionRepo BillingTransactionRepo
	Logger          *slog.Logger
}

// BillingLedgerService provides business logic for the credit ledger.
type BillingLedgerService struct {
	txRepo BillingTransactionRepo
	logger *slog.Logger
}

// NewBillingLedgerService creates a new BillingLedgerService.
func NewBillingLedgerService(cfg BillingLedgerServiceConfig) *BillingLedgerService {
	return &BillingLedgerService{
		txRepo: cfg.TransactionRepo,
		logger: cfg.Logger.With("component", "billing-ledger-service"),
	}
}

// CreditAccount adds funds to a customer's balance. Amount must be positive.
func (s *BillingLedgerService) CreditAccount(
	ctx context.Context,
	customerID string,
	amount int64,
	description string,
	idempotencyKey *string,
) (*models.BillingTransaction, error) {
	if amount <= 0 {
		return nil, sharederrors.NewValidationError("amount", "must be positive")
	}

	bt, err := s.txRepo.CreditAccount(ctx, customerID, amount, description, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("credit account %s: %w", customerID, err)
	}

	s.logger.Info("account credited",
		"customer_id", customerID,
		"amount", amount,
		"balance_after", bt.BalanceAfter,
	)
	return bt, nil
}

// DebitAccount deducts funds from a customer's balance. Amount must be positive.
func (s *BillingLedgerService) DebitAccount(
	ctx context.Context,
	customerID string,
	amount int64,
	description string,
	referenceType *string,
	referenceID *string,
	idempotencyKey *string,
) (*models.BillingTransaction, error) {
	if amount <= 0 {
		return nil, sharederrors.NewValidationError("amount", "must be positive")
	}

	bt, err := s.txRepo.DebitAccount(
		ctx, customerID, amount, description,
		referenceType, referenceID, idempotencyKey,
	)
	if err != nil {
		return nil, fmt.Errorf("debit account %s: %w", customerID, err)
	}

	s.logger.Info("account debited",
		"customer_id", customerID,
		"amount", amount,
		"balance_after", bt.BalanceAfter,
	)
	return bt, nil
}

// GetBalance returns the current balance for a customer.
func (s *BillingLedgerService) GetBalance(
	ctx context.Context, customerID string,
) (int64, error) {
	balance, err := s.txRepo.GetBalance(ctx, customerID)
	if err != nil {
		return 0, fmt.Errorf("get balance %s: %w", customerID, err)
	}
	return balance, nil
}

// GetTransactionHistory returns paginated transaction history.
func (s *BillingLedgerService) GetTransactionHistory(
	ctx context.Context,
	customerID string,
	filter models.PaginationParams,
) ([]models.BillingTransaction, bool, string, error) {
	txs, hasMore, lastID, err := s.txRepo.ListByCustomer(ctx, customerID, filter)
	if err != nil {
		return nil, false, "", fmt.Errorf("list transactions %s: %w", customerID, err)
	}
	return txs, hasMore, lastID, nil
}
