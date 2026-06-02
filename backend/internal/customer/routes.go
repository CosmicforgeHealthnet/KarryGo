package customer

import (
	"github.com/gin-gonic/gin"

	"karrygo/backend/internal/platform/apperrors"
	"karrygo/backend/internal/platform/httpx"
)

const (
	ContextUserIDKey     = "userID"
	ContextCustomerIDKey = "customerID"
	ContextRoleKey       = "role"
	CustomerRole         = "customer"
)

func RegisterRoutes(v1 *gin.RouterGroup) {
	profileController := NewProfileController(NewProfileService(NewProfileRepository()))

	customerRoutes := v1.Group("/customer")
	customerRoutes.GET("/me", RequireCustomerContext(), profileController.GetMe)
}

func RequireCustomerContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := c.Get(ContextUserIDKey); !ok {
			httpx.Abort(c, apperrors.Unauthorized("Customer authentication is required.", nil))
			return
		}

		if _, ok := c.Get(ContextCustomerIDKey); !ok {
			httpx.Abort(c, apperrors.Unauthorized("Customer authentication is required.", nil))
			return
		}

		roleValue, ok := c.Get(ContextRoleKey)
		if !ok {
			httpx.Abort(c, apperrors.Unauthorized("Customer authentication is required.", nil))
			return
		}

		role, ok := roleValue.(string)
		if !ok || role != CustomerRole {
			httpx.Abort(c, apperrors.Forbidden("Customer access is required.", nil))
			return
		}

		c.Next()
	}
}
