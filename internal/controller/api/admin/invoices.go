// Package admin provides HTTP handlers for the Admin API.
package admin

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

// ListInvoices handles GET /invoices — lists all invoices with optional filters.
func (h *AdminHandler) ListInvoices(c *gin.Context) {
	var filter models.InvoiceListFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_FILTER", "Invalid filter parameters")
		return
	}

	if filter.CustomerID != nil {
		if _, err := uuid.Parse(*filter.CustomerID); err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest,
				"INVALID_CUSTOMER_ID", "customer_id must be a valid UUID")
			return
		}
	}

	pagination := models.ParsePagination(c)

	invoices, nextCursor, err := h.invoiceService.ListAllInvoices(
		c.Request.Context(), filter, pagination.Cursor, pagination.PerPage)
	if err != nil {
		h.logger.Error("failed to list invoices",
			"error", err,
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

// GetInvoice handles GET /invoices/:id — returns a single invoice by UUID.
func (h *AdminHandler) GetInvoice(c *gin.Context) {
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

	c.JSON(http.StatusOK, models.Response{Data: invoice})
}

// VoidInvoice handles POST /invoices/:id/void — voids a draft or issued invoice.
func (h *AdminHandler) VoidInvoice(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_ID", "Invoice ID must be a valid UUID")
		return
	}

	err := h.invoiceService.VoidInvoice(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound,
				"INVOICE_NOT_FOUND", "Invoice not found")
			return
		}
		if errors.Is(err, sharederrors.ErrConflict) {
			middleware.RespondWithError(c, http.StatusConflict,
				"INVOICE_VOID_CONFLICT", err.Error())
			return
		}
		h.logger.Error("failed to void invoice",
			"error", err, "invoice_id", id,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"INVOICE_VOID_FAILED", "Failed to void invoice")
		return
	}

	h.logAuditEvent(c, "invoice.void", "billing_invoice", id, nil, true)

	c.JSON(http.StatusOK, models.Response{Data: map[string]string{
		"status": "voided",
	}})
}

// DownloadInvoicePDF handles GET /invoices/:id/pdf — streams the invoice PDF.
func (h *AdminHandler) DownloadInvoicePDF(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_ID", "Invoice ID must be a valid UUID")
		return
	}

	pdfPath, err := h.invoiceService.GetPDFPath(c.Request.Context(), id)
	if err != nil && errors.Is(err, sharederrors.ErrNotFound) {
		pdfPath, err = h.invoiceService.GeneratePDF(c.Request.Context(), id)
	}
	if err != nil {
		if errors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound,
				"INVOICE_NOT_FOUND", "Invoice not found")
			return
		}
		h.logger.Error("failed to get/generate invoice PDF",
			"error", err, "invoice_id", id,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PDF_GENERATION_FAILED", "Failed to generate invoice PDF")
		return
	}

	h.serveInvoicePDF(c, id, pdfPath)
}

// serveInvoicePDF opens the PDF file and streams it to the response.
func (h *AdminHandler) serveInvoicePDF(c *gin.Context, id, pdfPath string) {
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

	invoice, _ := h.invoiceService.GetInvoice(c.Request.Context(), id)
	filename := "invoice.pdf"
	if invoice != nil {
		filename = invoice.InvoiceNumber + ".pdf"
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	io.Copy(c.Writer, file) //nolint:errcheck
}
