// Package services provides business logic for VirtueStack Controller.
package services

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/go-pdf/fpdf"
)

// InvoicePDFGeneratorConfig holds dependencies for PDF generation.
type InvoicePDFGeneratorConfig struct {
	InvoiceRepo  *repository.BillingInvoiceRepository
	SettingsRepo *repository.SettingsRepository
	StoragePath  string
}

// InvoicePDFGenerator creates PDF files from invoice data.
type InvoicePDFGenerator struct {
	invoiceRepo  *repository.BillingInvoiceRepository
	settingsRepo *repository.SettingsRepository
	storagePath  string
}

// NewInvoicePDFGenerator creates a new InvoicePDFGenerator.
func NewInvoicePDFGenerator(cfg InvoicePDFGeneratorConfig) *InvoicePDFGenerator {
	return &InvoicePDFGenerator{
		invoiceRepo:  cfg.InvoiceRepo,
		settingsRepo: cfg.SettingsRepo,
		storagePath:  cfg.StoragePath,
	}
}

// GeneratePDF creates a PDF for the given invoice and returns the file path.
func (g *InvoicePDFGenerator) GeneratePDF(
	ctx context.Context, invoice *models.BillingInvoice,
	customerName, customerEmail string,
) (string, error) {
	companyName := g.getSettingOrDefault(ctx, "company_name", "VirtueStack")
	companyAddr := g.getSettingOrDefault(ctx, "company_address", "")

	pdfBytes, err := g.renderPDF(invoice, customerName, customerEmail, companyName, companyAddr)
	if err != nil {
		return "", fmt.Errorf("render PDF: %w", err)
	}

	dir := filepath.Join(
		g.storagePath,
		fmt.Sprintf("%d", invoice.PeriodStart.Year()),
		fmt.Sprintf("%02d", invoice.PeriodStart.Month()),
	)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create invoice directory: %w", err)
	}

	filePath := filepath.Join(dir, invoice.InvoiceNumber+".pdf")
	if err := os.WriteFile(filePath, pdfBytes, 0o640); err != nil {
		return "", fmt.Errorf("write invoice PDF: %w", err)
	}

	if err := g.invoiceRepo.SetPDFPath(ctx, invoice.ID, filePath); err != nil {
		return "", fmt.Errorf("save PDF path: %w", err)
	}

	return filePath, nil
}

// renderPDF generates the PDF bytes for an invoice.
func (g *InvoicePDFGenerator) renderPDF(
	invoice *models.BillingInvoice,
	customerName, customerEmail, companyName, companyAddress string,
) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()

	g.renderHeader(pdf, companyName, companyAddress, invoice)
	g.renderCustomerInfo(pdf, customerName, customerEmail)
	g.renderLineItemsTable(pdf, invoice.LineItems)
	g.renderTotals(pdf, invoice)
	g.renderFooter(pdf)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("generate PDF output: %w", err)
	}
	return buf.Bytes(), nil
}

// renderHeader renders the invoice header with company info and invoice number.
func (g *InvoicePDFGenerator) renderHeader(
	pdf *fpdf.Fpdf, companyName, companyAddress string,
	invoice *models.BillingInvoice,
) {
	pdf.SetFont("Helvetica", "B", 24)
	pdf.Cell(100, 12, "INVOICE")
	pdf.Ln(14)

	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(100, 100, 100)
	pdf.Cell(100, 6, companyName)
	pdf.Ln(5)
	if companyAddress != "" {
		pdf.Cell(100, 6, companyAddress)
		pdf.Ln(5)
	}
	pdf.Ln(5)

	pdf.SetTextColor(0, 0, 0)
	g.renderField(pdf, "Invoice Number:", invoice.InvoiceNumber)
	issuedDate := invoice.CreatedAt.Format("January 2, 2006")
	if invoice.IssuedAt != nil {
		issuedDate = invoice.IssuedAt.Format("January 2, 2006")
	}
	g.renderField(pdf, "Date Issued:", issuedDate)
	g.renderField(pdf, "Billing Period:", fmt.Sprintf("%s — %s",
		invoice.PeriodStart.Format("Jan 2, 2006"),
		invoice.PeriodEnd.AddDate(0, 0, -1).Format("Jan 2, 2006")))
	g.renderField(pdf, "Status:", invoice.Status)
	pdf.Ln(12)
}

