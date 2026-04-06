package services

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// BillingPaymentRepo defines the interface for payment persistence.
type BillingPaymentRepo interface {
	Create(ctx context.Context, payment *models.BillingPayment) error
	GetByID(ctx context.Context, id string) (*models.BillingPayment, error)
	GetByGatewayPaymentID(
		ctx context.Context, gateway, gatewayPaymentID string,
	) (*models.BillingPayment, error)
	UpdateStatus(
		ctx context.Context, id, status string, gatewayPaymentID *string,
	) error
	ListByCustomer(
		ctx context.Context, customerID string, filter models.PaginationParams,
	) ([]models.BillingPayment, bool, string, error)
	ListAll(
		ctx context.Context, filter repository.BillingPaymentListFilter,
	) ([]models.BillingPayment, bool, string, error)
}

// PaymentListFilter holds optional filters for listing payments (API layer).
type PaymentListFilter struct {
	CustomerID *string
	Gateway    *string
	Status     *string
	models.PaginationParams
}

// PaymentServiceConfig holds dependencies for the PaymentService.
type PaymentServiceConfig struct {
	PaymentRegistry *payments.PaymentRegistry
	LedgerService   *BillingLedgerService
	PaymentRepo     BillingPaymentRepo
	SettingsRepo    *repository.SettingsRepository
	Logger          *slog.Logger
}

// PaymentService orchestrates payment operations (top-up, webhook, refund).
type PaymentService struct {
	registry     *payments.PaymentRegistry
	ledger       *BillingLedgerService
	paymentRepo  BillingPaymentRepo
	settingsRepo *repository.SettingsRepository
	logger       *slog.Logger
}

// NewPaymentService creates a new PaymentService.
func NewPaymentService(cfg PaymentServiceConfig) *PaymentService {
	return &PaymentService{
		registry:     cfg.PaymentRegistry,
		ledger:       cfg.LedgerService,
		paymentRepo:  cfg.PaymentRepo,
		settingsRepo: cfg.SettingsRepo,
		logger:       cfg.Logger.With("component", "payment-service"),
	}
}

// InitiateTopUp creates a pending payment record and a gateway checkout session.
func (s *PaymentService) InitiateTopUp(
	ctx context.Context,
	customerID, email string,
	amountCents int64,
	currency, gateway, returnURL, cancelURL string,
) (*payments.PaymentSession, string, error) {
	provider, err := s.registry.Get(gateway)
	if err != nil {
		return nil, "", fmt.Errorf("get payment provider: %w", err)
	}

	payment := &models.BillingPayment{
		CustomerID: customerID,
		Gateway:    gateway,
		Amount:     amountCents,
		Currency:   currency,
		Status:     models.PaymentStatusPending,
	}
	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		return nil, "", fmt.Errorf("create payment record: %w", err)
	}

	sess, err := provider.CreatePaymentSession(ctx, payments.PaymentRequest{
		CustomerID:    customerID,
		CustomerEmail: email,
		AmountCents:   amountCents,
		Currency:      currency,
		Description:   "Credit Top-Up",
		ReturnURL:     returnURL,
		CancelURL:     cancelURL,
		Metadata: map[string]string{
			"payment_id":  payment.ID,
			"customer_id": customerID,
		},
	})
	if err != nil {
		return nil, "", fmt.Errorf("create payment session: %w", err)
	}

	// Store gateway session ID (e.g. PayPal order ID) for ownership validation on capture
	if sess.GatewaySessionID != "" {
		gwID := sess.GatewaySessionID
		if err := s.paymentRepo.UpdateStatus(
			ctx, payment.ID, models.PaymentStatusPending, &gwID,
		); err != nil {
			s.logger.Error("failed to store gateway session ID",
				"payment_id", payment.ID, "error", err)
		}
	}

	s.logger.Info("top-up payment initiated",
		"payment_id", payment.ID,
		"customer_id", customerID,
		"gateway", gateway,
		"amount_cents", amountCents,
	)

	return sess, payment.ID, nil
}

