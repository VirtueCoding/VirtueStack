package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// ExchangeRateRepository provides database operations for exchange rates.
type ExchangeRateRepository struct {
	db DB
}

// NewExchangeRateRepository creates a new ExchangeRateRepository.
func NewExchangeRateRepository(db DB) *ExchangeRateRepository {
	return &ExchangeRateRepository{db: db}
}

func scanExchangeRate(row pgx.Row) (models.ExchangeRate, error) {
	var er models.ExchangeRate
	err := row.Scan(&er.Currency, &er.RateToUSD, &er.Source, &er.UpdatedAt)
	return er, err
}

// GetRate returns the exchange rate for a currency.
func (r *ExchangeRateRepository) GetRate(
	ctx context.Context, currency string,
) (*models.ExchangeRate, error) {
	q := `SELECT currency, rate_to_usd, source, updated_at
		FROM exchange_rates WHERE currency = $1`
	rate, err := ScanRow(ctx, r.db, q, []any{currency}, scanExchangeRate)
	if err != nil {
		return nil, fmt.Errorf("get exchange rate %s: %w", currency, err)
	}
	return &rate, nil
}

// UpsertRate inserts or updates an exchange rate.
func (r *ExchangeRateRepository) UpsertRate(
	ctx context.Context, currency string, rate float64, source string,
) error {
	q := `INSERT INTO exchange_rates (currency, rate_to_usd, source, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (currency) DO UPDATE
		SET rate_to_usd = EXCLUDED.rate_to_usd,
			source = EXCLUDED.source,
			updated_at = NOW()`
	if _, err := r.db.Exec(ctx, q, currency, rate, source); err != nil {
		return fmt.Errorf("upsert exchange rate %s: %w", currency, err)
	}
	return nil
}

// ListAll returns all exchange rates.
func (r *ExchangeRateRepository) ListAll(
	ctx context.Context,
) ([]models.ExchangeRate, error) {
	q := `SELECT currency, rate_to_usd, source, updated_at
		FROM exchange_rates ORDER BY currency`
	rates, err := ScanRows(ctx, r.db, q, nil,
		func(rows pgx.Rows) (models.ExchangeRate, error) {
			return scanExchangeRate(rows)
		})
	if err != nil {
		return nil, fmt.Errorf("list exchange rates: %w", err)
	}
	return rates, nil
}
