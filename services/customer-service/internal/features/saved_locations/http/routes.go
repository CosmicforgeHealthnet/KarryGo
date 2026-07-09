package savedlocationhttp

import (
	"github.com/gin-gonic/gin"

	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	savedlocationusecases "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func RegisterSavedLocationRoutes(group *gin.RouterGroup, locations *savedlocationusecases.SavedLocationService, accessSigner *sharedauth.TokenSigner) {
	handler := NewSavedLocationHandler(locations)

	protected := group.Group("/saved-locations")
	protected.Use(sharedauth.BearerMiddleware(accessSigner, authusecases.CustomerRole, authusecases.CustomerService))
	protected.GET("", handler.ListLocations)
	protected.POST("", handler.AddLocation)
	protected.PUT("/:id", handler.UpdateLocation)
	protected.DELETE("/:id", handler.DeleteLocation)
}
