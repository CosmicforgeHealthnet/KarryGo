package httpx

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/logging"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" || len(requestID) > 128 || strings.IndexFunc(requestID, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
			requestID = uuid.NewString()
		}

		c.Set(RequestIDKey, requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				// Gin's default recovery dumps headers and the panic value before
				// invoking a callback. Neither is safe on credential endpoints.
				logging.Structured("error", "http_panic", map[string]any{"route": c.FullPath()})
				RespondError(c, apperrors.Internal("Something went wrong. Please try again.", nil))
				c.Abort()
			}
		}()
		c.Next()
	}
}

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 || c.Writer.Status() < http.StatusBadRequest && c.Writer.Written() {
			return
		}

		RespondError(c, c.Errors.Last().Err)
	}
}
