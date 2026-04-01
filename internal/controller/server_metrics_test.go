package controller

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupRoutes_PublicEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := &Server{
		config: &config.ControllerConfig{},
		router: gin.New(),
		logger: testLogger(),
	}

	server.setupRoutes()

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "health remains public",
			path:       "/health",
			wantStatus: http.StatusOK,
		},
		{
			name:       "metrics not exposed on public router",
			path:       "/metrics",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)

			server.router.ServeHTTP(recorder, req)

			assert.Equal(t, tt.wantStatus, recorder.Code)
		})
	}
}

func TestStart_ReleasesMetricsListenerWhenHTTPServeFailsAfterMetricsStartup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	httpAddr := reserveTCPAddress(t)
	metricsAddr := reserveTCPAddress(t)

	server := &Server{
		config: &config.ControllerConfig{
			ListenAddr:  httpAddr,
			MetricsAddr: metricsAddr,
		},
		router: gin.New(),
		logger: testLogger(),
		serveHTTPFunc: func(_ *http.Server, _ net.Listener) error {
			return errors.New("simulated serve failure")
		},
	}
	server.setupRoutes()

	err := server.Start(context.Background())
	require.Error(t, err)
	require.ErrorContains(t, err, "simulated serve failure")

	metricsListener, listenErr := net.Listen("tcp", metricsAddr)
	require.NoError(t, listenErr)
	_ = metricsListener.Close()
}

func TestStop_ShutsDownMetricsWhenHTTPShutdownFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	metricsAddr := reserveTCPAddress(t)

	server := &Server{
		config: &config.ControllerConfig{
			MetricsAddr: metricsAddr,
		},
		httpServer: &http.Server{},
		logger:     testLogger(),
		shutdownHTTPFunc: func(_ *http.Server, _ context.Context) error {
			return errors.New("simulated shutdown failure")
		},
	}

	require.NoError(t, server.startMetricsHTTPServer(context.Background()))

	err := server.Stop(context.Background())
	require.Error(t, err)
	require.ErrorContains(t, err, "simulated shutdown failure")

	metricsListener, listenErr := net.Listen("tcp", metricsAddr)
	require.NoError(t, listenErr)
	_ = metricsListener.Close()
}

func reserveTCPAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	return addr
}
