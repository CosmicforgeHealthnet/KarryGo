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
	internal.GET("/customers/:id", handler.GetCustomer)
}
