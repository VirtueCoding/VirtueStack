// Package customer provides HTTP handlers for the Customer API.
package customer

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ListMyInvoices handles GET /invoices — lists invoices for the authenticated customer.
func (h *CustomerHandler) ListMyInvoices(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	pagination := models.ParsePagination(c)

	invoices, nextCursor, err := h.invoiceService.ListInvoices(
		c.Request.Context(), customerID, pagination.Cursor, pagination.PerPage)
	if err != nil {
		h.logger.Error("failed to list customer invoices",
			"error", err, "customer_id", customerID,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"INVOICE_LIST_FAILED", "Failed to retrieve invoices")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: invoices,
		Meta: models.PaginationMeta{
			PerPage:    pagination.PerPage,
			HasMore:    nextCursor != "",
			NextCursor: nextCursor,
		},
	})
}

// GetMyInvoice handles GET /invoices/:id — returns a single invoice
// owned by the authenticated customer.
func (h *CustomerHandler) GetMyInvoice(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_ID", "Invoice ID must be a valid UUID")
		return
	}

	invoice, err := h.invoiceService.GetInvoice(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound,
				"INVOICE_NOT_FOUND", "Invoice not found")
			return
		}
		h.logger.Error("failed to get invoice",
			"error", err, "invoice_id", id,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"INVOICE_GET_FAILED", "Failed to retrieve invoice")
		return
	}

	if invoice.CustomerID != customerID {
		middleware.RespondWithError(c, http.StatusNotFound,
			"INVOICE_NOT_FOUND", "Invoice not found")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: invoice})
}

// DownloadMyInvoicePDF handles GET /invoices/:id/pdf — streams the invoice PDF
// for the authenticated customer.
func (h *CustomerHandler) DownloadMyInvoicePDF(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_ID", "Invoice ID must be a valid UUID")
		return
	}

	invoice, err := h.invoiceService.GetInvoice(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound,
				"INVOICE_NOT_FOUND", "Invoice not found")
			return
		}
		h.logger.Error("failed to get invoice for PDF",
			"error", err, "invoice_id", id,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"INVOICE_GET_FAILED", "Failed to retrieve invoice")
		return
	}

	if invoice.CustomerID != customerID {
		middleware.RespondWithError(c, http.StatusNotFound,
			"INVOICE_NOT_FOUND", "Invoice not found")
		return
	}

	pdfPath, err := h.invoiceService.GetPDFPath(c.Request.Context(), id)
	if err != nil && errors.Is(err, sharederrors.ErrNotFound) {
		pdfPath, err = h.invoiceService.GeneratePDF(c.Request.Context(), id)
	}
	if err != nil {
		h.logger.Error("failed to get/generate invoice PDF",
			"error", err, "invoice_id", id,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PDF_GENERATION_FAILED", "Failed to generate invoice PDF")
		return
	}

	file, err := os.Open(filepath.Clean(pdfPath))
	if err != nil {
		h.logger.Error("failed to open invoice PDF file",
			"error", err, "path", pdfPath,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PDF_READ_FAILED", "Failed to read invoice PDF")
		return
	}
	defer file.Close()

	filename := invoice.InvoiceNumber + ".pdf"
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	io.Copy(c.Writer, file) //nolint:errcheck
}
