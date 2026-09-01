package providerprofilehttp

import (
	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/usecases"
	providerprofileusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

// RegisterProfileRoutes mounts the driver-facing profile and vehicle surface.
// The passenger-facing vehicle lookup (/customer/vehicles/:id) lives with the
// other customer routes in the booking feature.
func RegisterProfileRoutes(
	group *gin.RouterGroup,
	profiles *providerprofileusecases.ProfileService,
	providerSigner *sharedauth.TokenSigner,
) {
	handler := NewProfileHandler(profiles)

	// Public: the passenger app renders fare tiles from this before booking.
	group.GET("/vehicle-classes", handler.ListVehicleClasses)

	provider := group.Group("/provider")
	provider.Use(sharedauth.BearerMiddleware(providerSigner, providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
	provider.GET("/profile", handler.GetProfile)
	provider.PUT("/profile", handler.UpdateProfile)
	provider.PUT("/parcel-preference", handler.SetParcelPreference)
	provider.GET("/contact-availability", handler.CheckContactAvailability)
	provider.POST("/vehicles", handler.CreateVehicle)
	provider.GET("/vehicles", handler.ListVehicles)
	provider.GET("/vehicles/:id", handler.GetVehicle)
	provider.PUT("/vehicles/:id", handler.UpdateVehicle)
}
