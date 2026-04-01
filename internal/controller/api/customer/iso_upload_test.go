package customer

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestISOMagicReadBytesReachesUDFRecognitionSequence(t *testing.T) {
	const udfEndOffset = 0x9001
	const udfIdentifierLength = len("TEA01")

	require.GreaterOrEqual(t, isoMagicReadBytes, udfEndOffset+udfIdentifierLength)
}

func TestIsValidISOMagic(t *testing.T) {
	tests := []struct {
		name string
		buf  []byte
		want bool
	}{
		{
			name: "accepts iso9660 signature",
			buf: func() []byte {
				buf := make([]byte, 0x8001+len("CD001"))
				copy(buf[0x8001:], []byte("CD001"))
				return buf
			}(),
			want: true,
		},
		{
			name: "accepts udf volume recognition sequence",
			buf: func() []byte {
				buf := make([]byte, 0x9001+len("TEA01"))
				copy(buf[0x8001:], []byte("BEA01"))
				copy(buf[0x8801:], []byte("NSR02"))
				copy(buf[0x9001:], []byte("TEA01"))
				return buf
			}(),
			want: true,
		},
		{
			name: "rejects missing signature in captured bytes",
			buf:  bytes.Repeat([]byte{0}, isoMagicReadBytes),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isValidISOMagic(tt.buf))
		})
	}
}
