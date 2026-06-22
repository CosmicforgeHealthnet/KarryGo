package availabilityhttp

import (
	"github.com/gin-gonic/gin"

	availabilityusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/usecases"
	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func RegisterAvailabilityRoutes(
	group *gin.RouterGroup,
	svc *availabilityusecases.AvailabilityService,
	authSvc *providerauthusecases.AuthService,
	customerSigner *sharedauth.TokenSigner,
) {
	handler := NewAvailabilityHandler(svc)

	// Provider routes (require truck_provider bearer)
	providerGroup := group.Group("/provider")
	providerGroup.Use(sharedauth.BearerMiddleware(authSvc.AccessSigner(), providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
	providerGroup.PUT("/availability", handler.SetAvailability)
	providerGroup.POST("/availability/heartbeat", handler.Heartbeat)
	providerGroup.GET("/availability", handler.GetStatus)

	// Customer route (require customer bearer)
	customerGroup := group.Group("/customer")
	customerGroup.Use(sharedauth.BearerMiddleware(customerSigner, "customer", "customer"))
	customerGroup.GET("/availability", handler.CheckAvailability)
}
