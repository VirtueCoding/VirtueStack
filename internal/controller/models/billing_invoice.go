// Package models provides data model types for VirtueStack Controller.
package models

import "time"

// Invoice status constants define the lifecycle states of a billing invoice.
const (
	InvoiceStatusDraft  = "draft"
	InvoiceStatusIssued = "issued"
	InvoiceStatusPaid   = "paid"
	InvoiceStatusVoid   = "void"
)

// BillingInvoice represents an invoice generated from billing transactions.
type BillingInvoice struct {
	ID            string            `json:"id" db:"id"`
	CustomerID    string            `json:"customer_id" db:"customer_id"`
	InvoiceNumber string            `json:"invoice_number" db:"invoice_number"`
	PeriodStart   time.Time         `json:"period_start" db:"period_start"`
	PeriodEnd     time.Time         `json:"period_end" db:"period_end"`
	Subtotal      int64             `json:"subtotal" db:"subtotal"`
	TaxAmount     int64             `json:"tax_amount" db:"tax_amount"`
	Total         int64             `json:"total" db:"total"`
	Currency      string            `json:"currency" db:"currency"`
	Status        string            `json:"status" db:"status"`
	LineItems     []InvoiceLineItem `json:"line_items" db:"line_items"`
	IssuedAt      *time.Time        `json:"issued_at,omitempty" db:"issued_at"`
	PaidAt        *time.Time        `json:"paid_at,omitempty" db:"paid_at"`
	PDFPath       *string           `json:"-" db:"pdf_path"`
	HasPDF        bool              `json:"has_pdf" db:"-"`
	CreatedAt     time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at" db:"updated_at"`
}

// InvoiceLineItem represents a single line item on an invoice.
// All monetary amounts are stored in integer minor units (cents).
type InvoiceLineItem struct {
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
	UnitPrice   int64  `json:"unit_price"`
	Amount      int64  `json:"amount"`
	VMName      string `json:"vm_name,omitempty"`
	VMID        string `json:"vm_id,omitempty"`
	PlanName    string `json:"plan_name,omitempty"`
	Hours       int    `json:"hours,omitempty"`
}

// InvoiceListFilter holds query parameters for filtering invoices.
type InvoiceListFilter struct {
	CustomerID *string `form:"customer_id" validate:"omitempty,uuid"`
	Status     *string `form:"status" validate:"omitempty,oneof=draft issued paid void"`
	StartDate  *string `form:"start_date" validate:"omitempty,datetime=2006-01-02"`
	EndDate    *string `form:"end_date" validate:"omitempty,datetime=2006-01-02"`
}

// InvoiceResponse wraps a BillingInvoice for API responses, adding the
// customer name for admin convenience.
type InvoiceResponse struct {
	BillingInvoice
	CustomerName  string `json:"customer_name,omitempty"`
	CustomerEmail string `json:"customer_email,omitempty"`
}
