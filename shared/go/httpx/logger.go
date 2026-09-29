package httpx

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/observability"
)

// Logger emits one structured access record per request (Customer V4 §27.1):
// request ID (hashed unless a UUID), W3C trace ID, the matched route
// TEMPLATE (never the raw path or query), method, status, latency and the
// bounded outcome category. It goes through the canonical logging.Structured
// / SafeFields path, so no header, body, query or credential is ever read.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := c.Writer.Status()
		fields := map[string]any{
			"request_id": LogRequestID(c),
			"route":      route,
			"method":     c.Request.Method,
			"status":     status,
			"latency_ms": float64(time.Since(start).Microseconds()) / 1000,
			"outcome":    string(observability.OutcomeForStatus(status)),
		}
		if traceID := TraceID(c.GetHeader("traceparent")); traceID != "" {
			fields["trace_id"] = traceID
		}
		level := "info"
		if status >= 500 {
			level = "error"
		}
		logging.Structured(level, "http_request", fields)
	}
}

// TraceID returns the trace-id of a well-formed W3C traceparent header and
// "" otherwise, so a malformed or hostile header never reaches a log line.
func TraceID(traceparent string) string {
	parts := strings.Split(strings.TrimSpace(traceparent), "-")
	if len(parts) != 4 || len(parts[0]) != 2 || len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return ""
	}
	for _, r := range parts[1] {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return ""
		}
	}
	if parts[1] == strings.Repeat("0", 32) {
		return ""
	}
	return parts[1]
}
