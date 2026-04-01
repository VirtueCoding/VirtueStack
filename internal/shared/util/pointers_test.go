package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringPtr(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"non-empty string", "hello"},
		{"string with spaces", "  hello world  "},
		{"unicode string", "日本語テスト"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringPtr(tt.input)
			require.NotNil(t, result)
			assert.Equal(t, tt.input, *result)
		})
	}
}

func TestStringPtr_ReturnsDistinctPointers(t *testing.T) {
	a := StringPtr("same")
	b := StringPtr("same")
	assert.NotSame(t, a, b, "each call should return a distinct pointer")
	assert.Equal(t, *a, *b, "values should be equal")
}

func TestStringPtr_MutationDoesNotAffectOriginal(t *testing.T) {
	original := "original"
	ptr := StringPtr(original)
	*ptr = "mutated"
	assert.Equal(t, "original", original, "original value should not be affected")
	assert.Equal(t, "mutated", *ptr)
}
