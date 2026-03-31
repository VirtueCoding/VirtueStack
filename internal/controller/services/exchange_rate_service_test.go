package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockExchangeRateRepo struct {
	getRateFunc   func(ctx context.Context, currency string) (*models.ExchangeRate, error)
	upsertRateFunc func(ctx context.Context, currency string, rate float64, source string) error
	listAllFunc   func(ctx context.Context) ([]models.ExchangeRate, error)
}

func (m *mockExchangeRateRepo) GetRate(ctx context.Context, currency string) (*models.ExchangeRate, error) {
	return m.getRateFunc(ctx, currency)
}

func (m *mockExchangeRateRepo) UpsertRate(ctx context.Context, currency string, rate float64, source string) error {
	return m.upsertRateFunc(ctx, currency, rate, source)
}

func (m *mockExchangeRateRepo) ListAll(ctx context.Context) ([]models.ExchangeRate, error) {
	return m.listAllFunc(ctx)
}

func newTestExchangeRateService(repo *mockExchangeRateRepo) *ExchangeRateService {
	return NewExchangeRateService(ExchangeRateServiceConfig{
		RateRepo: repo,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestExchangeRateService_GetRate(t *testing.T) {
	tests := []struct {
		name      string
		currency  string
		setupRepo func() *mockExchangeRateRepo
		wantErr   bool
		checkRate func(t *testing.T, rate *models.ExchangeRate)
	}{
		{
			name:     "happy path",
			currency: "EUR",
			setupRepo: func() *mockExchangeRateRepo {
				return &mockExchangeRateRepo{
					getRateFunc: func(_ context.Context, currency string) (*models.ExchangeRate, error) {
						return &models.ExchangeRate{
							Currency:  currency,
							RateToUSD: 0.92,
							Source:    models.ExchangeRateSourceAdmin,
						}, nil
					},
				}
			},
			wantErr: false,
			checkRate: func(t *testing.T, rate *models.ExchangeRate) {
				assert.Equal(t, "EUR", rate.Currency)
				assert.InDelta(t, 0.92, rate.RateToUSD, 0.001)
			},
		},
		{
			name:     "not found returns error",
			currency: "XYZ",
			setupRepo: func() *mockExchangeRateRepo {
				return &mockExchangeRateRepo{
					getRateFunc: func(_ context.Context, _ string) (*models.ExchangeRate, error) {
						return nil, sharederrors.ErrNotFound
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestExchangeRateService(tt.setupRepo())
			rate, err := svc.GetRate(context.Background(), tt.currency)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.checkRate != nil {
				tt.checkRate(t, rate)
			}
		})
	}
}

func TestExchangeRateService_UpdateRate(t *testing.T) {
	tests := []struct {
		name      string
		rate      float64
		setupRepo func() *mockExchangeRateRepo
		wantErr   bool
		checkErr  func(t *testing.T, err error)
	}{
		{
			name: "positive rate succeeds",
			rate: 1.25,
			setupRepo: func() *mockExchangeRateRepo {
				return &mockExchangeRateRepo{
					upsertRateFunc: func(_ context.Context, _ string, _ float64, _ string) error {
						return nil
					},
				}
			},
			wantErr: false,
		},
		{
			name: "zero rate returns validation error",
			rate: 0,
			setupRepo: func() *mockExchangeRateRepo {
				return &mockExchangeRateRepo{}
			},
			wantErr: true,
			checkErr: func(t *testing.T, err error) {
				var ve *sharederrors.ValidationError
				require.True(t, errors.As(err, &ve))
				assert.Equal(t, "rate_to_usd", ve.Field)
			},
		},
		{
			name: "negative rate returns validation error",
			rate: -0.5,
			setupRepo: func() *mockExchangeRateRepo {
				return &mockExchangeRateRepo{}
			},
			wantErr: true,
			checkErr: func(t *testing.T, err error) {
				var ve *sharederrors.ValidationError
				require.True(t, errors.As(err, &ve))
				assert.Equal(t, "rate_to_usd", ve.Field)
			},
		},
		{
			name: "repo error propagated",
			rate: 1.10,
			setupRepo: func() *mockExchangeRateRepo {
				return &mockExchangeRateRepo{
					upsertRateFunc: func(_ context.Context, _ string, _ float64, _ string) error {
						return errors.New("db write failed")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestExchangeRateService(tt.setupRepo())
			err := svc.UpdateRate(context.Background(), "EUR", tt.rate)
			if tt.wantErr {
				require.Error(t, err)
				if tt.checkErr != nil {
					tt.checkErr(t, err)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestExchangeRateService_ListRates(t *testing.T) {
	tests := []struct {
		name      string
		setupRepo func() *mockExchangeRateRepo
		wantCount int
		wantErr   bool
	}{
		{
			name: "returns all rates",
			setupRepo: func() *mockExchangeRateRepo {
				return &mockExchangeRateRepo{
					listAllFunc: func(_ context.Context) ([]models.ExchangeRate, error) {
						return []models.ExchangeRate{
							{Currency: "EUR", RateToUSD: 0.92},
							{Currency: "GBP", RateToUSD: 0.79},
							{Currency: "JPY", RateToUSD: 149.50},
						}, nil
					},
				}
			},
			wantCount: 3,
			wantErr:   false,
		},
		{
			name: "repo error propagated",
			setupRepo: func() *mockExchangeRateRepo {
				return &mockExchangeRateRepo{
					listAllFunc: func(_ context.Context) ([]models.ExchangeRate, error) {
						return nil, errors.New("query timeout")
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestExchangeRateService(tt.setupRepo())
			rates, err := svc.ListRates(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, rates, tt.wantCount)
		})
	}
}

func TestExchangeRateService_ConvertAmount(t *testing.T) {
	rateMap := map[string]*models.ExchangeRate{
		"USD": {Currency: "USD", RateToUSD: 1.0},
		"EUR": {Currency: "EUR", RateToUSD: 0.92},
		"GBP": {Currency: "GBP", RateToUSD: 0.79},
	}

	happyRepo := func() *mockExchangeRateRepo {
		return &mockExchangeRateRepo{
			getRateFunc: func(_ context.Context, currency string) (*models.ExchangeRate, error) {
				r, ok := rateMap[currency]
				if !ok {
					return nil, sharederrors.ErrNotFound
				}
				return r, nil
			},
		}
	}

	tests := []struct {
		name       string
		amount     int64
		from       string
		to         string
		setupRepo  func() *mockExchangeRateRepo
		wantAmount int64
		wantErr    bool
	}{
		{
			name:       "same currency returns same amount",
			amount:     1000,
			from:       "USD",
			to:         "USD",
			setupRepo:  happyRepo,
			wantAmount: 1000,
		},
		{
			name:       "USD to EUR conversion",
			amount:     1000,
			from:       "USD",
			to:         "EUR",
			setupRepo:  happyRepo,
			wantAmount: 920, // 1000 / 1.0 * 0.92 = 920
		},
		{
			name:       "EUR to GBP cross-rate via USD",
			amount:     1000,
			from:       "EUR",
			to:         "GBP",
			setupRepo:  happyRepo,
			wantAmount: 859, // 1000 / 0.92 * 0.79 ≈ 858.69 → 859 (rounded)
		},
		{
			name:   "missing source rate returns error",
			amount: 1000,
			from:   "BRL",
			to:     "USD",
			setupRepo: func() *mockExchangeRateRepo {
				return &mockExchangeRateRepo{
					getRateFunc: func(_ context.Context, currency string) (*models.ExchangeRate, error) {
						if currency == "BRL" {
							return nil, sharederrors.ErrNotFound
						}
						return rateMap[currency], nil
					},
				}
			},
			wantErr: true,
		},
		{
			name:   "missing target rate returns error",
			amount: 1000,
			from:   "USD",
			to:     "BRL",
			setupRepo: func() *mockExchangeRateRepo {
				return &mockExchangeRateRepo{
					getRateFunc: func(_ context.Context, currency string) (*models.ExchangeRate, error) {
						if currency == "BRL" {
							return nil, sharederrors.ErrNotFound
						}
						return rateMap[currency], nil
					},
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestExchangeRateService(tt.setupRepo())
			result, err := svc.ConvertAmount(context.Background(), tt.amount, tt.from, tt.to)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantAmount, result)
		})
	}
}
