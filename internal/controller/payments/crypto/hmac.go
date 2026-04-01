package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// verifyBTCPaySignature verifies the BTCPay-Sig header against the body.
// BTCPay signature format: "sha256=HEXDIGEST"
func verifyBTCPaySignature(secret, signature string, body []byte) error {
	if secret == "" {
		return fmt.Errorf("btcpay webhook secret not configured")
	}
	if signature == "" {
		return fmt.Errorf("missing BTCPay-Sig header")
	}

	parts := strings.SplitN(signature, "=", 2)
	if len(parts) != 2 || parts[0] != "sha256" {
		return fmt.Errorf("invalid BTCPay-Sig format: %q", signature)
	}

	expectedMAC, err := hex.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("decode signature hex: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	actualMAC := mac.Sum(nil)

	if !hmac.Equal(actualMAC, expectedMAC) {
		return fmt.Errorf("btcpay HMAC signature mismatch")
	}
	return nil
}

// computeHMACSHA256 computes an HMAC-SHA256 digest for the given body.
func computeHMACSHA256(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyNOWPaymentsSignature verifies the X-Nowpayments-Sig header.
// NOWPayments uses HMAC-SHA512 for IPN signature verification.
// The body must be sorted by keys before hashing.
func verifyNOWPaymentsSignature(secret, signature string, body []byte) error {
	if secret == "" {
		return fmt.Errorf("nowpayments IPN secret not configured")
	}
	if signature == "" {
		return fmt.Errorf("missing X-Nowpayments-Sig header")
	}

	// NOWPayments requires sorting the JSON keys before computing HMAC.
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fmt.Errorf("parse IPN body for signature: %w", err)
	}
	sorted, err := json.Marshal(decoded)
	if err != nil {
		return fmt.Errorf("re-marshal sorted IPN body: %w", err)
	}

	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(sorted)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("nowpayments HMAC signature mismatch")
	}
	return nil
}
