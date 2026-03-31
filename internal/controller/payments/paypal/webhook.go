package paypal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// verifyWebhookRequest holds the PayPal signature verification request.
type verifyWebhookRequest struct {
	AuthAlgo         string          `json:"auth_algo"`
	CertURL          string          `json:"cert_url"`
	TransmissionID   string          `json:"transmission_id"`
	TransmissionSig  string          `json:"transmission_sig"`
	TransmissionTime string          `json:"transmission_time"`
	WebhookID        string          `json:"webhook_id"`
	WebhookEvent     json.RawMessage `json:"webhook_event"`
}

type verifyWebhookResponse struct {
	VerificationStatus string `json:"verification_status"`
}

// VerifyWebhookSignature verifies a PayPal webhook using the
// POST /v1/notifications/verify-webhook-signature API.
func (p *Provider) VerifyWebhookSignature(
	ctx context.Context,
	headers http.Header,
	body []byte,
) error {
	token, err := p.tokenClient.GetAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("get paypal token: %w", err)
	}

	reqBody, err := p.buildVerifyRequest(headers, body)
	if err != nil {
		return err
	}

	return p.executeVerify(ctx, token, reqBody)
}

func (p *Provider) buildVerifyRequest(
	headers http.Header, body []byte,
) ([]byte, error) {
	verifyReq := verifyWebhookRequest{
		AuthAlgo:         headers.Get("Paypal-Auth-Algo"),
		CertURL:          headers.Get("Paypal-Cert-Url"),
		TransmissionID:   headers.Get("Paypal-Transmission-Id"),
		TransmissionSig:  headers.Get("Paypal-Transmission-Sig"),
		TransmissionTime: headers.Get("Paypal-Transmission-Time"),
		WebhookID:        p.webhookID,
		WebhookEvent:     body,
	}

	data, err := json.Marshal(verifyReq)
	if err != nil {
		return nil, fmt.Errorf("marshal verify request: %w", err)
	}
	return data, nil
}

func (p *Provider) executeVerify(
	ctx context.Context, token string, reqBody []byte,
) error {
	endpoint := p.tokenClient.BaseURL() +
		"/v1/notifications/verify-webhook-signature"

	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodPost, endpoint,
		strings.NewReader(string(reqBody)),
	)
	if err != nil {
		return fmt.Errorf("build verify request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("paypal verify webhook: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return fmt.Errorf("read verify response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"paypal verify webhook failed (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	var verifyResp verifyWebhookResponse
	if err := json.Unmarshal(respBody, &verifyResp); err != nil {
		return fmt.Errorf("decode verify response: %w", err)
	}
	if verifyResp.VerificationStatus != "SUCCESS" {
		return fmt.Errorf(
			"paypal webhook verification failed: %s",
			verifyResp.VerificationStatus,
		)
	}

	return nil
}
