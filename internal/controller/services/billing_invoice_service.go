// Package services provides business logic for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/google/uuid"
)

// BillingInvoiceServiceConfig holds dependencies for the invoice service.
type BillingInvoiceServiceConfig struct {
	InvoiceRepo     *repository.BillingInvoiceRepository
	TransactionRepo *repository.BillingTransactionRepository
	CustomerRepo    *repository.CustomerRepository
	VMRepo          *repository.VMRepository
	PlanRepo        *repository.PlanRepository
	PDFGenerator    *InvoicePDFGenerator
	Logger          *slog.Logger
}

// BillingInvoiceService handles invoice generation and management.
type BillingInvoiceService struct {
	invoiceRepo     *repository.BillingInvoiceRepository
	transactionRepo *repository.BillingTransactionRepository
	customerRepo    *repository.CustomerRepository
	vmRepo          *repository.VMRepository
	planRepo        *repository.PlanRepository
	pdfGenerator    *InvoicePDFGenerator
	logger          *slog.Logger
}

// NewBillingInvoiceService creates a new BillingInvoiceService.
func NewBillingInvoiceService(cfg BillingInvoiceServiceConfig) *BillingInvoiceService {
	return &BillingInvoiceService{
		invoiceRepo:     cfg.InvoiceRepo,
		transactionRepo: cfg.TransactionRepo,
		customerRepo:    cfg.CustomerRepo,
		vmRepo:          cfg.VMRepo,
		planRepo:        cfg.PlanRepo,
		pdfGenerator:    cfg.PDFGenerator,
		logger:          cfg.Logger.With("component", "billing-invoice-service"),
	}
}

// GenerateMonthlyInvoice creates an invoice for the given customer covering
// the specified billing period. It aggregates billing_transactions of type
// "debit" within the period, groups them by VM, and produces line items.
// Returns the created invoice or nil if no charges exist for the period.
func (s *BillingInvoiceService) GenerateMonthlyInvoice(
	ctx context.Context, customerID string, periodStart, periodEnd time.Time,
) (*models.BillingInvoice, error) {
	exists, err := s.invoiceRepo.ExistsForPeriod(ctx, customerID, periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("check existing invoice: %w", err)
	}
	if exists {
		s.logger.Info("invoice already exists for period, skipping",
			"customer_id", customerID,
			"period_start", periodStart,
			"period_end", periodEnd)
		return nil, nil
	}

	return s.buildAndCreateInvoice(ctx, customerID, periodStart, periodEnd)
}

// buildAndCreateInvoice fetches transactions and creates the invoice record.
func (s *BillingInvoiceService) buildAndCreateInvoice(
	ctx context.Context, customerID string, periodStart, periodEnd time.Time,
) (*models.BillingInvoice, error) {
	transactions, err := s.transactionRepo.ListByCustomerForPeriod(
		ctx, customerID, periodStart, periodEnd, models.BillingTxTypeDebit)
	if err != nil {
		return nil, fmt.Errorf("list transactions for period: %w", err)
	}

	if len(transactions) == 0 {
		s.logger.Debug("no charges found for period, skipping invoice",
			"customer_id", customerID, "period_start", periodStart)
		return nil, nil
	}

	lineItems := s.aggregateLineItems(transactions)
	var subtotal int64
	for _, item := range lineItems {
		subtotal += item.Amount
	}

	invoiceNumber, err := s.invoiceRepo.NextInvoiceNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("generate invoice number: %w", err)
	}

	now := time.Now().UTC()
	invoice := &models.BillingInvoice{
		ID:            uuid.New().String(),
		CustomerID:    customerID,
		InvoiceNumber: invoiceNumber,
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
		Subtotal:      subtotal,
		TaxAmount:     0,
		Total:         subtotal,
		Currency:      "USD",
		Status:        models.InvoiceStatusIssued,
		LineItems:     lineItems,
		IssuedAt:      &now,
	}

	if err := s.invoiceRepo.Create(ctx, invoice); err != nil {
		return nil, fmt.Errorf("create invoice: %w", err)
	}

	s.logger.Info("invoice generated",
		"invoice_id", invoice.ID, "invoice_number", invoiceNumber,
		"customer_id", customerID, "total", subtotal,
		"line_items_count", len(lineItems))

	return invoice, nil
}

// aggregateLineItems groups charge transactions by VM and creates
// one line item per VM with the total hours and amount.
func (s *BillingInvoiceService) aggregateLineItems(
	transactions []models.BillingTransaction,
) []models.InvoiceLineItem {
	type vmAgg struct {
		vmName string
		vmID   string
		hours  int
		amount int64
	}

	byVM := make(map[string]*vmAgg)
	var nonVMTotal int64

	for _, tx := range transactions {
		vmID := extractVMID(&tx)
		if vmID == "" {
			nonVMTotal += absInt64(tx.Amount)
			continue
		}
		agg, ok := byVM[vmID]
		if !ok {
			agg = &vmAgg{vmID: vmID}
			byVM[vmID] = agg
		}
		agg.hours++
		agg.amount += absInt64(tx.Amount)
		if agg.vmName == "" {
			agg.vmName = tx.Description
		}
	}

	items := make([]models.InvoiceLineItem, 0, len(byVM)+1)
	for _, agg := range byVM {
		unitPrice := agg.amount
		if agg.hours > 0 {
			unitPrice = agg.amount / int64(agg.hours)
		}
		items = append(items, models.InvoiceLineItem{
			Description: fmt.Sprintf("VM Usage — %s", agg.vmName),
			Quantity:    agg.hours,
			UnitPrice:   unitPrice,
			Amount:      agg.amount,
			VMName:      agg.vmName,
			VMID:        agg.vmID,
			Hours:       agg.hours,
		})
	}

	if nonVMTotal > 0 {
		items = append(items, models.InvoiceLineItem{
			Description: "Other charges",
			Quantity:    1,
			UnitPrice:   nonVMTotal,
			Amount:      nonVMTotal,
		})
	}

	return items
}

