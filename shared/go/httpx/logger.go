package httpx

import (
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	logReset  = "\033[0m"
	logRed    = "\033[31m"
	logGreen  = "\033[32m"
	logYellow = "\033[33m"
	logCyan   = "\033[36m"
	logBold   = "\033[1m"
	logDim    = "\033[2m"
)

// Logger returns a Gin middleware that logs every request with method, path, status, latency,
// and request ID in color: green for 2xx, yellow for 3xx/4xx, red for 5xx.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		if c.Request.URL.RawQuery != "" {
			path = path + "?" + c.Request.URL.RawQuery
		}

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		requestID := GetRequestID(c)

		color := statusColor(status)
		method := methodColor(c.Request.Method)

		log.Printf(
			"%s%s%s %s%-24s%s %s%3d%s  %s%-10s%s  %sreq=%s%s",
			logBold, method, logReset,
			logCyan, path, logReset,
			color+logBold, status, logReset,
			logDim, formatLatency(latency), logReset,
			logDim, requestID, logReset,
		)
	}
}

func statusColor(status int) string {
	switch {
	case status >= 500:
		return logRed
	case status >= 300:
		return logYellow
	default:
		return logGreen
	}
}

func methodColor(method string) string {
	switch method {
	case "GET":
		return logGreen + "GET   "
	case "POST":
		return logCyan + "POST  "
	case "PUT":
		return logYellow + "PUT   "
	case "PATCH":
		return logYellow + "PATCH "
	case "DELETE":
		return logRed + "DELETE"
	default:
		return fmt.Sprintf("%-6s", method)
	}
}

func formatLatency(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}
