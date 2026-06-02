package authhttp

import (
	"github.com/gin-gonic/gin"

	authusecases "karrygo/services/customer-service/internal/features/auth/usecases"
	sharedauth "karrygo/shared/go/auth"
)

func RegisterCustomerRoutes(group *gin.RouterGroup, authService *authusecases.AuthService) {
	handler := NewAuthHandler(authService)

	authGroup := group.Group("/auth")
	authGroup.POST("/start", handler.StartAuth)
	authGroup.POST("/verify", handler.VerifyAuth)
	authGroup.POST("/refresh", handler.Refresh)
	authGroup.POST("/logout", handler.Logout)

	protected := group.Group("")
	protected.Use(sharedauth.BearerMiddleware(authService.AccessSigner(), authusecases.CustomerRole, authusecases.CustomerService))
	protected.GET("/me", handler.Me)
}