// HandleWebhook processes an incoming gateway webhook.
func (s *PaymentService) HandleWebhook(
	ctx context.Context,
	gateway string,
	payload []byte,
	signature string,
) error {
	provider, err := s.registry.Get(gateway)
	if err != nil {
		return fmt.Errorf("get payment provider: %w", err)
	}

	event, err := provider.HandleWebhook(ctx, payload, signature)
	if err != nil {
		return fmt.Errorf("handle webhook: %w", err)
	}

	if event == nil {
		return nil
	}

	return s.routeWebhookEvent(ctx, gateway, event)
}

func (s *PaymentService) routeWebhookEvent(
	ctx context.Context, gateway string, event *payments.WebhookEvent,
) error {
	switch event.Type {
	case payments.WebhookEventPaymentCompleted:
		return s.handlePaymentCompleted(ctx, gateway, event)
	case payments.WebhookEventPaymentFailed:
		return s.handlePaymentFailed(ctx, event)
	case payments.WebhookEventRefundCompleted:
		return s.handleRefundCompleted(ctx, gateway, event)
	default:
		return nil
	}
}

func (s *PaymentService) handlePaymentCompleted(
	ctx context.Context, gateway string, event *payments.WebhookEvent,
) error {
	if gateway == models.PaymentGatewayPayPal {
		return s.handlePayPalPaymentCompleted(ctx, event)
	}

	customerID := event.Metadata["customer_id"]
	paymentID := event.Metadata["payment_id"]

	if paymentID != "" {
		if err := s.paymentRepo.UpdateStatus(
			ctx, paymentID, models.PaymentStatusCompleted, &event.PaymentID,
		); err != nil {
			s.logger.Error("failed to update payment status",
				"payment_id", paymentID, "error", err)
		}
	}

	if customerID == "" {
		return fmt.Errorf("webhook event missing customer_id metadata")
	}

	_, err := s.ledger.CreditAccount(
		ctx, customerID, event.AmountCents,
		fmt.Sprintf("Top-up via %s", gateway),
		&event.IdempotencyKey,
	)
	if err != nil {
		return fmt.Errorf("credit account: %w", err)
	}

	s.logger.Info("payment completed and ledger credited",
		"customer_id", customerID,
		"amount_cents", event.AmountCents,
		"gateway", gateway,
	)
	return nil
}

func (s *PaymentService) handlePayPalPaymentCompleted(
	ctx context.Context, event *payments.WebhookEvent,
) error {
	orderID := event.Metadata["payment_id"]
	if orderID == "" {
		return sharederrors.NewValidationError("payment_id",
			"paypal webhook missing order reference")
	}

	payment, err := s.paymentRepo.GetByGatewayPaymentID(
		ctx, models.PaymentGatewayPayPal, orderID,
	)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return sharederrors.NewValidationError("payment_id",
				"paypal webhook order reference not found")
		}
		return fmt.Errorf("resolve paypal payment %s: %w", orderID, err)
	}

	if payloadCustomerID := event.Metadata["customer_id"]; payloadCustomerID != "" &&
		payloadCustomerID != payment.CustomerID {
		return sharederrors.NewValidationError("customer_id",
			"paypal webhook customer mismatch")
	}

	if event.AmountCents != payment.Amount {
		return sharederrors.NewValidationError("amount",
			"paypal webhook amount mismatch")
	}

	if !strings.EqualFold(event.Currency, payment.Currency) {
		return sharederrors.NewValidationError("currency",
			"paypal webhook currency mismatch")
	}

	captureID := event.PaymentID
	if updateErr := s.paymentRepo.UpdateStatus(
		ctx, payment.ID, models.PaymentStatusCompleted, &captureID,
	); updateErr != nil {
		return fmt.Errorf("update paypal payment status: %w", updateErr)
	}

	_, err = s.ledger.CreditAccount(
		ctx, payment.CustomerID, event.AmountCents,
		"Top-up via paypal",
		&event.IdempotencyKey,
	)
	if err != nil {
		return fmt.Errorf("credit account: %w", err)
	}

	s.logger.Info("paypal payment completed and ledger credited",
		"payment_id", payment.ID,
		"customer_id", payment.CustomerID,
		"gateway_order_id", orderID,
		"capture_id", captureID,
		"amount_cents", event.AmountCents,
	)
	return nil
}

