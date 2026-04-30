package paypal

import "github.com/AbuGosok/VirtueStack/internal/controller/payments"

func paymentRequestFixture() payments.PaymentRequest {
	return payments.PaymentRequest{
		CustomerID:    "acct-1",
		CustomerEmail: "test@example.com",
		AmountCents:   1050,
		Currency:      "usd",
		Description:   "Credit Top-Up",
		ReturnURL:     "https://example.com/return",
		CancelURL:     "https://example.com/cancel",
		Metadata: map[string]string{
			"payment_id":  "pay-1",
			"customer_id": "acct-1",
		},
	}
}
