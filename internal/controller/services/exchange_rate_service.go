package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// ExchangeRateRepo defines the interface for exchange rate persistence.
type ExchangeRateRepo interface {
	GetRate(ctx context.Context, currency string) (*models.ExchangeRate, error)
	UpsertRate(ctx context.Context, currency string, rate float64, source string) error
	ListAll(ctx context.Context) ([]models.ExchangeRate, error)
}

// ExchangeRateServiceConfig holds dependencies for ExchangeRateService.
type ExchangeRateServiceConfig struct {
	RateRepo ExchangeRateRepo
	Logger   *slog.Logger
}

// ExchangeRateService provides exchange rate operations.
type ExchangeRateService struct {
	rateRepo ExchangeRateRepo
	logger   *slog.Logger
}

// NewExchangeRateService creates a new ExchangeRateService.
func NewExchangeRateService(cfg ExchangeRateServiceConfig) *ExchangeRateService {
	return &ExchangeRateService{
		rateRepo: cfg.RateRepo,
		logger:   cfg.Logger.With("component", "exchange-rate-service"),
	}
}

// GetRate returns the exchange rate for a currency to USD.
func (s *ExchangeRateService) GetRate(
	ctx context.Context, currency string,
) (*models.ExchangeRate, error) {
	rate, err := s.rateRepo.GetRate(ctx, currency)
	if err != nil {
		return nil, fmt.Errorf("get rate for %s: %w", currency, err)
	}
	return rate, nil
}

// UpdateRate sets an exchange rate (admin-managed).
func (s *ExchangeRateService) UpdateRate(
	ctx context.Context, currency string, rateToUSD float64,
) error {
	if rateToUSD <= 0 {
		return sharederrors.NewValidationError("rate_to_usd", "must be positive")
	}
	err := s.rateRepo.UpsertRate(ctx, currency, rateToUSD, models.ExchangeRateSourceAdmin)
	if err != nil {
		return fmt.Errorf("update rate for %s: %w", currency, err)
	}
	s.logger.Info("exchange rate updated",
		"currency", currency,
		"rate_to_usd", rateToUSD,
		"source", models.ExchangeRateSourceAdmin,
	)
	return nil
}

// ListRates returns all exchange rates.
func (s *ExchangeRateService) ListRates(
	ctx context.Context,
) ([]models.ExchangeRate, error) {
	rates, err := s.rateRepo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list exchange rates: %w", err)
	}
	return rates, nil
}

// ConvertAmount converts an amount from one currency to another via USD.
// Returns the converted amount in cents (rounded to nearest).
func (s *ExchangeRateService) ConvertAmount(
	ctx context.Context,
	amount int64,
	fromCurrency string,
	toCurrency string,
) (int64, error) {
	if fromCurrency == toCurrency {
		return amount, nil
	}

	fromRate, err := s.rateRepo.GetRate(ctx, fromCurrency)
	if err != nil {
		return 0, fmt.Errorf("get rate for source %s: %w", fromCurrency, err)
	}
	toRate, err := s.rateRepo.GetRate(ctx, toCurrency)
	if err != nil {
		return 0, fmt.Errorf("get rate for target %s: %w", toCurrency, err)
	}

	// Convert: from → USD → to
	usdAmount := float64(amount) / fromRate.RateToUSD
	converted := usdAmount * toRate.RateToUSD

	return int64(math.Round(converted)), nil
}
