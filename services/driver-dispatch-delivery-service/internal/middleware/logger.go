package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
)

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		log.Printf(
			"request_id=%s method=%s path=%s status=%d latency_ms=%d",
			httpx.LogRequestID(c),
			c.Request.Method,
			c.FullPath(),
			c.Writer.Status(),
			time.Since(start).Milliseconds(),
		)
	}
}
