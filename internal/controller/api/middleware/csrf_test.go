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

func init() {
	gin.SetMode(gin.TestMode)
}

func TestSkipCSRFForAPIKey(t *testing.T) {
	config := DefaultCSRFConfig()

	tests := []struct {
		name         string
		setupContext func(c *gin.Context)
		method       string
		wantStatus   int
		description  string
	}{
		{
			name:         "GET request succeeds without API key",
			setupContext: func(c *gin.Context) {},
			method:       http.MethodGet,
			wantStatus:   http.StatusOK,
			description:  "GET requests should pass through for JWT sessions",
		},
		{
			name: "POST with API key skips CSRF",
			setupContext: func(c *gin.Context) {
				c.Set("api_key_id", "key-123")
			},
			method:     http.MethodPost,
			wantStatus: http.StatusOK,
			description: "API key authenticated requests should skip CSRF validation",
		},
		{
			name:         "POST without API key requires CSRF",
			setupContext: func(c *gin.Context) {},
			method:       http.MethodPost,
			wantStatus:   http.StatusForbidden,
			description:  "JWT sessions should require CSRF token for state-changing requests",
		},
		{
			name: "POST with empty API key requires CSRF",
			setupContext: func(c *gin.Context) {
				c.Set("api_key_id", "")
			},
			method:     http.MethodPost,
			wantStatus: http.StatusForbidden,
			description: "Empty API key should not bypass CSRF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(func(c *gin.Context) {
				tt.setupContext(c)
				c.Next()
			})
			r.Use(SkipCSRFForAPIKey(config))
			r.Handle(tt.method, "/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(tt.method, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.wantStatus, w.Code, tt.description)
			if tt.wantStatus == http.StatusForbidden {
				var body ErrorResponse
				err := json.Unmarshal(w.Body.Bytes(), &body)
				require.NoError(t, err)
				assert.Contains(t, body.Error.Code, "CSRF")
			}
		})
	}
}

func TestSkipCSRFForAPIKey_SetsCookieForGet(t *testing.T) {
	config := DefaultCSRFConfig()

	r := gin.New()
	r.Use(SkipCSRFForAPIKey(config))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should set CSRF cookie for GET requests
	cookies := w.Result().Cookies()
	var csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == config.CookieName {
			csrfCookie = c
			break
		}
	}
	require.NotNil(t, csrfCookie, "CSRF cookie should be set for GET requests")
	assert.NotEmpty(t, csrfCookie.Value)
}

func TestSkipCSRFForAPIKey_NoCookieForAPIKeyPost(t *testing.T) {
	config := DefaultCSRFConfig()

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("api_key_id", "key-123")
		c.Next()
	})
	r.Use(SkipCSRFForAPIKey(config))
	r.POST("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "POST with API key should succeed without CSRF token")
}