// renderField renders a labeled field in the header.
func (g *InvoicePDFGenerator) renderField(pdf *fpdf.Fpdf, label, value string) {
	pdf.SetFont("Helvetica", "B", 10)
	pdf.Cell(40, 6, label)
	pdf.SetFont("Helvetica", "", 10)
	pdf.Cell(60, 6, value)
	pdf.Ln(6)
}

// renderCustomerInfo renders the bill-to customer section.
func (g *InvoicePDFGenerator) renderCustomerInfo(
	pdf *fpdf.Fpdf, customerName, customerEmail string,
) {
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(100, 100, 100)
	pdf.Cell(40, 6, "BILL TO")
	pdf.Ln(6)

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 10)
	pdf.Cell(100, 6, customerName)
	pdf.Ln(5)
	pdf.Cell(100, 6, customerEmail)
	pdf.Ln(12)
}

// renderLineItemsTable renders the line items in a table format.
func (g *InvoicePDFGenerator) renderLineItemsTable(
	pdf *fpdf.Fpdf, items []models.InvoiceLineItem,
) {
	pdf.SetFillColor(240, 240, 240)
	pdf.SetFont("Helvetica", "B", 9)

	pdf.CellFormat(80, 8, "Description", "1", 0, "L", true, 0, "")
	pdf.CellFormat(25, 8, "Qty/Hours", "1", 0, "C", true, 0, "")
	pdf.CellFormat(35, 8, "Unit Price", "1", 0, "R", true, 0, "")
	pdf.CellFormat(35, 8, "Amount", "1", 0, "R", true, 0, "")
	pdf.Ln(8)

	pdf.SetFont("Helvetica", "", 9)
	for _, item := range items {
		desc := item.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		pdf.CellFormat(80, 7, desc, "1", 0, "L", false, 0, "")
		pdf.CellFormat(25, 7, fmt.Sprintf("%d", item.Quantity), "1", 0, "C", false, 0, "")
		pdf.CellFormat(35, 7, formatCents(item.UnitPrice), "1", 0, "R", false, 0, "")
		pdf.CellFormat(35, 7, formatCents(item.Amount), "1", 0, "R", false, 0, "")
		pdf.Ln(7)
	}
}

// renderTotals renders the subtotal, tax, and total.
func (g *InvoicePDFGenerator) renderTotals(
	pdf *fpdf.Fpdf, invoice *models.BillingInvoice,
) {
	pdf.Ln(4)
	pdf.SetFont("Helvetica", "", 10)

	pdf.Cell(105, 7, "")
	pdf.CellFormat(35, 7, "Subtotal:", "", 0, "R", false, 0, "")
	pdf.CellFormat(35, 7, formatCents(invoice.Subtotal), "", 0, "R", false, 0, "")
	pdf.Ln(7)

	pdf.Cell(105, 7, "")
	pdf.CellFormat(35, 7, "Tax:", "", 0, "R", false, 0, "")
	pdf.CellFormat(35, 7, formatCents(invoice.TaxAmount), "", 0, "R", false, 0, "")
	pdf.Ln(7)

	pdf.SetFont("Helvetica", "B", 11)
	pdf.Cell(105, 8, "")
	pdf.CellFormat(35, 8, "Total:", "", 0, "R", false, 0, "")
	pdf.CellFormat(35, 8, fmt.Sprintf("%s %s", invoice.Currency, formatCents(invoice.Total)),
		"", 0, "R", false, 0, "")
	pdf.Ln(8)
}

// renderFooter renders a small footer note.
func (g *InvoicePDFGenerator) renderFooter(pdf *fpdf.Fpdf) {
	pdf.Ln(15)
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(150, 150, 150)
	pdf.Cell(175, 5, fmt.Sprintf("Generated on %s", time.Now().UTC().Format("2006-01-02 15:04 UTC")))
}

// formatCents formats an integer cents amount as a dollar string.
func formatCents(cents int64) string {
	if cents < 0 {
		return fmt.Sprintf("-$%d.%02d", -cents/100, -cents%100)
	}
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

// getSettingOrDefault fetches a system setting or returns a default value.
func (g *InvoicePDFGenerator) getSettingOrDefault(ctx context.Context, key, defaultVal string) string {
	setting, err := g.settingsRepo.Get(ctx, key)
	if err != nil || setting == nil || setting.Value == "" {
		return defaultVal
	}
	return setting.Value
}
