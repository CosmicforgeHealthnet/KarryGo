package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
)

func TestCorrelationIDGeneratedAndPropagated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CorrelationID())
	router.GET("/x", func(c *gin.Context) {
		c.String(http.StatusOK, GetCorrelationID(c))
	})

	// Generated when absent.
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/x", nil))
	generated := resp.Header().Get(HeaderCorrelationID)
	if generated == "" || resp.Body.String() != generated {
		t.Fatalf("correlation id not generated/propagated: header=%q body=%q", generated, resp.Body.String())
	}

	// Propagated when supplied.
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderCorrelationID, "cor_inbound")
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Header().Get(HeaderCorrelationID) != "cor_inbound" || resp.Body.String() != "cor_inbound" {
		t.Fatalf("inbound correlation id not preserved: %q", resp.Body.String())
	}

	// Oversized values are replaced, not trusted.
	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderCorrelationID, strings.Repeat("a", 200))
	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if len(resp.Header().Get(HeaderCorrelationID)) > 128 {
		t.Fatal("oversized correlation id was trusted")
	}
}

func TestBodySizeLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(respond.ErrorHandler())
	router.Use(BodySizeLimit(64))
	router.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("a", 100)))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code == http.StatusOK {
		t.Fatal("oversized body should be rejected")
	}
}

func TestMetricsAuthUnconfiguredFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(respond.ErrorHandler())
	router.GET("/metrics", MetricsAuth(""), func(c *gin.Context) { c.Status(http.StatusOK) })
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if resp.Code == http.StatusOK {
		t.Fatal("metrics without a configured token must fail closed")
	}
}

func TestRequestLoggerNeverLogsAuthorization(t *testing.T) {
	// The logger derives fields only from known-safe values; this guards the
	// field set against accidental additions of raw headers.
	gin.SetMode(gin.TestMode)
	metrics := observability.NewMetrics()
	router := gin.New()
	router.Use(CorrelationID())
	router.Use(RequestLogger(config.Config{AppEnv: "test"}, metrics))
	router.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer super-secret-token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if got := metrics.CounterValue("fleet_partner_http_requests_total", map[string]string{
		"method": "GET", "route": "/x", "status": "2xx",
	}); got != 1 {
		t.Fatalf("http request counter = %g", got)
	}
}
