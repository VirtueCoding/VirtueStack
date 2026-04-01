package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadinessHandler(t *testing.T) {
	tests := []struct {
		name          string
		dbErr         error
		natsStatus    nats.Status
		wantHTTP      int
		wantContains  string
		wantJSONField string
	}{
		{
			name:          "ready when db and nats are healthy",
			dbErr:         nil,
			natsStatus:    nats.CONNECTED,
			wantHTTP:      http.StatusOK,
			wantContains:  `"nats":"connected"`,
			wantJSONField: `"status":"ready"`,
		},
		{
			name:          "returns 503 when nats disconnected",
			dbErr:         nil,
			natsStatus:    nats.DISCONNECTED,
			wantHTTP:      http.StatusServiceUnavailable,
			wantContains:  `"nats":"disconnected"`,
			wantJSONField: `"status":"ready"`,
		},
		{
			name:         "returns 503 when db unavailable",
			dbErr:        errors.New("db down"),
			natsStatus:   nats.CONNECTED,
			wantHTTP:     http.StatusServiceUnavailable,
			wantContains: `"code":"DATABASE_UNAVAILABLE"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/ready", nil)

			srv := &Server{
				dbPool: &pgxpool.Pool{},
			}
			srv.readinessDBPing = func(context.Context) error { return tt.dbErr }
			srv.readinessNATSStatus = func() nats.Status { return tt.natsStatus }

			srv.readinessHandler(c)

			require.Equal(t, tt.wantHTTP, w.Code)
			if tt.wantContains != "" {
				assert.Contains(t, w.Body.String(), tt.wantContains)
			}
			if tt.wantJSONField != "" {
				assert.Contains(t, w.Body.String(), tt.wantJSONField)
			}
		})
	}
}
