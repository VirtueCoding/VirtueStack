package downloadutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
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

func VerifyFileIntegrity(path string, expectedSize int64, expectedChecksum string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	if expectedSize > 0 && info.Size() != expectedSize {
		return fmt.Errorf("size mismatch: got %d want %d", info.Size(), expectedSize)
	}
	if expectedChecksum == "" {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash file: %w", err)
	}

	actualChecksum := hex.EncodeToString(hash.Sum(nil))
	if actualChecksum != strings.ToLower(expectedChecksum) {
		return fmt.Errorf("checksum mismatch: got %s want %s", actualChecksum, strings.ToLower(expectedChecksum))
	}

	return nil
}