// extractVMID returns the VM ID from a transaction if it references a VM.
func extractVMID(tx *models.BillingTransaction) string {
	if tx.ReferenceType == nil || tx.ReferenceID == nil {
		return ""
	}
	if *tx.ReferenceType == models.BillingRefTypeVMUsage {
		return *tx.ReferenceID
	}
	return ""
}

// GenerateAllMonthlyInvoices generates invoices for all native billing
// customers for the given month. Returns the count of invoices generated.
func (s *BillingInvoiceService) GenerateAllMonthlyInvoices(
	ctx context.Context, year int, month time.Month,
) (int, error) {
	periodStart := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	customers, err := s.customerRepo.ListNativeBillingCustomerIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list native billing customer IDs: %w", err)
	}

	generated := 0
	for _, custID := range customers {
		invoice, genErr := s.GenerateMonthlyInvoice(ctx, custID, periodStart, periodEnd)
		if genErr != nil {
			s.logger.Error("failed to generate invoice for customer",
				"customer_id", custID, "error", genErr)
			continue
		}
		if invoice != nil {
			generated++
		}
	}

	s.logger.Info("monthly invoice generation complete",
		"year", year, "month", month,
		"generated", generated, "total_customers", len(customers))

	return generated, nil
}

// GetInvoice returns an invoice by ID.
func (s *BillingInvoiceService) GetInvoice(
	ctx context.Context, id string,
) (*models.BillingInvoice, error) {
	inv, err := s.invoiceRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get invoice: %w", err)
	}
	return inv, nil
}

// ListInvoices returns paginated invoices for a customer.
func (s *BillingInvoiceService) ListInvoices(
	ctx context.Context, customerID, cursor string, perPage int,
) ([]models.BillingInvoice, string, error) {
	return s.invoiceRepo.ListByCustomer(ctx, customerID, cursor, perPage)
}

// ListAllInvoices returns paginated invoices with optional filtering (admin).
func (s *BillingInvoiceService) ListAllInvoices(
	ctx context.Context, filter models.InvoiceListFilter, cursor string, perPage int,
) ([]models.BillingInvoice, string, error) {
	return s.invoiceRepo.ListAll(ctx, filter, cursor, perPage)
}

// VoidInvoice marks an invoice as void. Only draft or issued invoices
// can be voided. Returns ErrConflict if the invoice is already paid.
func (s *BillingInvoiceService) VoidInvoice(ctx context.Context, id string) error {
	inv, err := s.invoiceRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get invoice for void: %w", err)
	}

	if inv.Status == models.InvoiceStatusPaid {
		return fmt.Errorf("cannot void a paid invoice: %w", sharederrors.ErrConflict)
	}
	if inv.Status == models.InvoiceStatusVoid {
		return fmt.Errorf("invoice is already void: %w", sharederrors.ErrConflict)
	}

	if err := s.invoiceRepo.UpdateStatus(ctx, id, models.InvoiceStatusVoid); err != nil {
		return fmt.Errorf("void invoice: %w", err)
	}

	s.logger.Info("invoice voided",
		"invoice_id", id, "invoice_number", inv.InvoiceNumber)
	return nil
}

// MarkAsPaid marks an invoice as paid with the current timestamp.
// Only issued invoices can be marked as paid.
func (s *BillingInvoiceService) MarkAsPaid(ctx context.Context, id string) error {
	inv, err := s.invoiceRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get invoice for payment: %w", err)
	}

	if inv.Status != models.InvoiceStatusIssued {
		return fmt.Errorf(
			"only issued invoices can be marked paid (current: %s): %w",
			inv.Status, sharederrors.ErrConflict)
	}

	now := time.Now().UTC()
	if err := s.invoiceRepo.SetPaidAt(ctx, id, now); err != nil {
		return fmt.Errorf("mark invoice paid: %w", err)
	}

	s.logger.Info("invoice marked as paid",
		"invoice_id", id, "invoice_number", inv.InvoiceNumber)
	return nil
}

// GeneratePDF creates and stores a PDF for the given invoice ID.
func (s *BillingInvoiceService) GeneratePDF(ctx context.Context, id string) (string, error) {
	inv, err := s.invoiceRepo.GetByID(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get invoice for PDF: %w", err)
	}

	customer, err := s.customerRepo.GetByID(ctx, inv.CustomerID)
	if err != nil {
		return "", fmt.Errorf("get customer for PDF: %w", err)
	}

	path, err := s.pdfGenerator.GeneratePDF(ctx, inv, customer.Name, customer.Email)
	if err != nil {
		return "", fmt.Errorf("generate PDF: %w", err)
	}

	s.logger.Info("invoice PDF generated",
		"invoice_id", id, "invoice_number", inv.InvoiceNumber, "path", path)
	return path, nil
}

// GetPDFPath returns the file path of a generated invoice PDF.
func (s *BillingInvoiceService) GetPDFPath(ctx context.Context, id string) (string, error) {
	inv, err := s.invoiceRepo.GetByID(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get invoice for PDF path: %w", err)
	}

	if inv.PDFPath == nil || *inv.PDFPath == "" {
		return "", fmt.Errorf(
			"no PDF generated for invoice %s: %w",
			inv.InvoiceNumber, sharederrors.ErrNotFound)
	}

	return *inv.PDFPath, nil
}

// absInt64 returns the absolute value of an int64.
func absInt64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
