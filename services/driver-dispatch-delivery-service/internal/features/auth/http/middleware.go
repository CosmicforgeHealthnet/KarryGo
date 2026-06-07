package authhttp

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
)

const (
	ContextDispatchRiderID = "dispatch_rider_id"
	ContextPhoneNumber     = "phone_number"
	ContextSessionID       = "session_id"
	ContextRole            = "role"
)

func DispatchRiderAuthRequired(tokens *authusecases.TokenUsecase) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		parts := strings.Fields(authHeader)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			httpx.Abort(c, apperrors.Unauthorized("Access token is invalid.", nil))
			return
		}

		rawToken := parts[1]
		claims, err := tokens.ValidateAccessToken(rawToken)
		if err != nil {
			if errors.Is(err, authusecases.ErrExpiredToken) {
				httpx.Abort(c, apperrors.Unauthorized("Access token has expired.", nil))
				return
			}

			httpx.Abort(c, apperrors.Unauthorized("Access token is invalid.", nil))
			return
		}

		// TODO: JWT jti claim is not currently included; access-token blacklist will be completed when jti support is added.
		c.Set(ContextDispatchRiderID, claims.DispatchRiderID)
		c.Set(ContextPhoneNumber, claims.PhoneNumber)
		c.Set(ContextSessionID, claims.SessionID)
		c.Set(ContextRole, claims.Role)
		c.Next()
	}
}

func DispatchRiderID(c *gin.Context) string {
	return c.GetString(ContextDispatchRiderID)
}

func Role(c *gin.Context) string {
	return c.GetString(ContextRole)
}

func SessionID(c *gin.Context) string {
	return c.GetString(ContextSessionID)
}

func PhoneNumber(c *gin.Context) string {
	return c.GetString(ContextPhoneNumber)
}
