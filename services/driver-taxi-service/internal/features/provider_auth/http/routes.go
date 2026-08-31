package providerauthhttp

import (
	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func RegisterProviderAuthRoutes(group *gin.RouterGroup, authService *providerauthusecases.AuthService, otpDebug bool) {
	handler := NewAuthHandler(authService, otpDebug)

	authGroup := group.Group("/provider/auth")
	authGroup.POST("/start", handler.StartAuth)
	authGroup.POST("/verify", handler.VerifyAuth)
	authGroup.POST("/refresh", handler.Refresh)
	authGroup.POST("/logout", handler.Logout)

	protected := group.Group("/provider")
	protected.Use(sharedauth.BearerMiddleware(authService.AccessSigner(), providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
	protected.GET("/me", handler.Me)
	protected.POST("/phone/change/start", handler.ChangePhoneStart)
	protected.POST("/phone/change/verify", handler.ChangePhoneVerify)
}
