package services

import (
	"context"
	"encoding/json"
	"errors"
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
	CompleteWithGatewayPaymentIDAndCredit(
		ctx context.Context, req repository.PaymentCompletionCredit,
	) (bool, error)
	CompletePayPalCaptureAndCredit(
		ctx context.Context, req repository.PayPalCaptureCredit,
	) (bool, error)
	MarkRefundedAndDebit(
		ctx context.Context, req repository.PaymentRefundDebit,
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
	PaymentRepo     BillingPaymentRepo
	SettingsRepo    *repository.SettingsRepository
	Logger          *slog.Logger
}

// PaymentService orchestrates payment operations (top-up, webhook, refund).
type PaymentService struct {
	registry     *payments.PaymentRegistry
	paymentRepo  BillingPaymentRepo
	settingsRepo *repository.SettingsRepository
	logger       *slog.Logger
}

type paymentSettlement struct {
	Payment          *models.BillingPayment
	Gateway          string
	GatewayPaymentID string
	PayPalOrderID    string
	CustomerID       string
	AmountCents      int64
	Description      string
	IdempotencyKey   string
}

// NewPaymentService creates a new PaymentService.
func NewPaymentService(cfg PaymentServiceConfig) *PaymentService {
	return &PaymentService{
		registry:     cfg.PaymentRegistry,
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
	payment, handled, err := s.paymentForCompletedEvent(ctx, gateway, event)
	if err != nil || handled {
		return err
	}

	idempotencyKey := paymentCreditIdempotencyKey(gateway, event)
	claimed, err := s.settlePayment(ctx, paymentSettlement{
		Payment:          payment,
		Gateway:          gateway,
		GatewayPaymentID: event.PaymentID,
		PayPalOrderID:    event.Metadata["payment_id"],
		CustomerID:       payment.CustomerID,
		AmountCents:      event.AmountCents,
		Description:      fmt.Sprintf("Top-up via %s", gateway),
		IdempotencyKey:   idempotencyKey,
	})
	if err != nil {
		return fmt.Errorf("complete payment: %w", err)
	}
	if !claimed {
		s.logger.Warn("ignoring duplicate gateway payment",
			"gateway", gateway,
			"gateway_payment_id", event.PaymentID,
			"payment_id", payment.ID,
		)
		return nil
	}

	s.logger.Info("payment completed and ledger credited",
		"customer_id", payment.CustomerID,
		"amount_cents", event.AmountCents,
		"gateway", gateway,
	)
	return nil
}

func (s *PaymentService) settlePayment(ctx context.Context, settlement paymentSettlement) (bool, error) {
	if settlement.Gateway == models.PaymentGatewayPayPal && settlement.PayPalOrderID != "" {
		return s.paymentRepo.CompletePayPalCaptureAndCredit(ctx, repository.PayPalCaptureCredit{
			PaymentID:      settlement.Payment.ID,
			OrderID:        settlement.PayPalOrderID,
			CaptureID:      settlement.GatewayPaymentID,
			CustomerID:     settlement.CustomerID,
			Amount:         settlement.AmountCents,
			Description:    settlement.Description,
			IdempotencyKey: settlement.IdempotencyKey,
		})
	}
	return s.paymentRepo.CompleteWithGatewayPaymentIDAndCredit(ctx, repository.PaymentCompletionCredit{
		PaymentID:        settlement.Payment.ID,
		Gateway:          settlement.Gateway,
		GatewayPaymentID: settlement.GatewayPaymentID,
		CustomerID:       settlement.CustomerID,
		Amount:           settlement.AmountCents,
		Description:      settlement.Description,
		IdempotencyKey:   settlement.IdempotencyKey,
	})
}

func (s *PaymentService) paymentForCompletedEvent(
	ctx context.Context, gateway string, event *payments.WebhookEvent,
) (*models.BillingPayment, bool, error) {
	customerID := event.Metadata["customer_id"]
	if customerID == "" {
		return nil, false, fmt.Errorf("webhook event missing customer_id metadata")
	}
	payment, handled, err := s.lookupWebhookPayment(ctx, gateway, event)
	if err != nil || handled {
		return nil, handled, err
	}
	if err := validatePaymentMatchesEvent(payment, customerID, event); err != nil {
		return nil, false, err
	}
	return payment, false, nil
}

func (s *PaymentService) lookupWebhookPayment(
	ctx context.Context, gateway string, event *payments.WebhookEvent,
) (*models.BillingPayment, bool, error) {
	paymentRef := event.Metadata["payment_id"]
	if paymentRef == "" {
		return nil, false, fmt.Errorf("webhook event missing payment_id metadata")
	}
	if gateway == models.PaymentGatewayPayPal {
		return s.lookupPayPalWebhookPayment(ctx, paymentRef, event)
	}
	payment, err := s.paymentRepo.GetByID(ctx, paymentRef)
	return payment, false, err
}

func (s *PaymentService) lookupPayPalWebhookPayment(
	ctx context.Context, orderID string, event *payments.WebhookEvent,
) (*models.BillingPayment, bool, error) {
	if event.PaymentID != "" {
		payment, err := s.paymentRepo.GetByGatewayPaymentID(
			ctx, models.PaymentGatewayPayPal, event.PaymentID,
		)
		if err == nil {
			return payment, false, nil
		}
		if !isNotFoundError(err) {
			return nil, false, err
		}
	}
	payment, err := s.paymentRepo.GetByGatewayPaymentID(
		ctx, models.PaymentGatewayPayPal, orderID,
	)
	return payment, false, err
}

func validatePaymentMatchesEvent(
	payment *models.BillingPayment, customerID string, event *payments.WebhookEvent,
) error {
	if payment.CustomerID != customerID {
		return sharederrors.NewValidationError("customer_id", "payment customer mismatch")
	}
	if payment.Amount != event.AmountCents {
		return sharederrors.NewValidationError("amount", "payment amount mismatch")
	}
	if !strings.EqualFold(payment.Currency, event.Currency) {
		return sharederrors.NewValidationError("currency", "payment currency mismatch")
	}
	return nil
}

func paymentCreditIdempotencyKey(gateway string, event *payments.WebhookEvent) string {
	if gateway == models.PaymentGatewayPayPal && event.IdempotencyKey != "" {
		return event.IdempotencyKey
	}
	if event.PaymentID != "" {
		return gateway + ":payment:" + event.PaymentID
	}
	return event.IdempotencyKey
}

func isNotFoundError(err error) bool {
	return errors.Is(err, sharederrors.ErrNotFound)
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

	err = s.paymentRepo.MarkRefundedAndDebit(ctx, repository.PaymentRefundDebit{
		PaymentID:      existing.ID,
		CustomerID:     existing.CustomerID,
		Amount:         event.AmountCents,
		Description:    fmt.Sprintf("Refund via %s", gateway),
		IdempotencyKey: event.IdempotencyKey,
	})
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
	externalPaymentID string,
	idempotencyKey string,
) error {
	existing, err := s.cryptoPaymentForCredit(ctx, gateway, gatewayPaymentID)
	if err != nil {
		return fmt.Errorf("get crypto payment: %w", err)
	}
	if err := validateCryptoPayment(existing, accountID, amountCents, currency); err != nil {
		return err
	}
	if externalPaymentID == "" {
		externalPaymentID = gatewayPaymentID
	}
	claimed, err := s.settlePayment(ctx, paymentSettlement{
		Payment:          existing,
		Gateway:          gateway,
		GatewayPaymentID: externalPaymentID,
		CustomerID:       accountID,
		AmountCents:      amountCents,
		Description:      fmt.Sprintf("Top-up via %s", gateway),
		IdempotencyKey:   idempotencyKey,
	})
	if err != nil {
		return fmt.Errorf("complete crypto payment: %w", err)
	}
	if !claimed {
		return nil
	}

	s.logger.Info("crypto payment credited to ledger",
		"customer_id", accountID,
		"amount_cents", amountCents,
		"gateway", gateway,
		"gateway_payment_id", externalPaymentID,
	)
	return nil
}

func (s *PaymentService) cryptoPaymentForCredit(
	ctx context.Context, gateway, gatewayPaymentID string,
) (*models.BillingPayment, error) {
	payment, err := s.paymentRepo.GetByGatewayPaymentID(ctx, gateway, gatewayPaymentID)
	if err == nil {
		return payment, nil
	}
	if !isNotFoundError(err) {
		return nil, err
	}
	return s.paymentRepo.GetByID(ctx, gatewayPaymentID)
}

func validateCryptoPayment(
	payment *models.BillingPayment, accountID string, amountCents int64, currency string,
) error {
	if payment.CustomerID != accountID {
		return sharederrors.NewValidationError("customer_id", "payment customer mismatch")
	}
	if payment.Amount != amountCents {
		return sharederrors.NewValidationError("amount", "payment amount mismatch")
	}
	if !strings.EqualFold(payment.Currency, currency) {
		return sharederrors.NewValidationError("currency", "payment currency mismatch")
	}
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

	gatewayPaymentID := refundGatewayPaymentID(payment)
	result, err := provider.RefundPayment(
		ctx, gatewayPaymentID, amountCents, payment.Currency,
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

	if refundGatewayPaymentID(payment) == "" {
		return sharederrors.NewValidationError("gateway_payment_id",
			"payment has no gateway reference for refund")
	}

	if amountCents <= 0 || amountCents > payment.Amount {
		return sharederrors.NewValidationError("amount",
			"refund amount must be between 1 and the original payment amount")
	}

	return nil
}

func refundGatewayPaymentID(payment *models.BillingPayment) string {
	if payment.Gateway == models.PaymentGatewayPayPal {
		if captureID := paypalCaptureIDFromMetadata(payment.Metadata); captureID != "" {
			return captureID
		}
	}
	if payment.GatewayPaymentID == nil {
		return ""
	}
	return *payment.GatewayPaymentID
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
	if result := completedPayPalCaptureResult(payment); result != nil {
		return s.creditFromCapture(ctx, payment.ID, customerID, orderID, result)
	}

	provider, err := s.registry.Get("paypal")
	if err != nil {
		return nil, fmt.Errorf("get paypal provider: %w", err)
	}

	capturer, ok := provider.(PayPalOrderCapturer)
	if !ok {
		return nil, fmt.Errorf("paypal provider missing capture support")
	}

	capID, status, currency, cents, err := capturer.CaptureOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("capture paypal order: %w", err)
	}

	result := &PayPalCaptureResult{
		CaptureID:   capID,
		Status:      status,
		AmountCents: cents,
		Currency:    currency,
	}
	if err := validatePayPalCapture(payment, result); err != nil {
		return nil, err
	}
	return s.creditFromCapture(ctx, payment.ID, customerID, orderID, result)
}

func completedPayPalCaptureResult(payment *models.BillingPayment) *PayPalCaptureResult {
	if payment.Status != models.PaymentStatusCompleted {
		return nil
	}
	captureID := paypalCaptureIDFromMetadata(payment.Metadata)
	if captureID == "" {
		return nil
	}
	return &PayPalCaptureResult{
		CaptureID:   captureID,
		Status:      "COMPLETED",
		AmountCents: payment.Amount,
		Currency:    payment.Currency,
	}
}

func paypalCaptureIDFromMetadata(metadata json.RawMessage) string {
	var data struct {
		PayPalCaptureID string `json:"paypal_capture_id"`
	}
	if len(metadata) == 0 || json.Unmarshal(metadata, &data) != nil {
		return ""
	}
	return data.PayPalCaptureID
}

func validatePayPalCapture(
	payment *models.BillingPayment, result *PayPalCaptureResult,
) error {
	if result.Status != "COMPLETED" {
		return sharederrors.NewValidationError("status", "paypal capture not completed")
	}
	if payment.Amount != result.AmountCents {
		return sharederrors.NewValidationError("amount", "paypal capture amount mismatch")
	}
	if !strings.EqualFold(payment.Currency, result.Currency) {
		return sharederrors.NewValidationError("currency", "paypal capture currency mismatch")
	}
	return nil
}

func (s *PaymentService) creditFromCapture(
	ctx context.Context, paymentRecordID, customerID, orderID string,
	result *PayPalCaptureResult,
) (*PayPalCaptureResult, error) {
	idempotencyKey := "paypal:capture:" + result.CaptureID
	claimed, err := s.paymentRepo.CompletePayPalCaptureAndCredit(
		ctx, repository.PayPalCaptureCredit{
			PaymentID:      paymentRecordID,
			OrderID:        orderID,
			CaptureID:      result.CaptureID,
			CustomerID:     customerID,
			Amount:         result.AmountCents,
			Description:    "Top-up via paypal",
			IdempotencyKey: idempotencyKey,
		})
	if err != nil {
		return nil, fmt.Errorf("complete paypal capture: %w", err)
	}
	if !claimed {
		return nil, fmt.Errorf("paypal capture already claimed: %w", sharederrors.ErrConflict)
	}

	s.logger.Info("paypal capture credited",
		"customer_id", customerID,
		"order_id", orderID,
		"capture_id", result.CaptureID,
		"amount_cents", result.AmountCents,
	)
	return result, nil
}
