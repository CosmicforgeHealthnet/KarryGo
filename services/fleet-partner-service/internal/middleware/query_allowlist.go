package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/filters"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/queryallow"
)

// QueryAllowlist rejects unknown, repeated or malformed query parameters on
// the tenant GET routes declared in internal/platform/queryallow (spec 6:
// "Explicit allowlisted filters and sort fields. Reject unknown filters.").
//
// It runs inside the route group, after the route has matched, so FullPath()
// names the route pattern rather than the raw URL. Routes that are not
// declared (single-record reads ending in a path parameter) and non-GET
// requests pass through unchanged.
func QueryAllowlist() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet || c.Request.URL.RawQuery == "" {
			c.Next()
			return
		}
		schema, ok := queryallow.Lookup(c.Request.Method, c.FullPath())
		if !ok {
			c.Next()
			return
		}
		if err := filters.Validate(c.Request.URL.Query(), schema); err != nil {
			httpx.Abort(c, err)
			return
		}
		c.Next()
	}
}
