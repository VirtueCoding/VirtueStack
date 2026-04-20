package payments_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AbuGosok/VirtueStack/internal/controller/payments"
)

func TestPaymentProviderInterfaceCompiles(t *testing.T) {
	var _ payments.PaymentProvider
	_ = payments.PaymentRequest{}
	_ = payments.PaymentSession{}
	_ = payments.WebhookEvent{}
	_ = payments.PaymentStatus{}
	_ = payments.RefundResult{}
}

func TestWebhookEventTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		val  payments.WebhookEventType
		want string
	}{
		{"payment completed", payments.WebhookEventPaymentCompleted, "payment.completed"},
		{"payment failed", payments.WebhookEventPaymentFailed, "payment.failed"},
		{"refund completed", payments.WebhookEventRefundCompleted, "refund.completed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(tt.val))
		})
	}
}
