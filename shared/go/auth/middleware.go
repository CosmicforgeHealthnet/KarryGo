package auth

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
)

const ClaimsContextKey = "auth_claims"

func BearerMiddleware(signer *TokenSigner, role string, service string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
			return
		}

		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if token == header || token == "" {
			httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
			return
		}

		claims, err := signer.Verify(token)
		if err != nil {
			message := "Your session has expired. Please sign in again."
			if errors.Is(err, ErrInvalidToken) {
				message = "Authentication is invalid. Please sign in again."
			}
			httpx.Abort(c, apperrors.Unauthorized(message, err))
			return
		}

		if claims.Type != TokenTypeAccess || claims.Role != role || claims.Service != service {
			httpx.Abort(c, apperrors.Forbidden("You do not have access to this action.", nil))
			return
		}

		c.Set(ClaimsContextKey, claims)
		c.Next()
	}
}

func ClaimsFromContext(c *gin.Context) (Claims, bool) {
	value, ok := c.Get(ClaimsContextKey)
	if !ok {
		return Claims{}, false
	}

	claims, ok := value.(Claims)
	return claims, ok
}
