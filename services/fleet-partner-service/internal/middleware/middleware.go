// Package middleware implements the HTTP middleware chain for
// fleet-partner-service.
//
// Documented ordering (outermost first):
//
//  1. Recovery                (panic -> safe 500 envelope)
//  2. RequestID               (shared httpx X-Request-ID)
//  3. CorrelationID           (X-Correlation-ID in/out)
//  4. Tracing                 (W3C traceparent in/out — see internal/platform/tracing)
//  5. SecureHeaders
//  6. BodySizeLimit
//  7. RequestLogger + Metrics instrumentation
//  8. ErrorEnvelope           (terminal error mapping)
//  9. Route-class boundaries: ContentType, Bearer auth / service auth,
//     tenant resolution — applied per route group.
//
// This ordering is exercised end to end by app.BuildRouter and its own test
// suite (internal/app); this package's unit tests cover each middleware in
// isolation instead of re-asserting gin's registration order, which is not
// itself independently observable outside of BuildRouter's own construction.
//
// Authorization headers, cookies, tokens and raw request bodies are never
// logged.
package middleware

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tracing"
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

// TraceContextKey is the gin context key holding the resolved trace context.
const TraceContextKey = "trace_context"

// HeaderTraceParent is the W3C propagation header (see internal/platform/tracing).
const HeaderTraceParent = "traceparent"

// HeaderTraceState is the opaque, capped W3C companion header.
const HeaderTraceState = "tracestate"

// Tracing extracts a valid inbound "traceparent"/"tracestate" pair or starts
// a fresh root trace, attaches it to both the gin context (for handlers and
// RequestLogger) and the request's context.Context (so every downstream
// platformclients call and usecase automatically inherits it without having
// to thread it through explicitly — see internal/platformclients/base.go's
// Exchange, which reads it back out via tracing.FromContext).
//
// sampleRate only applies to a NEW root trace; a propagated trace always
// keeps the upstream sampling decision (W3C head-based sampling).
func Tracing(sampleRate float64) gin.HandlerFunc {
	return func(c *gin.Context) {
		tc, ok := tracing.ParseTraceParent(c.GetHeader(HeaderTraceParent))
		if !ok {
			tc = tracing.New(tracing.ShouldSample(sampleRate))
		}
		if state := c.GetHeader(HeaderTraceState); state != "" {
			tc = tc.WithTraceState(state)
		}
		c.Set(TraceContextKey, tc)
		c.Request = c.Request.WithContext(tracing.WithContext(c.Request.Context(), tc))
		c.Next()
	}
}

// GetTraceContext returns the resolved trace context for the request.
func GetTraceContext(c *gin.Context) tracing.Context {
	value, _ := c.Get(TraceContextKey)
	tc, _ := value.(tracing.Context)
	return tc
}

// Recovery replaces shared/go/httpx.Recovery for this service: it responds
// identically (httpx.RespondError's safe 500 envelope, request ID included)
// but logs the panic as a structured JSON line via observability.Log instead
// of shared/go/logging's plain-text "recovered req=%s value=%s" format.
// httpx.Recovery is a monorepo-wide shared package used by other services
// too, so its log format is left unchanged there; this is a service-local
// correction, not a second recovery mechanism -- exactly one panic handler
// is ever installed (see app.BuildRouter).
func Recovery(cfg config.Config) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		tc := GetTraceContext(c)
		value := fmt.Sprintf("%v", recovered)
		if len(value) > 500 {
			value = value[:500]
		}
		observability.Log(config.ServiceName, cfg.AppEnv, "error", "panic_recovered", map[string]any{
			"requestId":      httpx.GetRequestID(c),
			"correlationId":  GetCorrelationID(c),
			"traceId":        tc.TraceID,
			"spanId":         tc.SpanID,
			"route":          c.FullPath(),
			"method":         c.Request.Method,
			"safeErrorClass": "panic",
			"recovered":      value,
		})
		httpx.RespondError(c, apperrors.Internal("Something went wrong. Please try again.", nil))
		c.Abort()
	})
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

		tc := GetTraceContext(c)
		fields := map[string]any{
			"requestId":       httpx.GetRequestID(c),
			"correlationId":   GetCorrelationID(c),
			"traceId":         tc.TraceID,
			"spanId":          tc.SpanID,
			"normalizedRoute": route,
			"route":           route,
			"method":          c.Request.Method,
			"statusCode":      status,
			"status":          status,
			"durationMs":      duration.Milliseconds(),
		}
		if tenantCtx, ok := TenantFromGin(c); ok {
			fields["organizationId"] = tenantCtx.OrganizationID
			fields["actorType"] = string(tenantCtx.ActorType)
			fields["actorId"] = tenantCtx.ActorID
		} else if orgID, ok := c.Get(TenantOrganizationIDKey); ok {
			fields["organizationId"] = orgID
		}
		if caller := c.GetString(ServiceCallerKey); caller != "" {
			fields["serviceCallerId"] = caller
			if _, hasActor := fields["actorType"]; !hasActor {
				fields["actorType"] = "service"
				fields["actorId"] = caller
			}
		}
		if branchID := c.Param("branchId"); branchID != "" {
			fields["branchId"] = branchID
		}
		if len(c.Errors) > 0 {
			appErr := apperrors.From(c.Errors.Last().Err)
			fields["safeErrorClass"] = statusClass(status)
			fields["safeErrorCode"] = string(appErr.Code)
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
