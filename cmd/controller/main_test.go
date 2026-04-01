package main

import (
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller"
	sharedconfig "github.com/AbuGosok/VirtueStack/internal/shared/config"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func TestConnectNATS_PassesConfiguredAuthToken(t *testing.T) {
	originalConnector := natsConnector
	t.Cleanup(func() {
		natsConnector = originalConnector
	})

	var capturedOptions nats.Options
	natsConnector = func(url string, options ...nats.Option) (*nats.Conn, error) {
		for _, option := range options {
			require.NoError(t, option(&capturedOptions))
		}
		return nil, fmt.Errorf("stop after inspecting options")
	}

	_, _, err := connectNATS("nats://example:4222", "super-secret-token", testMainLogger())
	require.Error(t, err)
	require.Contains(t, err.Error(), "stop after inspecting options")
	require.Equal(t, "super-secret-token", capturedOptions.Token)
	require.Equal(t, "VirtueStack-Controller", capturedOptions.Name)
}

func TestBuildNodeClient_UsesMTLSConstructorWhenClientCertificateConfigured(t *testing.T) {
	originalSecure := secureNodeClientFactory
	originalMTLS := mTLSNodeClientFactory
	originalLookup := envLookup
	t.Cleanup(func() {
		secureNodeClientFactory = originalSecure
		mTLSNodeClientFactory = originalMTLS
		envLookup = originalLookup
	})

	cfg := &controller.Config{
		ControllerConfig: &sharedconfig.ControllerConfig{
			Environment: "development",
		},
	}

	secureCalled := false
	mTLSCalled := false

	secureNodeClientFactory = func(caCertPath string, logger *slog.Logger) (*controller.NodeClient, error) {
		secureCalled = true
		return nil, fmt.Errorf("unexpected secure factory call")
	}
	mTLSNodeClientFactory = func(caCertPath, clientCertPath, clientKeyPath string, logger *slog.Logger) (*controller.NodeClient, error) {
		mTLSCalled = true
		require.Equal(t, "ca.pem", caCertPath)
		require.Equal(t, "client.pem", clientCertPath)
		require.Equal(t, "client.key", clientKeyPath)
		return controller.InsecureNodeClient(logger), nil
	}
	envLookup = func(key string) string {
		switch key {
		case "TLS_CA_FILE":
			return "ca.pem"
		case "TLS_CERT_FILE":
			return "client.pem"
		case "TLS_KEY_FILE":
			return "client.key"
		default:
			return ""
		}
	}

	client, err := buildNodeClient(cfg, testMainLogger())
	require.NoError(t, err)
	require.NotNil(t, client)
	require.False(t, secureCalled)
	require.True(t, mTLSCalled)
}

func TestBuildNodeClient_UsesSecureConstructorWithoutClientCertificate(t *testing.T) {
	originalSecure := secureNodeClientFactory
	originalMTLS := mTLSNodeClientFactory
	originalLookup := envLookup
	t.Cleanup(func() {
		secureNodeClientFactory = originalSecure
		mTLSNodeClientFactory = originalMTLS
		envLookup = originalLookup
	})

	cfg := &controller.Config{
		ControllerConfig: &sharedconfig.ControllerConfig{
			Environment: "development",
		},
	}

	secureCalled := false
	mTLSCalled := false

	secureNodeClientFactory = func(caCertPath string, logger *slog.Logger) (*controller.NodeClient, error) {
		secureCalled = true
		require.Equal(t, "ca.pem", caCertPath)
		return controller.InsecureNodeClient(logger), nil
	}
	mTLSNodeClientFactory = func(caCertPath, clientCertPath, clientKeyPath string, logger *slog.Logger) (*controller.NodeClient, error) {
		mTLSCalled = true
		return nil, fmt.Errorf("unexpected mtls factory call")
	}
	envLookup = func(key string) string {
		if key == "TLS_CA_FILE" {
			return "ca.pem"
		}
		return ""
	}

	client, err := buildNodeClient(cfg, testMainLogger())
	require.NoError(t, err)
	require.NotNil(t, client)
	require.True(t, secureCalled)
	require.False(t, mTLSCalled)
}

func TestBuildNodeClient_RequiresCAFileInProduction(t *testing.T) {
	originalLookup := envLookup
	t.Cleanup(func() {
		envLookup = originalLookup
	})
	envLookup = func(string) string { return "" }

	cfg := &controller.Config{
		ControllerConfig: &sharedconfig.ControllerConfig{
			Environment: "production",
		},
	}

	client, err := buildNodeClient(cfg, testMainLogger())
	require.Nil(t, client)
	require.Error(t, err)
	require.Contains(t, err.Error(), "TLS_CA_FILE must be set in production")
}

func TestBuildNodeClient_RejectsPartialMTLSConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name: "certificate without key",
			env: map[string]string{
				"TLS_CERT_FILE": "client.pem",
			},
			wantErr: "TLS_CERT_FILE and TLS_KEY_FILE must either both be set or both be empty",
		},
		{
			name: "ca and certificate without key",
			env: map[string]string{
				"TLS_CA_FILE":   "ca.pem",
				"TLS_CERT_FILE": "client.pem",
			},
			wantErr: "TLS_CERT_FILE and TLS_KEY_FILE must either both be set or both be empty",
		},
		{
			name: "key without certificate",
			env: map[string]string{
				"TLS_KEY_FILE": "client.key",
			},
			wantErr: "TLS_CERT_FILE and TLS_KEY_FILE must either both be set or both be empty",
		},
		{
			name: "ca and key without certificate",
			env: map[string]string{
				"TLS_CA_FILE":  "ca.pem",
				"TLS_KEY_FILE": "client.key",
			},
			wantErr: "TLS_CERT_FILE and TLS_KEY_FILE must either both be set or both be empty",
		},
		{
			name: "certificate pair without ca",
			env: map[string]string{
				"TLS_CERT_FILE": "client.pem",
				"TLS_KEY_FILE":  "client.key",
			},
			wantErr: "TLS_CA_FILE must be set when TLS_CERT_FILE and TLS_KEY_FILE are configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalSecure := secureNodeClientFactory
			originalMTLS := mTLSNodeClientFactory
			originalLookup := envLookup
			t.Cleanup(func() {
				secureNodeClientFactory = originalSecure
				mTLSNodeClientFactory = originalMTLS
				envLookup = originalLookup
			})

			secureCalled := false
			mTLSCalled := false

			secureNodeClientFactory = func(string, *slog.Logger) (*controller.NodeClient, error) {
				secureCalled = true
				return nil, fmt.Errorf("unexpected secure factory call")
			}
			mTLSNodeClientFactory = func(string, string, string, *slog.Logger) (*controller.NodeClient, error) {
				mTLSCalled = true
				return nil, fmt.Errorf("unexpected mtls factory call")
			}
			envLookup = func(key string) string {
				return tt.env[key]
			}

			cfg := &controller.Config{
				ControllerConfig: &sharedconfig.ControllerConfig{
					Environment: "development",
				},
			}

			client, err := buildNodeClient(cfg, testMainLogger())
			require.Nil(t, client)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
			require.False(t, secureCalled)
			require.False(t, mTLSCalled)
		})
	}
}

func testMainLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
