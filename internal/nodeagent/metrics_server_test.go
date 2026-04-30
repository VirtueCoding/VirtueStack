package nodeagent

import (
	"testing"
	"time"
)

// TestNewMetricsHTTPServerTimeouts verifies that the metrics HTTP server is
// constructed with non-zero timeouts to mitigate Slowloris attacks (gosec G112).
func TestNewMetricsHTTPServerTimeouts(t *testing.T) {
	srv := newMetricsHTTPServer("127.0.0.1:0", nil)

	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want %v", srv.ReadHeaderTimeout, 5*time.Second)
	}
	if srv.ReadTimeout != 10*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", srv.ReadTimeout, 10*time.Second)
	}
	if srv.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", srv.WriteTimeout, 30*time.Second)
	}
	if srv.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout = %v, want %v", srv.IdleTimeout, 60*time.Second)
	}
	if srv.Addr != "127.0.0.1:0" {
		t.Errorf("Addr = %q, want %q", srv.Addr, "127.0.0.1:0")
	}
}
