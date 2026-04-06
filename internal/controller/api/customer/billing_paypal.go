package customer

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// capturePayPalPaymentRequest holds the order ID from PayPal redirect.
type capturePayPalPaymentRequest struct {
	OrderID string `json:"order_id" validate:"required"`
}

// CapturePayPalPayment captures an approved PayPal order and credits
// the customer's billing account.
func (h *CustomerHandler) CapturePayPalPayment(c *gin.Context) {
	var req capturePayPalPaymentRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		respondBindingError(c, err, "Invalid request")
		return
	}

	customerID := middleware.GetUserID(c)

	result, err := h.paymentService.CapturePayPalOrder(
		c.Request.Context(), customerID, req.OrderID,
	)
	if err != nil {
		h.logger.Error("failed to capture paypal order",
			"error", err,
			"customer_id", customerID,
			"order_id", req.OrderID,
			"correlation_id", middleware.GetCorrelationID(c),
		)
		middleware.RespondWithError(c, http.StatusBadRequest,
			"CAPTURE_FAILED", "Failed to capture PayPal payment")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: result})
}
