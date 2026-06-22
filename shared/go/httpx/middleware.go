package httpx

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/logging"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}

		c.Set(RequestIDKey, requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		logging.Error("panic", "recovered req=%s value=%s", GetRequestID(c), fmt.Sprintf("%v", recovered))
		RespondError(c, apperrors.Internal("Something went wrong. Please try again.", nil))
		c.Abort()
	})
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
