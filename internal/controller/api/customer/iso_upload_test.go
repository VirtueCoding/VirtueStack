package customer

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	sharedconfig "github.com/AbuGosok/VirtueStack/internal/shared/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestISOMagicReadBytesCoverage(t *testing.T) {
	tests := []struct {
		name    string
		wantMin int
	}{
		{
			name:    "captures full udf recognition sequence",
			wantMin: 0x9001 + len("TEA01"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.GreaterOrEqual(t, isoMagicReadBytes, tt.wantMin)
		})
	}
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
			name: "accepts udf volume recognition sequence with nsr03",
			buf: func() []byte {
				buf := make([]byte, 0x9001+len("TEA01"))
				copy(buf[0x8001:], []byte("BEA01"))
				copy(buf[0x8801:], []byte("NSR03"))
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

func TestCustomerHandlerValidateISOFile_UsesConfiguredMaxISOLimit(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)

	tests := []struct {
		name            string
		maxISOSizeBytes int64
		fileSize        int64
		wantValid       bool
		wantStatus      int
		wantCode        string
		wantMessage     string
	}{
		{
			name:            "rejects file above configured limit",
			maxISOSizeBytes: 5 * gib,
			fileSize:        6 * gib,
			wantValid:       false,
			wantStatus:      http.StatusBadRequest,
			wantCode:        "FILE_TOO_LARGE",
			wantMessage:     "ISO file exceeds maximum allowed size of 5 GB",
		},
		{
			name:            "accepts file below configured limit",
			maxISOSizeBytes: 7 * gib,
			fileSize:        6 * gib,
			wantValid:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/vms/vm-1/iso/upload", nil)

			handler := &CustomerHandler{
				logger:          testAuthHandlerLogger(),
				maxISOSizeBytes: tt.maxISOSizeBytes,
			}

			valid := handler.validateISOFile(ctx, &multipart.FileHeader{
				Filename: "installer.iso",
				Size:     tt.fileSize,
			})
			assert.Equal(t, tt.wantValid, valid)

			if tt.wantValid {
				assert.Equal(t, http.StatusOK, recorder.Code)
				return
			}

			assert.Equal(t, tt.wantStatus, recorder.Code)

			var resp map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
			errObj, ok := resp["error"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, errObj["code"])
			assert.Equal(t, tt.wantMessage, errObj["message"])
		})
	}
}

func TestCustomerHandlerResolvedMaxISOSizeBytes_UsesSharedDefault(t *testing.T) {
	handler := &CustomerHandler{}

	assert.Equal(t, sharedconfig.DefaultISOMaxSizeBytes(), handler.resolvedMaxISOSizeBytes())
}
