package availabilityhttp

import (
	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/usecases"
	availabilityusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func RegisterAvailabilityRoutes(
	group *gin.RouterGroup,
	availability *availabilityusecases.AvailabilityService,
	providerSigner *sharedauth.TokenSigner,
	customerSigner *sharedauth.TokenSigner,
	defaultRadiusKm float64,
) {
	handler := NewAvailabilityHandler(availability, defaultRadiusKm)

	provider := group.Group("/provider")
	provider.Use(sharedauth.BearerMiddleware(providerSigner, providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
	provider.PUT("/availability", handler.SetAvailability)
	provider.POST("/availability/heartbeat", handler.Heartbeat)
	provider.GET("/availability", handler.GetAvailability)

	// Passenger-facing: verifies role `customer` / service `customer` against
	// the secret customer-service signs with.
	customer := group.Group("/customer")
	customer.Use(sharedauth.BearerMiddleware(customerSigner, "customer", "customer"))
	customer.GET("/availability", handler.CheckAvailability)
}
