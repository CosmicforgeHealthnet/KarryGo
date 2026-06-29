package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

const InternalServiceKeyHeader = "X-Internal-Service-Key"

func RequireServiceKey(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isBearerAuthorization(c.GetHeader("Authorization")) {
			httpx.Abort(c, apperrors.Unauthorized("Internal service key is required.", nil))
			return
		}

		expected = strings.TrimSpace(expected)
		provided := strings.TrimSpace(c.GetHeader(InternalServiceKeyHeader))
		if expected == "" || provided == "" {
			httpx.Abort(c, apperrors.Unauthorized("Internal service key is required.", nil))
			return
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			httpx.Abort(c, apperrors.Unauthorized("Internal service key is invalid.", nil))
			return
		}
		c.Next()
	}
}

func isBearerAuthorization(header string) bool {
	parts := strings.Fields(header)
	return len(parts) == 2 && strings.EqualFold(parts[0], "Bearer")
}
