package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateIPAllowlistConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    IPAllowlistConfig
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid single IP",
			config: IPAllowlistConfig{
				AllowedIPs: []string{"192.168.1.1"},
			},
			wantErr: false,
		},
		{
			name: "valid CIDR",
			config: IPAllowlistConfig{
				AllowedIPs: []string{"10.0.0.0/24"},
			},
			wantErr: false,
		},
		{
			name: "valid IPv6",
			config: IPAllowlistConfig{
				AllowedIPs: []string{"::1", "2001:db8::/32"},
			},
			wantErr: false,
		},
		{
			name: "mixed valid IPs and CIDRs",
			config: IPAllowlistConfig{
				AllowedIPs: []string{"192.168.1.1", "10.0.0.0/24", "::1", "2001:db8::/32"},
			},
			wantErr: false,
		},
		{
			name: "empty entries are skipped",
			config: IPAllowlistConfig{
				AllowedIPs: []string{"", "  ", "192.168.1.1"},
			},
			wantErr: false,
		},
		{
			name: "empty list is valid",
			config: IPAllowlistConfig{
				AllowedIPs: nil,
			},
			wantErr: false,
		},
		{
			name: "invalid IP",
			config: IPAllowlistConfig{
				AllowedIPs: []string{"not-an-ip"},
			},
			wantErr:   true,
			errSubstr: "invalid IP entry",
		},
		{
			name: "invalid CIDR",
			config: IPAllowlistConfig{
				AllowedIPs: []string{"10.0.0.0/99"},
			},
			wantErr:   true,
			errSubstr: "invalid CIDR entry",
		},
		{
			name: "invalid IP among valid ones",
			config: IPAllowlistConfig{
				AllowedIPs: []string{"192.168.1.1", "bad-ip", "10.0.0.0/24"},
			},
			wantErr:   true,
			errSubstr: "invalid IP entry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPAllowlistConfig(tt.config)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errSubstr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIPAllowlist_Disabled(t *testing.T) {
	r := gin.New()
	r.Use(IPAllowlist(IPAllowlistConfig{Enabled: false}))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestIPAllowlist_AllowedIP(t *testing.T) {
	r := gin.New()
	r.Use(IPAllowlist(IPAllowlistConfig{
		Enabled:    true,
		AllowedIPs: []string{"192.168.1.1"},
	}))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestIPAllowlist_AllowedCIDR(t *testing.T) {
	r := gin.New()
	// Trust all proxies so X-Forwarded-For is used
	r.SetTrustedProxies(nil)
	r.Use(IPAllowlist(IPAllowlistConfig{
		Enabled:    true,
		AllowedIPs: []string{"10.0.0.0/24"},
	}))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.50:12345"
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestIPAllowlist_DeniedIP(t *testing.T) {
	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(IPAllowlist(IPAllowlistConfig{
		Enabled:    true,
		AllowedIPs: []string{"192.168.1.1"},
	}))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.50:12345"
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	errorObj := resp["error"].(map[string]interface{})
	assert.Equal(t, "IP_NOT_ALLOWED", errorObj["code"])
}

func TestIPAllowlist_EmptyAllowlist_DeniesAll(t *testing.T) {
	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(IPAllowlist(IPAllowlistConfig{
		Enabled:    true,
		AllowedIPs: nil,
	}))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestIPAllowlist_InvalidEntries_Skipped(t *testing.T) {
	r := gin.New()
	r.SetTrustedProxies(nil)
	r.Use(IPAllowlist(IPAllowlistConfig{
		Enabled:    true,
		AllowedIPs: []string{"not-valid", "10.0.0.0/24"},
	}))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Valid IP in the CIDR range should still work
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.50:12345"
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
