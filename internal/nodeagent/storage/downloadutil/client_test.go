package downloadutil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestNewHTTPClient(t *testing.T) {
	dialCalled := false
	client := NewHTTPClient(func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialCalled = true
		return nil, errors.New("dial blocked for test")
	}, 5)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", client.Transport)
	}

	if transport.ResponseHeaderTimeout != responseHeaderTimeout {
		t.Fatalf("ResponseHeaderTimeout = %v, want %v", transport.ResponseHeaderTimeout, responseHeaderTimeout)
	}
	if transport.TLSHandshakeTimeout != tlsHandshakeTimeout {
		t.Fatalf("TLSHandshakeTimeout = %v, want %v", transport.TLSHandshakeTimeout, tlsHandshakeTimeout)
	}
	if transport.ExpectContinueTimeout != expectContinueTimeout {
		t.Fatalf("ExpectContinueTimeout = %v, want %v", transport.ExpectContinueTimeout, expectContinueTimeout)
	}
	if transport.IdleConnTimeout != idleConnTimeout {
		t.Fatalf("IdleConnTimeout = %v, want %v", transport.IdleConnTimeout, idleConnTimeout)
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/file.iso", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	if err := client.CheckRedirect(req, make([]*http.Request, 5)); err == nil {
		t.Fatal("CheckRedirect() expected redirect limit error")
	}

	if _, err := transport.DialContext(context.Background(), "tcp", "example.com:443"); err == nil {
		t.Fatal("DialContext() expected injected error")
	}
	if !dialCalled {
		t.Fatal("DialContext() did not call injected dialer")
	}
}
