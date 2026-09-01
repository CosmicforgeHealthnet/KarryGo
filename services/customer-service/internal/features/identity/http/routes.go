package identityhttp

import (
	"github.com/gin-gonic/gin"

	profileusecases "cosmicforge/logistics/services/customer-service/internal/features/profile/usecases"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// RegisterIdentityRoutes wires the internal customer-lookup endpoint behind HMAC
// service-auth. Registration is skipped when no service secrets are configured.
func RegisterIdentityRoutes(group *gin.RouterGroup, profileService *profileusecases.ProfileService, verifier *serviceauth.Verifier) {
	if verifier == nil {
		return
	}
	handler := NewHandler(profileService)

	internal := group.Group("/internal")
	internal.Use(serviceauth.Middleware(verifier))
	// The wildcard must be named exactly as it is on the sibling
	// /internal/customers/:customerId/saved-locations/... route: gin panics at
	// registration when the same path position uses two different wildcard
	// names, which took customer-service down at startup entirely. The name is
	// internal to routing — the URL contract is unchanged.
	internal.GET("/customers/:customerId", handler.GetCustomer)
}
