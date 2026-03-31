package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyBTCPaySignature(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		body    string
		sig     string
		wantErr bool
	}{
		{
			"valid signature",
			"my-secret",
			`{"type":"InvoiceSettled","invoiceId":"INV-1"}`,
			"sha256=" + computeHMACSHA256("my-secret", []byte(`{"type":"InvoiceSettled","invoiceId":"INV-1"}`)),
			false,
		},
		{
			"invalid signature",
			"my-secret",
			`{"type":"InvoiceSettled"}`,
			"sha256=0000000000000000000000000000000000000000000000000000000000000000",
			true,
		},
		{"empty secret", "", `{}`, "sha256=abc", true},
		{"empty signature", "secret", `{}`, "", true},
		{"invalid format no prefix", "secret", `{}`, "abc123", true},
		{"invalid hex", "secret", `{}`, "sha256=ZZZZ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyBTCPaySignature(tt.secret, tt.sig, []byte(tt.body))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestComputeHMACSHA256(t *testing.T) {
	result := computeHMACSHA256("secret", []byte("hello"))
	assert.Len(t, result, 64, "SHA256 hex digest should be 64 chars")

	// Same input → same output (deterministic)
	result2 := computeHMACSHA256("secret", []byte("hello"))
	assert.Equal(t, result, result2)

	// Different secret → different output
	result3 := computeHMACSHA256("other-secret", []byte("hello"))
	assert.NotEqual(t, result, result3)
}

func TestCentsToDecimal_Crypto(t *testing.T) {
	tests := []struct {
		name  string
		cents int64
		want  string
	}{
		{"zero", 0, "0.00"},
		{"one dollar", 100, "1.00"},
		{"fractional", 1234, "12.34"},
		{"large", 1000000, "10000.00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, centsToDecimal(tt.cents))
		})
	}
}

func TestDecimalToCents_Crypto(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"integer", "50", 5000, false},
		{"with decimals", "25.50", 2550, false},
		{"single decimal", "10.5", 1050, false},
		{"zero", "0.00", 0, false},
		{"invalid", "xyz", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decimalToCents(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
