package controller

import (
	"context"
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

func TestStart_ReleasesMetricsListenerWhenHTTPStartupFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	httpListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = httpListener.Close()
	})

	metricsAddr := reserveTCPAddress(t)

	server := &Server{
		config: &config.ControllerConfig{
			ListenAddr:  httpListener.Addr().String(),
			MetricsAddr: metricsAddr,
		},
		router: gin.New(),
		logger: testLogger(),
	}
	server.setupRoutes()

	err = server.Start(context.Background())
	require.Error(t, err)

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
