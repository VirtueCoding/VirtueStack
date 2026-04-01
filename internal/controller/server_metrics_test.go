package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