func (s *PaymentService) handlePaymentFailed(
	ctx context.Context, event *payments.WebhookEvent,
) error {
	paymentID := event.Metadata["payment_id"]
	if paymentID != "" {
		return s.paymentRepo.UpdateStatus(
			ctx, paymentID, models.PaymentStatusFailed, &event.PaymentID,
		)
	}
	return nil
}

func (s *PaymentService) handleRefundCompleted(
	ctx context.Context, gateway string, event *payments.WebhookEvent,
) error {
	existing, err := s.paymentRepo.GetByGatewayPaymentID(
		ctx, gateway, event.PaymentID,
	)
	if err != nil {
		return fmt.Errorf("get payment for refund: %w", err)
	}

	if err := s.paymentRepo.UpdateStatus(
		ctx, existing.ID, models.PaymentStatusRefunded, nil,
	); err != nil {
		return fmt.Errorf("update payment status for refund: %w", err)
	}

	refType := models.BillingRefTypeRefund
	_, err = s.ledger.DebitAccount(
		ctx, existing.CustomerID, event.AmountCents,
		fmt.Sprintf("Refund via %s", gateway),
		&refType, &existing.ID, &event.IdempotencyKey,
	)
	return err
}

// CreditFromPayment credits a customer's billing account from a crypto payment.
// This is called by the crypto webhook handler after signature verification.
func (s *PaymentService) CreditFromPayment(
	ctx context.Context,
	accountID string,
	amountCents int64,
	currency string,
	gateway string,
	gatewayPaymentID string,
	idempotencyKey string,
) error {
	// Update payment record status to completed
	existing, lookupErr := s.paymentRepo.GetByGatewayPaymentID(ctx, gateway, gatewayPaymentID)
	if lookupErr == nil && existing != nil {
		if err := s.paymentRepo.UpdateStatus(
			ctx, existing.ID, models.PaymentStatusCompleted, &gatewayPaymentID,
		); err != nil {
			s.logger.Error("failed to update crypto payment status",
				"payment_id", existing.ID, "error", err)
		}
	}

	_, err := s.ledger.CreditAccount(
		ctx, accountID, amountCents,
		fmt.Sprintf("Top-up via %s", gateway),
		&idempotencyKey,
	)
	if err != nil {
		return fmt.Errorf("credit account: %w", err)
	}

	s.logger.Info("crypto payment credited to ledger",
		"customer_id", accountID,
		"amount_cents", amountCents,
		"gateway", gateway,
		"gateway_payment_id", gatewayPaymentID,
	)
	return nil
}

// GetPaymentHistory returns paginated payment history for a customer.
func (s *PaymentService) GetPaymentHistory(
	ctx context.Context, customerID string, filter models.PaginationParams,
) ([]models.BillingPayment, bool, string, error) {
	return s.paymentRepo.ListByCustomer(ctx, customerID, filter)
}

// ListAllPayments returns paginated payments across all customers (admin).
func (s *PaymentService) ListAllPayments(
	ctx context.Context, filter PaymentListFilter,
) ([]models.BillingPayment, bool, string, error) {
	repoFilter := repository.BillingPaymentListFilter{
		CustomerID:       filter.CustomerID,
		Gateway:          filter.Gateway,
		Status:           filter.Status,
		PaginationParams: filter.PaginationParams,
	}
	return s.paymentRepo.ListAll(ctx, repoFilter)
}

