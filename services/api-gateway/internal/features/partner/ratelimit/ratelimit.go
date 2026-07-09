// Package ratelimit provides Redis-backed per-partner, per-route rate limiting
// for the Partner API. It is attached after authentication so the partner
// identity is available in the Gin context.
package ratelimit

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partnerauth"
)

// Window is the fixed rate-limit window.
const Window = time.Minute

// Store is a minimal counter used by the limiter. Incr increments the key's
// counter and returns the new value, setting the TTL only when the key is first
// created so the window is not extended indefinitely.
type Store interface {
	Incr(ctx context.Context, key string, window time.Duration) (int64, error)
}

// Limiter enforces per-partner, per-route request limits.
type Limiter struct {
	store  Store
	limits map[string]int64
	window time.Duration
}

// NewLimiter builds a Limiter with the default Partner API limits.
func NewLimiter(store Store) *Limiter {
	return &Limiter{store: store, limits: DefaultLimits(), window: Window}
}

// DefaultLimits maps "METHOD FULLPATH" to a per-minute request limit.
func DefaultLimits() map[string]int64 {
	const base = "/api/v1/partner/v1"
	return map[string]int64{
		"POST " + base + "/quotes":            120,
		"POST " + base + "/deliveries":        60,
		"GET " + base + "/deliveries":         120,
		"GET " + base + "/deliveries/:id":     300,
		"DELETE " + base + "/deliveries/:id":  20,
		"POST " + base + "/webhooks":          10,
		"GET " + base + "/webhooks":           60,
		"DELETE " + base + "/webhooks/:id":    10,
		"POST " + base + "/webhooks/:id/test": 10,
	}
}

func (l *Limiter) limitFor(method, fullPath string) int64 {
	return l.limits[method+" "+fullPath]
}

// Middleware enforces the configured limit for the matched route. It fails OPEN
// (allows the request) if the store errors, so a rate-limit backend hiccup does
// not take the Partner API down; the error is logged.
func (l *Limiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		partnerID := partnerauth.PartnerID(c)
		route := c.FullPath()
		limit := l.limitFor(c.Request.Method, route)
		if partnerID == "" || limit <= 0 {
			c.Next()
			return
		}

		// Key uses the route PATTERN (c.FullPath()) so path params like :id do not
		// create unbounded distinct keys.
		key := "rl:" + partnerID + ":" + c.Request.Method + ":" + route
		count, err := l.store.Incr(c.Request.Context(), key, l.window)
		if err != nil {
			log.Printf("ratelimit: store error (failing open): %v", err)
			c.Next()
			return
		}

		remaining := limit - count
		if remaining < 0 {
			remaining = 0
		}
		c.Header("X-RateLimit-Limit", strconv.FormatInt(limit, 10))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

		if count > limit {
			writeRateLimited(c)
			return
		}
		c.Next()
	}
}

func writeRateLimited(c *gin.Context) {
	// Uses the shared Partner API error envelope (success:false, error:{code,message,fields:[]}).
	partnerauth.WriteError(c, http.StatusTooManyRequests, "rate_limit_exceeded", "Too many requests. Slow down.")
}
