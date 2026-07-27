// Package middleware implements the HTTP middleware chain for
// fleet-partner-service.
//
// Documented ordering (outermost first) — enforced by TestMiddlewareOrdering:
//
//  1. Recovery                (panic -> safe 500 envelope)
//  2. RequestID               (shared httpx X-Request-ID)
//  3. CorrelationID           (X-Correlation-ID in/out)
//  4. SecureHeaders
//  5. BodySizeLimit
//  6. RequestLogger + Metrics instrumentation
//  7. ErrorEnvelope           (terminal error mapping)
//  8. Route-class boundaries: ContentType, Bearer auth / service auth,
//     tenant resolution — applied per route group.
//
// Authorization headers, cookies, tokens and raw request bodies are never
// logged.
package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// CorrelationIDKey is the gin context key holding the correlation ID.
const CorrelationIDKey = "correlation_id"

// HeaderCorrelationID is the propagation header for cross-service tracing.
const HeaderCorrelationID = "X-Correlation-ID"

// CorrelationID accepts an inbound correlation ID or generates one, and
// always echoes it on the response.
func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		correlationID := strings.TrimSpace(c.GetHeader(HeaderCorrelationID))
		if correlationID == "" || len(correlationID) > 128 {
			correlationID = uuid.NewString()
		}
		c.Set(CorrelationIDKey, correlationID)
		c.Header(HeaderCorrelationID, correlationID)
		c.Next()
	}
}

// GetCorrelationID returns the resolved correlation ID.
func GetCorrelationID(c *gin.Context) string {
	value, _ := c.Get(CorrelationIDKey)
	correlationID, _ := value.(string)
	return correlationID
}

// SecureHeaders sets conservative security headers for an API service.
func SecureHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

// BodySizeLimit rejects request bodies above the configured maximum.
func BodySizeLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			httpx.Abort(c, apperrors.New(413, apperrors.CodeBadRequest, "Request body is too large.", nil))
			return
		}
		c.Next()
	}
}

// RequireJSON enforces application/json on mutating requests.
// multipart/form-data is reserved for a future direct-upload handoff and is
// not accepted by any foundation route.
func RequireJSON() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case "POST", "PUT", "PATCH":
			contentType := c.ContentType()
			if c.Request.ContentLength != 0 && contentType != "application/json" {
				httpx.Abort(c, apperrors.New(415, apperrors.CodeBadRequest, "Content-Type must be application/json.", nil))
				return
			}
		}
		c.Next()
	}
}

// RequestLogger emits one structured log line per request and records HTTP
// metrics. Route templates (not raw paths) are used as metric labels.
func RequestLogger(cfg config.Config, metrics *observability.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		labels := map[string]string{"method": c.Request.Method, "route": route}
		metrics.AddGauge("fleet_partner_http_active_requests", nil, 1)

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()
		metrics.AddGauge("fleet_partner_http_active_requests", nil, -1)
		metrics.IncCounter("fleet_partner_http_requests_total", map[string]string{
			"method": labels["method"], "route": labels["route"], "status": statusClass(status),
		}, 1)
		metrics.ObserveDuration("fleet_partner_http_request_duration", labels, duration)

		fields := map[string]any{
			"requestId":     httpx.GetRequestID(c),
			"correlationId": GetCorrelationID(c),
			"route":         route,
			"method":        c.Request.Method,
			"status":        status,
			"durationMs":    duration.Milliseconds(),
		}
		if orgID, ok := c.Get(TenantOrganizationIDKey); ok {
			fields["organizationId"] = orgID
		}
		if len(c.Errors) > 0 {
			fields["errorCode"] = string(apperrors.From(c.Errors.Last().Err).Code)
		}
		observability.Log(config.ServiceName, cfg.AppEnv, levelFor(status), "http_request", fields)
	}
}

func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

func levelFor(status int) string {
	switch {
	case status >= 500:
		return "error"
	case status >= 400:
		return "warn"
	default:
		return "info"
	}
}