// TopUpConfig holds the top-up configuration returned to clients.
type TopUpConfig struct {
	MinAmountCents int64    `json:"min_amount_cents"`
	MaxAmountCents int64    `json:"max_amount_cents"`
	Presets        []int64  `json:"presets"`
	Gateways       []string `json:"gateways"`
	Currency       string   `json:"currency"`
}

// GetTopUpConfig returns the top-up configuration.
func (s *PaymentService) GetTopUpConfig(ctx context.Context) (*TopUpConfig, error) {
	gateways := s.registry.Available()

	minAmount := int64(500)
	maxAmount := int64(50000)
	presets := []int64{500, 1000, 2500, 5000, 10000}

	if s.settingsRepo != nil {
		s.loadTopUpSettings(ctx, &minAmount, &maxAmount, &presets)
	}

	return &TopUpConfig{
		MinAmountCents: minAmount,
		MaxAmountCents: maxAmount,
		Presets:        presets,
		Gateways:       gateways,
		Currency:       "USD",
	}, nil
}

func (s *PaymentService) loadTopUpSettings(
	ctx context.Context, minAmount, maxAmount *int64, presets *[]int64,
) {
	if setting, err := s.settingsRepo.Get(ctx, "billing.topup.min_amount"); err == nil {
		if v, parseErr := strconv.ParseInt(setting.Value, 10, 64); parseErr == nil {
			*minAmount = v
		}
	}
	if setting, err := s.settingsRepo.Get(ctx, "billing.topup.max_amount"); err == nil {
		if v, parseErr := strconv.ParseInt(setting.Value, 10, 64); parseErr == nil {
			*maxAmount = v
		}
	}
	if setting, err := s.settingsRepo.Get(ctx, "billing.topup.presets"); err == nil {
		if parsed := parseInt64Slice(setting.Value); len(parsed) > 0 {
			*presets = parsed
		}
	}
}

func parseInt64Slice(s string) []int64 {
	parts := strings.Split(s, ",")
	result := make([]int64, 0, len(parts))
	for _, part := range parts {
		v, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil {
			result = append(result, v)
		}
	}
	return result
}

// RefundPayment initiates a refund for a completed payment.
func (s *PaymentService) RefundPayment(
	ctx context.Context, paymentID string, amountCents int64,
) (*payments.RefundResult, error) {
	payment, err := s.paymentRepo.GetByID(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("get payment for refund: %w", err)
	}

	if err := s.validateRefund(payment, amountCents); err != nil {
		return nil, err
	}

	provider, err := s.registry.Get(payment.Gateway)
	if err != nil {
		return nil, fmt.Errorf("get payment provider: %w", err)
	}

	result, err := provider.RefundPayment(
		ctx, *payment.GatewayPaymentID, amountCents, payment.Currency,
	)
	if err != nil {
		return nil, fmt.Errorf("process refund via %s: %w", payment.Gateway, err)
	}

	s.logger.Info("refund processed",
		"payment_id", paymentID,
		"refund_id", result.GatewayRefundID,
		"amount_cents", amountCents,
	)

	return result, nil
}

func (s *PaymentService) validateRefund(
	payment *models.BillingPayment, amountCents int64,
) error {
	if payment.Status != models.PaymentStatusCompleted {
		return sharederrors.NewValidationError("status",
			"can only refund completed payments")
	}

	if payment.GatewayPaymentID == nil || *payment.GatewayPaymentID == "" {
		return sharederrors.NewValidationError("gateway_payment_id",
			"payment has no gateway reference for refund")
	}

	if amountCents <= 0 || amountCents > payment.Amount {
		return sharederrors.NewValidationError("amount",
			"refund amount must be between 1 and the original payment amount")
	}

	return nil
}

