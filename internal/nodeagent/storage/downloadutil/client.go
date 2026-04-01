package downloadutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

const (
	responseHeaderTimeout = 30 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	expectContinueTimeout = 1 * time.Second
	idleConnTimeout       = 90 * time.Second
	dialTimeout           = 30 * time.Second
)

func NewHTTPClient(dialContext func(context.Context, string, string) (net.Conn, error), maxRedirects int) *http.Client {
	if maxRedirects <= 0 {
		maxRedirects = 5
	}
	if dialContext == nil {
		dialer := &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}
		dialContext = dialer.DialContext
	}
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           dialContext,
			ResponseHeaderTimeout: responseHeaderTimeout,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ExpectContinueTimeout: expectContinueTimeout,
			IdleConnTimeout:       idleConnTimeout,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects (max %d)", maxRedirects)
			}
			return nil
		},
	}
}
