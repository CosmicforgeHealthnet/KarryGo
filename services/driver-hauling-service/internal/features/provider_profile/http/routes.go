package providerprofilehttp

import (
	"github.com/gin-gonic/gin"

	providerprofileusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_profile/usecases"
	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func RegisterProfileRoutes(group *gin.RouterGroup, svc *providerprofileusecases.ProfileService, authSvc *providerauthusecases.AuthService) {
	handler := NewProfileHandler(svc)

	protected := group.Group("/provider")
	protected.Use(sharedauth.BearerMiddleware(authSvc.AccessSigner(), providerauthusecases.ProviderRole, providerauthusecases.ProviderService))

	protected.GET("/profile", handler.GetProfile)
	protected.PUT("/profile", handler.UpdateProfile)

	protected.POST("/trucks", handler.CreateTruck)
	protected.GET("/trucks", handler.ListTrucks)
	protected.GET("/trucks/:id", handler.GetTruck)
	protected.PUT("/trucks/:id", handler.UpdateTruck)
}