// PayPalWebhookVerifier can verify PayPal webhook signatures.
type PayPalWebhookVerifier interface {
	VerifyWebhookSignature(
		ctx context.Context, headers http.Header, body []byte,
	) error
}

// HandlePayPalWebhook verifies signature via PayPal API, then
// processes the webhook event through the standard flow.
func (s *PaymentService) HandlePayPalWebhook(
	ctx context.Context, headers http.Header, payload []byte,
) error {
	provider, err := s.registry.Get("paypal")
	if err != nil {
		return fmt.Errorf("get paypal provider: %w", err)
	}

	verifier, ok := provider.(PayPalWebhookVerifier)
	if !ok {
		return fmt.Errorf("paypal provider missing webhook verifier")
	}

	if err := verifier.VerifyWebhookSignature(ctx, headers, payload); err != nil {
		return fmt.Errorf("verify paypal webhook: %w", err)
	}

	return s.HandleWebhook(ctx, "paypal", payload, "")
}

// PayPalOrderCapturer can capture an approved PayPal order.
type PayPalOrderCapturer interface {
	CaptureOrder(ctx context.Context, orderID string) (captureID, status, currency string, amountCents int64, err error)
}

// PayPalCaptureResult holds the result of a PayPal order capture.
type PayPalCaptureResult struct {
	CaptureID   string `json:"capture_id"`
	Status      string `json:"status"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
}

// CapturePayPalOrder captures an approved PayPal order, updates the
// payment record, and credits the customer's account.
func (s *PaymentService) CapturePayPalOrder(
	ctx context.Context, customerID, orderID string,
) (*PayPalCaptureResult, error) {
	// Validate ownership: look up the payment record by PayPal order ID
	payment, err := s.paymentRepo.GetByGatewayPaymentID(ctx, "paypal", orderID)
	if err != nil {
		return nil, fmt.Errorf("payment for order %s: %w", orderID, sharederrors.ErrNotFound)
	}
	if payment.CustomerID != customerID {
		return nil, fmt.Errorf("payment ownership mismatch: %w", sharederrors.ErrForbidden)
	}

	provider, err := s.registry.Get("paypal")
	if err != nil {
		return nil, fmt.Errorf("get paypal provider: %w", err)
	}

	capturer, ok := provider.(PayPalOrderCapturer)
	if !ok {
		return nil, fmt.Errorf("paypal provider missing capture support")
	}

	capID, status, currency, cents, captureErr := capturer.CaptureOrder(ctx, orderID)
	if captureErr != nil {
		return nil, fmt.Errorf("capture paypal order: %w", captureErr)
	}

	result := &PayPalCaptureResult{
		CaptureID:   capID,
		Status:      status,
		AmountCents: cents,
		Currency:    currency,
	}
	return s.creditFromCapture(ctx, payment.ID, customerID, orderID, result)
}

func (s *PaymentService) creditFromCapture(
	ctx context.Context, paymentRecordID, customerID, orderID string,
	result *PayPalCaptureResult,
) (*PayPalCaptureResult, error) {
	// Update payment record to completed with the capture ID
	captureRef := result.CaptureID
	if err := s.paymentRepo.UpdateStatus(
		ctx, paymentRecordID, models.PaymentStatusCompleted, &captureRef,
	); err != nil {
		s.logger.Error("failed to update payment status after capture",
			"payment_id", paymentRecordID, "error", err)
	}

	idempotencyKey := "paypal:capture:" + result.CaptureID
	_, err := s.ledger.CreditAccount(
		ctx, customerID, result.AmountCents,
		"Top-up via paypal",
		&idempotencyKey,
	)
	if err != nil {
		return nil, fmt.Errorf("credit account: %w", err)
	}

	s.logger.Info("paypal capture credited",
		"customer_id", customerID,
		"order_id", orderID,
		"capture_id", result.CaptureID,
		"amount_cents", result.AmountCents,
	)
	return result, nil
}
